// Package server exposes go-eventkit's calendar and reminders bindings as
// a localhost-only REST API with an OpenAPI 3.1 specification.
//
// The package is designed to be the foundation for an MCP (Model Context
// Protocol) server that hands AI agents controlled, allowlisted access to
// the user's macOS Calendar and Reminders. Every request passes through
// multiple independent enforcement layers (defense in depth):
//
//   - The HTTP server binds 127.0.0.1 by default and rejects non-loopback
//     Host headers.
//   - A YAML-driven [policy.Policy] declares which calendars, reminder
//     lists, and account sources the API is allowed to read or write.
//   - The [scoped] package wraps the bridges so every method re-checks
//     the policy on entry and re-verifies the result on exit.
//   - huma performs request validation (required fields, ISO 8601, enums)
//     before any handler runs.
//
// # Quick start
//
//	cal, _ := calendar.New()
//	rem, _ := reminders.New()
//	pol, _ := policy.Load(strings.NewReader(policyYAML))
//
//	srv, err := server.New(
//	    server.WithCalendarBridge(cal),
//	    server.WithRemindersBridge(rem),
//	    server.WithPolicy(pol),
//	)
//	if err != nil { log.Fatal(err) }
//	log.Fatal(http.ListenAndServe("127.0.0.1:8765", srv.Handler()))
//
// See docs/prd/rest-api-prd.md for the full design.
package server
