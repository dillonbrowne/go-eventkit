package server

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"

	"github.com/dillonbrowne/go-eventkit/internal/version"
	"github.com/dillonbrowne/go-eventkit/server/handlers"
	"github.com/dillonbrowne/go-eventkit/server/middleware"
	"github.com/dillonbrowne/go-eventkit/server/policy"
	"github.com/dillonbrowne/go-eventkit/server/scoped"
)

// ErrMissingBridges is returned by [New] when either the calendar or
// reminders bridge is nil.
var ErrMissingBridges = errors.New("server: calendar and reminders bridges are required")

// Server wires the chi router, huma API, scoped wrappers, middleware,
// and handlers into a single http.Handler.
type Server struct {
	handler http.Handler
	api     huma.API
	scCal   *scoped.ScopedCalendar
	scRem   *scoped.ScopedReminders
}

// ReplacePolicy atomically swaps the policy referenced by the scoped
// wrappers. Used by the binary's SIGHUP handler for hot-reload — readers
// see the new policy on their next call without taking any lock.
func (s *Server) ReplacePolicy(p *policy.Policy) {
	s.scCal.ReplacePolicy(p)
	s.scRem.ReplacePolicy(p)
}

// New builds a Server. Pass at minimum the policy and both bridges.
func New(opts ...Option) (*Server, error) {
	cfg := &config{
		bodyLimit: middleware.DefaultBodyLimit,
		apiTitle:  "go-eventkit REST API",
		version:   version.Version,
	}
	for _, opt := range opts {
		opt(cfg)
	}
	if cfg.cal == nil || cfg.rem == nil {
		return nil, ErrMissingBridges
	}
	if cfg.pol == nil {
		cfg.pol = defaultPolicy(cfg.cal, cfg.rem, cfg.logger)
	}

	scCal := scoped.NewCalendar(scopedCalendarAdapter{cfg.cal}, cfg.pol)
	scRem := scoped.NewReminders(scopedRemindersAdapter{cfg.rem}, cfg.pol)

	calHandler := &handlers.ScopedCalendarHandler{S: scCal}
	remHandler := &handlers.ScopedRemindersHandler{S: scRem}

	router := chi.NewRouter()
	// Order matters. The chain is read outside-in:
	//   Recover wraps everything so any panic anywhere becomes a 500.
	//   StripServerHeader runs *inside* Recover so its wrapped writer
	//   reaches each handler.
	//   Loopback rejects non-loopback traffic before any work happens.
	//   RequestID + RejectMethodOverride + RequestTimeout shape the
	//   request before Audit observes it.
	//   BodyLimit + RequireContentType protect the body decode path.
	//   Audit logs the final outcome.
	router.Use(middleware.Recover(cfg.logger))
	router.Use(middleware.StripServerHeader)
	router.Use(middleware.Loopback)
	router.Use(middleware.RequestID)
	router.Use(middleware.RejectMethodOverride)
	router.Use(middleware.RateLimit(cfg.rateLimitRPS, cfg.rateLimitBurst))
	router.Use(middleware.RequestTimeout(cfg.requestTimeout))
	router.Use(middleware.BodyLimit(cfg.bodyLimit))
	router.Use(middleware.RequireContentType("application/json"))
	router.Use(middleware.Idempotency(cfg.idempotencyTTL, cfg.idempotencyCache))
	router.Use(middleware.Audit(cfg.logger))

	api := humachi.New(router, newHumaConfig(cfg))
	handlers.Register(api, calHandler, remHandler)

	return &Server{handler: router, api: api, scCal: scCal, scRem: scRem}, nil
}

// Handler returns the http.Handler ready to serve.
func (s *Server) Handler() http.Handler { return s.handler }

// API returns the underlying huma.API. Useful in tests for
// [humatest.New] and OpenAPI spec walking.
func (s *Server) API() huma.API { return s.api }

func newHumaConfig(cfg *config) huma.Config {
	c := huma.DefaultConfig(cfg.apiTitle, cfg.version)
	c.Info.Description = "Localhost REST API over go-eventkit. Foundation for an MCP server. " +
		"Access is gated by a YAML allowlist policy enforced at every read and write."
	// Scalar API Reference (https://scalar.com/) for the /docs UI. huma
	// bundles first-party renderers with integrity-hashed CDN references —
	// no third-party Go shim or custom HTML needed.
	c.DocsRenderer = huma.DocsRendererScalar
	return c
}

// Used to bridge the local interface declarations into scoped's. The
// scoped package redeclares the interfaces to avoid an import cycle; the
// adapters below let server callers pass any value that satisfies the
// server-level interface.
type scopedCalendarAdapter struct{ CalendarBridge }
type scopedRemindersAdapter struct{ RemindersBridge }

// defaultPolicy resolves the implicit allowlist used when WithPolicy is
// not supplied. It asks the bridge for the default calendar / list and
// allows only those — read+write. If either bridge cannot report a
// default (no writable target, or the bridge returns an error), the
// corresponding package falls back to deny-all. This keeps the no-policy
// experience safe: the API only writes where EventKit would have
// written by default, never to every calendar in every account.
func defaultPolicy(cal CalendarBridge, rem RemindersBridge, logger *slog.Logger) *policy.Policy {
	pol := policy.DenyAll()

	if defCal, err := cal.DefaultCalendar(); err == nil && defCal != nil {
		pol.Calendar.Entries = append(pol.Calendar.Entries, policy.Entry{
			ID:   defCal.ID,
			Mode: policy.ModeReadWrite,
		})
		if logger != nil {
			logger.Warn(
				"no policy supplied; restricting calendar access to the system default",
				slog.String("calendar.id", defCal.ID),
				slog.String("calendar.title", defCal.Title),
				slog.String("calendar.source", defCal.Source),
			)
		}
	} else if logger != nil {
		logger.Warn("no policy supplied and no default calendar available; calendar access is denied", slog.Any("err", err))
	}

	if defList, err := rem.DefaultList(); err == nil && defList != nil {
		pol.Reminders.Entries = append(pol.Reminders.Entries, policy.Entry{
			ID:   defList.ID,
			Mode: policy.ModeReadWrite,
		})
		if logger != nil {
			logger.Warn(
				"no policy supplied; restricting reminders access to the system default list",
				slog.String("list.id", defList.ID),
				slog.String("list.title", defList.Title),
				slog.String("list.source", defList.Source),
			)
		}
	} else if logger != nil {
		logger.Warn("no policy supplied and no default reminders list available; reminders access is denied", slog.Any("err", err))
	}

	return pol
}
