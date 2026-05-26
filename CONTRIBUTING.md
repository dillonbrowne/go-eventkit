# Contributing to go-eventkit

Thanks for the interest. This repo aims to stay small, well-tested, and idiomatic Go. Before opening a PR, please read the rules below.

## Commit messages

All commits MUST follow [Conventional Commits](https://www.conventionalcommits.org/). Examples:

- `feat(reminders): add Flagged field`
- `fix(scoped): use calendarItemWithIdentifier for delete on macOS 26+`
- `docs(playbook): cover SIGHUP reload`
- `test(server): cover the 422 post-fetch path`
- `chore(deps): bump huma to v2.39`

Types: `feat`, `fix`, `docs`, `style`, `refactor`, `test`, `chore`, `build`, `ci`, `perf`. No exceptions.

## Before pushing

Run locally:

```sh
gofmt -l .                                    # must be empty
go vet ./...                                  # must be clean
golangci-lint run ./...                       # must be clean (config in .golangci.yml)
go test -race -count=1 ./...                  # must be green
GOOS=linux CGO_ENABLED=0 go build ./...       # cross-build must still succeed
```

CI runs the same set on every PR. If `golangci-lint` is not installed locally:

```sh
brew install golangci-lint
```

## Adding dependencies

This repo deliberately keeps third-party deps minimal: huma/v2, chi/v5, yaml.v3, and golang.org/x/time. Any new third-party dep needs justification in the PR description — "common Go idiom" is not sufficient if the same thing can be done with stdlib.

## cgo bridge changes

The `calendar/` and `reminders/` packages have C/Objective-C source that runs against Apple's EventKit framework. If you change `*.m` or `*.h` files:

- `#cgo CFLAGS: -x objective-c -fobjc-arc` is mandatory. Without ARC, EventKit returns empty results or segfaults.
- Bridge functions must return `ek_result_t` (struct with `result` + `error` pointers). No `__thread` TLS — unsafe under Go's M:N scheduler.
- All write operations must run inside `dispatch_sync(get_write_queue(), ^{...})`.
- For reminders, lookups feeding `removeReminder:` / `saveReminder:` must go through `resolve_reminder_for_write` — see the comment in `reminders/bridge_darwin.m` for the macOS 26 rationale.

Test bridge changes with the integration scripts:

```sh
go run -tags integration ./scripts/integration.go
go run -tags integration ./scripts/integration_reminders.go
go run -tags integration ./scripts/integration_server.go
```

These hit a real EventKit and require TCC permission for the terminal app.

## Tests

- Use the standard `testing` package — no testify.
- Table-driven tests with `t.Run` subtests.
- Race detector must be clean (`go test -race ./...`).
- New middleware: add a `_test.go` next to it; cover the happy path and the failure path.
- New cgo bridge function: add a JSON-contract test in `parse_test.go` even though the cgo call itself is uncovered.

### Three test tiers

1. **Unit tests** (alongside source): pure-Go functions in isolation. `gofmt`, `dateparser`, `redact`, `policy.Resolve`, etc.
2. **Component tests** (e.g. `server/server_test.go`, `mcp/server_test.go`): exercise one package's public API with `httptest.NewRecorder` or the SDK's `NewInMemoryTransports`. Fast, deterministic, no real network.
3. **End-to-end regression tests** — the canonical regression catchers:
   - `server/e2e_test.go` — REST API over a real `http.Client` against an `httptest.NewServer`. Catches wire-format regressions (headers, encoding, status codes).
   - `mcp/e2e_test.go` — MCP client → MCP server → REST server (httptest) → testfakes bridges. Every one of the 24 MCP tools is exercised at least once; lifecycle round-trips for events, reminders, calendars, lists; policy denial; natural-language dates; bad input.

None of these need TCC or real EventKit, so they run on every commit. Add a new tool? Add a case to the corresponding e2e file. The e2e tests are the contract.

For changes to the cgo bridges (which can't be reached by `go test`), exercise via `go run -tags integration ./scripts/integration*.go` after granting TCC.

## Reviewing your own PR

Before requesting review, run through this list:

- [ ] Conventional Commits message
- [ ] gofmt / vet / lint all clean
- [ ] `go test -race ./...` green
- [ ] No new third-party deps without justification
- [ ] Comments explain *why*, not *what*
- [ ] Public API changes have godoc

## Reporting bugs / requesting features

Open a GitHub issue. For security-sensitive reports, see [SECURITY.md](SECURITY.md) — please do not file a public issue for an exploitable vulnerability.

## Operational questions

See [docs/operator-playbook.md](docs/operator-playbook.md) for install / troubleshooting. For the MCP threat model, see [docs/prd/mcp-threats.md](docs/prd/mcp-threats.md).
