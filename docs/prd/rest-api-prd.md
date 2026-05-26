# REST API + Defense-in-Depth Scoping PRD

## Goal

Expose `go-eventkit`'s calendar and reminders surface as a localhost REST API with an OpenAPI 3.1 spec, gated by an allowlist policy that enforces *which* calendars, reminder lists, and sources the API can touch. The API is the foundation for an MCP server that will hand AI agents controlled access to the user's macOS Calendar and Reminders without exposing every account by default.

## Non-goals

- Authentication (deferred — localhost-only is the perimeter).
- Multi-tenant or multi-user. Single user, single server, single policy file.
- Cross-host exposure. Server binds 127.0.0.1 by default and rejects non-loopback `Host` headers.
- Change-notification streaming (SSE/WS) — deferred to v2.
- Natural-language date parsing in request DTOs. REST inputs are ISO 8601; consumers (e.g. the MCP server) can wrap with `dateparser` themselves.

## Design

### Package layout

```
server/                        # library entrypoint (importable for MCP server use)
├── server.go                  # Server struct, New(opts ...Option), Handler()
├── options.go                 # Option funcs (WithAddr, WithPolicy, WithLogger…)
├── bridge.go                  # CalendarBridge, RemindersBridge interfaces
├── humax.go                   # huma API construction, error mapping
├── policy/                    # YAML-driven allowlist
├── scoped/                    # Bridge wrappers that re-check the policy on every call
├── handlers/                  # huma operation registrations + DTOs
├── middleware/                # loopback + body-limit + audit
└── doc.go

cmd/eventkit-server/           # binary
└── main.go
```

### Defense-in-depth layers

A request must pass every check. Each layer assumes upstream layers may have a bug.

| Layer | Where | What it does |
|---|---|---|
| Transport bind | binary main | Default `127.0.0.1:8765`. Refuses to bind a non-loopback address without an explicit `--insecure-bind` flag. |
| `Recover` | `middleware/recover.go` | `defer/recover` around every request. A panicking handler becomes 500 with a logged stack; subsequent requests keep serving. `http.ErrAbortHandler` is propagated. |
| `StripServerHeader` | `middleware/serverheader.go` | Deletes the default `Server: ...` response header to avoid runtime version disclosure. |
| `Loopback` | `middleware/loopback.go` | Rejects requests whose `Host` header is not loopback *or* whose `RemoteAddr` is not loopback. 403, no body. |
| `RequestID` | `middleware/requestid.go` | Generates an opaque 128-bit hex id (or echoes a sanitized incoming `X-Request-ID`); stored in request context and emitted on every response and audit log. |
| `RejectMethodOverride` | `middleware/methodoverride.go` | 400 if any of `X-HTTP-Method-Override`, `X-HTTP-Method`, `X-Method-Override` is set — the server doesn't honor remapping. |
| `RequestTimeout` | `middleware/timeout.go` | `context.WithTimeout` (30s default, configurable). Slow handler → 504. Panics inside the spawned goroutine are re-raised to the outer `Recover`. |
| `BodyLimit` | `middleware/bodylimit.go` | `http.MaxBytesReader` caps body at 1 MiB. |
| `RequireContentType` | `middleware/contenttype.go` | POST/PUT/PATCH with non-empty body must use `application/json` (charset allowed). 415 otherwise. |
| huma validation | huma framework | Required fields, enum values, ISO 8601 formats checked before the handler runs. |
| Policy gate | `scoped/*.go` | Every read/write looks up the target's calendar/list ID + source against the loaded `Policy`. Reads → must be ≥ `read`. Writes → must be `readwrite`. |
| Post-fetch re-verification | `scoped/*.go` | After EventKit returns a saved/fetched object, the result's calendar/source is re-checked against the policy. Mismatch returns the typed `*PostFetchScopeViolation` carrying orphan id + source; the handler renders 422 with structured detail. **No auto-rollback** — EventKit is not transactional. |
| `Audit` | `middleware/audit.go` | Every non-GET request logged with method, path, status, elapsed, and request id. |

Three explicit checks across the gate, the post-fetch verify, and the audit log give multiple independent code paths that must all agree before a write commits to the API.

**Skipped (deliberately):** Origin / Referer header verification — the loopback middleware (Host + RemoteAddr) is the perimeter; modern browsers cannot override `Host` from JavaScript on cross-origin requests.

### Policy file

```yaml
calendar:
  default: deny             # or "read" / "readwrite"
  allow:
    - id: ABCDEF-1234       # match by calendar identifier
      mode: readwrite
    - source: iCloud        # match all calendars under this source
      mode: read

reminders:
  default: deny
  allow:
    - list_id: GHIJKL-5678
      mode: readwrite
    - source: iCloud
      mode: read
```

**Precedence:** if any allow-entry names the ID explicitly, that entry's mode wins (whether more or less permissive than a source-level entry). Otherwise, the most permissive among source-matching entries wins. If nothing matches, the package's `default` applies.

This rule lets users write "allow iCloud broadly, but lock down a specific cal to read-only" without surprises.

### REST surface

All routes mirror the existing Go API 1:1 under `/v1`.

```
GET    /healthz
GET    /openapi.json
GET    /docs                                     # huma's Swagger UI

GET    /v1/calendars                             # filtered to allowed
POST   /v1/calendars
GET    /v1/calendars/{id}
PATCH  /v1/calendars/{id}
DELETE /v1/calendars/{id}

GET    /v1/events?start=&end=&calendar=&calendar_id=&search=
POST   /v1/events
GET    /v1/events/{id}
PATCH  /v1/events/{id}?span=this|future
DELETE /v1/events/{id}?span=this|future
POST   /v1/events/batch-delete                   # body: {ids, span}

GET    /v1/lists
POST   /v1/lists
GET    /v1/lists/{id}
PATCH  /v1/lists/{id}
DELETE /v1/lists/{id}

GET    /v1/reminders?list=&list_id=&completed=&due_before=&due_after=&search=
POST   /v1/reminders
GET    /v1/reminders/{id}
PATCH  /v1/reminders/{id}
DELETE /v1/reminders/{id}
POST   /v1/reminders/{id}/complete
POST   /v1/reminders/{id}/uncomplete
POST   /v1/reminders/batch-delete                # body: {ids}
```

### Error mapping

| Source | HTTP |
|---|---|
| `calendar.ErrNotFound` / `reminders.ErrNotFound` | 404 |
| Policy denial | 404 (deliberately matches not-found to avoid leaking existence) |
| `calendar.ErrImmutable` / `reminders.ErrImmutable` | 409 |
| huma validation failure | 422 |
| `calendar.ErrAccessDenied` / `reminders.ErrAccessDenied` | 503 (TCC not granted) |
| `calendar.ErrUnsupported` / `reminders.ErrUnsupported` | 501 |
| Anything else | 500 |

## Dependencies added

Three small libraries. The project moves from zero deps to three; the cost is justified by:

| Dep | Use |
|---|---|
| `github.com/danielgtaylor/huma/v2` | OpenAPI 3.1 generation from Go types, request validation. Removes the entire class of "spec drifted from code" bugs. |
| `github.com/go-chi/chi/v5` | Router under huma's `humachi` adapter. Idiomatic Go, no surprises. |
| `gopkg.in/yaml.v3` | Policy file parser. |

## Testing

Match the project's existing convention: standard `testing` package, table-driven, `t.Run` subtests, no assertion library.

- **Policy unit tests** — YAML round-trip, allow matrix, precedence rules, invalid-config rejection.
- **Scoped client unit tests** — Recording fake bridge + permutations of policy modes. Verify list-filter behavior; verify reads return `ErrNotFound` on denial; verify writes refuse before reaching the bridge; verify post-fetch re-verification rejects results outside scope.
- **Middleware unit tests** — Accept loopback, reject everything else.
- **Handler unit tests** — `humatest` exercises full request → response cycle with a stub scoped client. Cover happy path, validation failure, policy denial, not-found.
- **OpenAPI contract test** — Walk the generated spec; assert every bridge method has at least one registered operation.
- **Integration test** (`scripts/integration_server.go`, build-tagged `darwin && integration`) — Start the server over an ephemeral port with real `calendar.Client` + `reminders.Client`, round-trip every endpoint, clean up. One restricted-policy test verifies that writes to forbidden calendars/lists return 404 while reads still return 200.

## Open decisions deferred to implementation

- **Audit log destination**: stdout for now; a future flag can route to a file.
- **Hot reload of policy**: not in v1. Restart the server to apply a new policy.
- **OpenAPI servers list**: hardcoded to `http://127.0.0.1:8765` in v1.

## v2 hardening pass (post-MVP)

After v1 shipped the basic CRUD + policy, a hardening pass added:

- `Recover`, `RequestID`, `RequestTimeout`, `RejectMethodOverride`, `RequireContentType`, `StripServerHeader` middleware (table above).
- Scalar API Reference at `/docs` via huma's built-in `DocsRendererScalar` (was Stoplight Elements).
- Typed `*PostFetchScopeViolation` carrying orphan id/source; mapped to 422 with structured `ErrorDetail` entries so the client can render a useful message and the operator can locate the orphan.
- Test depth: `audit` / `recover` / `timeout` / `contenttype` / `methodoverride` / `serverheader` / `requestid` unit tests; per-bridge error-injection across all scoped operations; explicit `Uncomplete` direct test; concurrent reads under `-race`; policy fuzz (`FuzzLoad`) + YAML testdata fixtures; handler `MapError` matrix; OpenAPI 3.1 spec structural check; HTTP-level `X-Request-ID` / `Server` header assertions; restricted-policy integration variant.
