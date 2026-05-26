# Operator playbook

End-to-end runbook for installing, configuring, and operating `eventkit-server`. Assumes macOS 14 (Sonoma) or newer; macOS 26 (Tahoe) is the dev target.

## 1. Install

```sh
brew tap dillonbrowne/go-eventkit https://github.com/dillonbrowne/go-eventkit.git
brew install eventkit-server
brew services start eventkit-server
```

The first start pops two macOS Privacy & Security dialogs — Calendar, then Reminders. Click **Allow** on each. The decision is keyed to the binary's path + signature and persists across restarts.

If the dialogs do not appear (the terminal app may have previously denied):

```sh
tccutil reset Calendar
tccutil reset Reminders
brew services restart eventkit-server
```

## 2. First requests

Default port is 8765 (loopback). Confirm it's up:

```sh
curl -s http://127.0.0.1:8765/healthz                       # liveness (always 200)
curl -s http://127.0.0.1:8765/readyz                        # readiness (200 if EventKit reachable, 503 if not)
curl -s http://127.0.0.1:8765/v1/calendars | jq '.calendars[].title'
open http://127.0.0.1:8765/docs                             # Scalar API Reference
```

Without an explicit `--policy`, the server allows access only to **EventKit's default calendar** and **default reminders list** — what Calendar.app and Reminders.app use for new items when no list is picked. This is the safest no-config default for an MCP agent.

## 3. Authoring a policy file

To grant access to additional calendars / lists, write a YAML allowlist. Use the running server to look up identifiers:

```sh
curl -s http://127.0.0.1:8765/v1/calendars | jq '.calendars[] | {id, title, source}'
curl -s http://127.0.0.1:8765/v1/lists     | jq '.lists[]     | {id, title, source}'
```

Pick the targets you want and write `~/Library/Application Support/eventkit-server/policy.yaml`:

```yaml
calendar:
  default: deny
  allow:
    - source: iCloud           # all iCloud cals, both read + write
      mode: readwrite
    - id: 12345678-AAAA-BBBB-CCCC-DEADBEEF0001
      mode: read               # explicit ID can demote (or promote) below source

reminders:
  default: deny
  allow:
    - source: iCloud
      mode: readwrite
```

Precedence: an explicit `id` match beats a `source` match; the most permissive among matching source entries wins for non-matched IDs; if nothing matches, `default` applies.

Point the brew service at it:

```sh
brew services edit eventkit-server
# add `--policy /Users/you/Library/Application\ Support/eventkit-server/policy.yaml` to ProgramArguments
brew services restart eventkit-server
```

## 4. Hot reload (SIGHUP)

The server re-reads the policy file on SIGHUP without dropping in-flight requests:

```sh
kill -HUP $(pgrep -f eventkit-server)
```

Parse errors leave the previous policy in place and log at WARN. The reload log entry includes old vs new entry counts so you can confirm the change took effect.

## 5. MCP-oriented hardening flags

For an MCP agent backed by this server, enable rate limiting and idempotency:

```sh
eventkit-server \
  --policy /Users/you/.../policy.yaml \
  --rate-limit-rps 5 --rate-limit-burst 20 \
  --idempotency-cache 1024 --idempotency-window 24h
```

Rate limiting protects iCloud sync from a runaway agent. Idempotency-Key support lets agent clients retry safely without duplicating events. See [`docs/prd/mcp-threats.md`](prd/mcp-threats.md).

## 5a. MCP server (AI-native)

`eventkit-mcp` is a separate binary that exposes the REST API as a [Streamable HTTP MCP](https://modelcontextprotocol.io/) server. AI clients (Claude Desktop, Continue, Cline locally; Claude web, Claude mobile, ChatGPT custom connectors via tunnel) call 24 curated tools that translate to REST requests on `127.0.0.1:8765`.

### Install + run

```sh
brew install eventkit-mcp        # depends on eventkit-server
brew services start eventkit-mcp
```

It listens on `127.0.0.1:8766` with **no auth**. Verify:

```sh
curl -s -H "Content-Type: application/json" \
     http://127.0.0.1:8766/mcp \
     -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | jq '.result.tools[].name'
# → 24 tool names (list_calendars, create_event, complete_reminder, …)
```

### Local clients

In `~/Library/Application Support/Claude/claude_desktop_config.json` (or the equivalent for your client):

```json
{"mcpServers": {"eventkit": {"url": "http://127.0.0.1:8766"}}}
```

### Remote clients (cloud LLMs)

For Claude web / Claude mobile / ChatGPT custom connectors, you need a public URL. The MCP server has **no auth**, so put a tunnel + identity provider in front. We don't prescribe a specific path — Cloudflare Tunnel + Cloudflare Access, Tailscale Funnel, ngrok with auth, a self-hosted reverse proxy + OAuth, etc. all work. The MCP server only ever sees loopback traffic; the perimeter is the operator's responsibility.

### Natural-language dates

Every tool that accepts a date accepts either RFC 3339 (`2026-05-26T15:00:00Z`) or natural language (`tomorrow 2pm`, `next friday`, `eod`, `in 2 hours`). The MCP server converts to RFC 3339 via the `dateparser` package before forwarding to the REST API.

### Prompt-injection mitigation

User-controlled string fields (event titles, notes, attendee names/emails, reminder bodies) are wrapped in `<USER_DATA>…</USER_DATA>` delimiters and truncated to 512 chars when returned to the MCP client. The wrapping is a hint to the LLM that the enclosed text is untrusted and must not be interpreted as instructions. See [`docs/prd/mcp-threats.md`](prd/mcp-threats.md).

## 6. Troubleshooting

### `EventKit access not granted` (503 on /readyz)

TCC permission was revoked or never granted. Fix:

```sh
open "x-apple.systempreferences:com.apple.preference.security?Privacy_Calendars"
# toggle the eventkit-server entry on; same for Reminders
brew services restart eventkit-server
```

### `post-fetch scope violation` (422 on a write)

The write succeeded in EventKit but landed in a different calendar / list than the one the request named. This usually means the policy is misconfigured or EventKit silently fell back to the default. The 422 response body lists `orphan.id`, `orphan.container_id`, `intended.container_id` — open Calendar.app or Reminders.app, navigate to the orphan's container, and either delete the unintended object or update the policy so the orphan's container is intentionally writable.

### `429 Too Many Requests`

The agent exceeded `--rate-limit-rps`. The response includes `Retry-After: N` seconds. Honor it (back off exponentially) and consider whether the agent's retry loop is well-behaved.

### Audit log inspection

```sh
tail -f $(brew --prefix)/var/log/eventkit-server.log | jq 'select(.msg == "audit")'
```

Each audit line includes `method`, `path`, `status`, `target_id`, `request_id`, `elapsed`, and `policy_denied` (when applicable). For incident investigation, search by `request_id` to follow one logical request across the audit + any handler-level logs.

## 7. Upgrade

```sh
brew update
brew upgrade eventkit-server
brew services restart eventkit-server
```

If the new version invalidates the binary's code signature, macOS may re-prompt for Calendar / Reminders access on the next start. Click Allow again.

## 8. Uninstall

```sh
brew services stop eventkit-server
brew uninstall eventkit-server
brew untap dillonbrowne/go-eventkit
# Optionally revoke TCC entries (they remain even after uninstall):
tccutil reset Calendar
tccutil reset Reminders
```

## See also

- [`docs/prd/rest-api-prd.md`](prd/rest-api-prd.md) — defense-in-depth design, error mapping table.
- [`docs/prd/mcp-threats.md`](prd/mcp-threats.md) — risks specific to MCP / AI-agent use.
- [`Formula/eventkit-server.rb`](../Formula/eventkit-server.rb) — Homebrew formula source.
