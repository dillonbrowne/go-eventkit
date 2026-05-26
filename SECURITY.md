# Security policy

## Scope

`go-eventkit` and its companion REST server / `bundle-app` are intended to run on a user's own macOS machine, bound to `127.0.0.1`. The threat model assumes:

- The operator trusts the macOS system, the Homebrew tap, and the binary they installed.
- The operator may run an AI agent (via an MCP wrapper or other client) against the REST API.
- The operator does **not** expose the server to the public internet. If you bind it to a non-loopback address via `--insecure-bind`, you opt out of the perimeter and you are on your own.

Reports out of scope:

- Anything that requires already having admin access to the machine.
- Anything that requires having `127.0.0.1` access (you are the user; you can read your own calendars).
- DoS via `--rate-limit-*` not being aggressive enough — that's a tuning question, not a bug.

In scope:

- Bypass of the YAML allowlist policy (a request reaches a calendar / list that is not in the allowlist).
- Code execution through a request body (e.g. via a crafted recurrence rule or attendee field).
- Information disclosure beyond what the allowlist permits (e.g. `/openapi.json` leaking unrelated data).
- Issues in the cgo bridge that allow EventKit operations the policy should have refused.
- Supply-chain issues in the Homebrew formula, the `bundle-app` tool, or the GitHub release pipeline.

The [REST PRD](docs/prd/rest-api-prd.md) lists the defense-in-depth layers; the [MCP threat doc](docs/prd/mcp-threats.md) lists known limitations relevant to AI-agent use.

## Reporting

Please report vulnerabilities **privately** via GitHub Security Advisories:

> https://github.com/dillonbrowne/go-eventkit/security/advisories/new

Do not open a regular public issue for an exploitable bug.

### What to include

- A short description of the impact.
- A minimal reproduction (curl command, policy file, request body — whatever applies).
- The version (`eventkit-server --version`) or commit SHA.
- Your macOS version (`sw_vers`) — TCC behavior differs across releases.

### What to expect

- Acknowledgement within 7 days.
- A first-pass triage within 14 days.
- A fix or mitigation plan within 30 days for confirmed issues. Larger refactors may take longer.

There is no bug bounty. Public credit in the release notes if you'd like it.

## Known limitations (not vulnerabilities)

These are intentional trade-offs documented elsewhere:

- **Ad-hoc code signing** means macOS may re-prompt for TCC after a reinstall. A Developer ID signature would fix this; we don't have one yet.
- **Loopback-only by default**. Binding non-loopback is a documented operator choice and removes a layer of defense.
- **No auth** on the REST API. The perimeter is the loopback bind + the policy file. An MCP wrapper can add per-tool auth on top.
- **Policy denial returns 404, not 403**. Deliberate, to avoid leaking the existence of denied resources to a probing client.
- **Audit log writes to stderr** by default. Redirect via the launchd plist; see the operator playbook.
