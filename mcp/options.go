package mcp

import (
	"log/slog"
	"time"
)

// Option configures a [Server] via functional options.
type Option func(*config)

type config struct {
	apiBase        string
	apiTimeout     time.Duration
	logger         *slog.Logger
	rateLimitRPS   float64
	rateLimitBurst int
}

// WithAPIBase sets the URL of the eventkit-server REST API. Defaults to
// http://127.0.0.1:8765.
func WithAPIBase(u string) Option { return func(c *config) { c.apiBase = u } }

// WithAPITimeout caps how long the MCP server waits on a REST call.
// Defaults to 30s.
func WithAPITimeout(d time.Duration) Option { return func(c *config) { c.apiTimeout = d } }

// WithLogger sets the slog logger used by the wrapping HTTP middleware.
// Defaults to slog.Default().
func WithLogger(l *slog.Logger) Option { return func(c *config) { c.logger = l } }

// WithRateLimit caps the inbound request rate. Pass 0 to disable.
// Defaults to 5 rps / 20 burst — appropriate for an LLM that may retry.
func WithRateLimit(rps float64, burst int) Option {
	return func(c *config) { c.rateLimitRPS = rps; c.rateLimitBurst = burst }
}
