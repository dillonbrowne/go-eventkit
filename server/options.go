package server

import (
	"log/slog"
	"time"

	"github.com/dillonbrowne/go-eventkit/server/policy"
)

// Option configures the [Server] via the functional-options pattern.
type Option func(*config)

type config struct {
	cal              CalendarBridge
	rem              RemindersBridge
	pol              *policy.Policy
	logger           *slog.Logger
	bodyLimit        int64
	requestTimeout   time.Duration
	rateLimitRPS     float64
	rateLimitBurst   int
	idempotencyTTL   time.Duration
	idempotencyCache int
	apiTitle         string
	version          string
	publicURL        string
}

// WithCalendarBridge injects the bridge used to talk to EventKit calendars.
// Pass *calendar.Client in production, or a fake in tests.
func WithCalendarBridge(b CalendarBridge) Option { return func(c *config) { c.cal = b } }

// WithRemindersBridge injects the bridge used to talk to EventKit reminders.
func WithRemindersBridge(b RemindersBridge) Option { return func(c *config) { c.rem = b } }

// WithPolicy injects the allowlist policy. A policy is required — New
// returns an error if none is set.
func WithPolicy(p *policy.Policy) Option { return func(c *config) { c.pol = p } }

// WithLogger sets the slog logger used by the audit middleware. Defaults
// to slog.Default().
func WithLogger(l *slog.Logger) Option { return func(c *config) { c.logger = l } }

// WithBodyLimit overrides the per-request body cap (default 1 MiB).
// Pass 0 to keep the default.
func WithBodyLimit(n int64) Option { return func(c *config) { c.bodyLimit = n } }

// WithRequestTimeout overrides the per-request handler timeout (default
// 30s). Bridge calls that thread the context observe cancellation.
func WithRequestTimeout(d time.Duration) Option { return func(c *config) { c.requestTimeout = d } }

// WithAPITitle overrides the OpenAPI document title.
func WithAPITitle(s string) Option { return func(c *config) { c.apiTitle = s } }

// WithAPIVersion overrides the OpenAPI document version.
func WithAPIVersion(s string) Option { return func(c *config) { c.version = s } }

// WithPublicURL sets the absolute base URL the API is reached at from
// outside (e.g. through a reverse proxy or tunnel). When set, it becomes
// the OpenAPI `servers[0].url`. Tools that require a server URL — notably
// the ChatGPT / Custom GPT Actions builder — reject a spec without one, so
// set this whenever the server is exposed under a public hostname. Leave
// empty for the default loopback-only deployment (no servers block).
func WithPublicURL(s string) Option { return func(c *config) { c.publicURL = s } }

// WithRateLimit enables a global token-bucket rate limit. Pass rps=0 to
// disable (the default).
func WithRateLimit(rps float64, burst int) Option {
	return func(c *config) { c.rateLimitRPS = rps; c.rateLimitBurst = burst }
}

// WithIdempotency enables in-memory caching of POST/PATCH/DELETE
// responses keyed by the inbound Idempotency-Key header. Pass cache=0
// to disable (the default); ttl=0 falls back to a 24h window inside the
// middleware.
func WithIdempotency(ttl time.Duration, cache int) Option {
	return func(c *config) { c.idempotencyTTL = ttl; c.idempotencyCache = cache }
}
