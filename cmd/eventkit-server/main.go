// Command eventkit-server serves a localhost REST API over go-eventkit.
//
// Defense-in-depth in this binary:
//
//   - Default bind is 127.0.0.1:8765. Non-loopback bind is refused unless
//     the operator passes --insecure-bind (and is warned loudly).
//   - A policy YAML file is optional. Without one, the server allows only
//     EventKit's default calendar and default reminders list.
//   - SIGHUP re-parses the policy file and atomically swaps it in without
//     dropping in-flight requests.
//   - SIGINT/SIGTERM triggers a 5s graceful drain.
//
// Configuration precedence (highest wins): explicit flag → environment
// variable → built-in default. Every flag has an EVENTKIT_<NAME> env-var
// counterpart documented in the flag's help text.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/dillonbrowne/go-eventkit/calendar"
	"github.com/dillonbrowne/go-eventkit/internal/version"
	"github.com/dillonbrowne/go-eventkit/reminders"
	"github.com/dillonbrowne/go-eventkit/server"
	"github.com/dillonbrowne/go-eventkit/server/policy"
)

func main() {
	cfg, err := parseConfig(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "eventkit-server:", err)
		os.Exit(2)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.logLevel}))

	if err := run(cfg, logger); err != nil {
		logger.Error("server failed", slog.Any("err", err))
		os.Exit(1)
	}
}

type binaryConfig struct {
	listen           string
	policyPath       string
	insecureBind     bool
	readTimeout      time.Duration
	writeTimeout     time.Duration
	bodyLimit        int64
	requestTimeout   time.Duration
	rateLimitRPS     float64
	rateLimitBurst   int
	idempotencyTTL   time.Duration
	idempotencyCache int
	logLevel         slog.Level
	apiTitle         string
	apiVersion       string
	publicURL        string
}

// flagEnv chains a CLI flag value, an env-var fallback, and a default.
// The returned string is what should be parsed by the caller into its
// concrete type. Empty string means the default applies.
func flagEnv(flagVal, envVar string) string {
	if flagVal != "" {
		return flagVal
	}
	if envVar != "" {
		return os.Getenv(envVar)
	}
	return ""
}

func parseConfig(args []string) (*binaryConfig, error) {
	fs := flag.NewFlagSet("eventkit-server", flag.ContinueOnError)

	listen := fs.String("listen", "", "host:port to bind. Loopback only unless --insecure-bind. Env: EVENTKIT_LISTEN. Default: 127.0.0.1:8765.")
	policyPath := fs.String("policy", "", "Path to the YAML allowlist policy. Optional — if omitted, only EventKit's default calendar and list are accessible. Env: EVENTKIT_POLICY.")
	insecureBindFlag := fs.Bool("insecure-bind", false, "Permit binding to a non-loopback address. Env: EVENTKIT_INSECURE_BIND=1. WARNING: removes the localhost-only perimeter.")
	readTimeout := fs.Duration("read-timeout", 0, "HTTP server read timeout. Env: EVENTKIT_READ_TIMEOUT. Default: 15s.")
	writeTimeout := fs.Duration("write-timeout", 0, "HTTP server write timeout. Env: EVENTKIT_WRITE_TIMEOUT. Default: 15s.")
	bodyLimit := fs.Int64("body-limit", 0, "Max request body bytes. Env: EVENTKIT_BODY_LIMIT. Default: 1 MiB.")
	requestTimeout := fs.Duration("request-timeout", 0, "Per-request handler timeout (separate from R/W). Env: EVENTKIT_REQUEST_TIMEOUT. Default: 30s.")
	rateLimitRPS := fs.Float64("rate-limit-rps", 0, "Global token-bucket refill rate. Env: EVENTKIT_RATE_LIMIT_RPS. 0 disables (default).")
	rateLimitBurst := fs.Int("rate-limit-burst", 0, "Global token-bucket burst capacity. Env: EVENTKIT_RATE_LIMIT_BURST. 0 disables (default).")
	idempotencyTTL := fs.Duration("idempotency-window", 0, "Idempotency-Key cache window. Env: EVENTKIT_IDEMPOTENCY_WINDOW. Default: 24h when caching is enabled.")
	idempotencyCache := fs.Int("idempotency-cache", 0, "Idempotency-Key cache capacity. Env: EVENTKIT_IDEMPOTENCY_CACHE. 0 disables (default).")
	logLevelStr := fs.String("log-level", "", "Log level: debug|info|warn|error. Env: EVENTKIT_LOG_LEVEL. Default: info.")
	apiTitle := fs.String("api-title", "", "OpenAPI document title. Env: EVENTKIT_API_TITLE.")
	apiVersion := fs.String("api-version", "", "OpenAPI document version. Env: EVENTKIT_API_VERSION. Default: build-injected version (currently \""+version.Version+"\").")
	publicURL := fs.String("public-url", "", "Absolute public base URL (e.g. https://host) the API is reached at through a proxy/tunnel. Sets OpenAPI servers[0].url — required by the GPT Actions builder. Env: EVENTKIT_PUBLIC_URL.")
	showVersion := fs.Bool("version", false, "Print version and exit.")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if *showVersion {
		fmt.Println(version.Version)
		os.Exit(0)
	}

	cfg := &binaryConfig{
		listen:       firstNonEmpty(*listen, os.Getenv("EVENTKIT_LISTEN"), "127.0.0.1:8765"),
		policyPath:   firstNonEmpty(*policyPath, os.Getenv("EVENTKIT_POLICY"), ""),
		insecureBind: *insecureBindFlag || envBool("EVENTKIT_INSECURE_BIND"),
		apiTitle:     firstNonEmpty(*apiTitle, os.Getenv("EVENTKIT_API_TITLE"), ""),
		apiVersion:   firstNonEmpty(*apiVersion, os.Getenv("EVENTKIT_API_VERSION"), version.Version),
		publicURL:    firstNonEmpty(*publicURL, os.Getenv("EVENTKIT_PUBLIC_URL"), ""),
	}

	if cfg.publicURL != "" {
		if err := validatePublicURL(cfg.publicURL); err != nil {
			return nil, fmt.Errorf("--public-url: %w", err)
		}
	}

	var err error
	if cfg.readTimeout, err = durationOr(*readTimeout, "EVENTKIT_READ_TIMEOUT", 15*time.Second); err != nil {
		return nil, fmt.Errorf("--read-timeout: %w", err)
	}
	if cfg.writeTimeout, err = durationOr(*writeTimeout, "EVENTKIT_WRITE_TIMEOUT", 15*time.Second); err != nil {
		return nil, fmt.Errorf("--write-timeout: %w", err)
	}
	if cfg.requestTimeout, err = durationOr(*requestTimeout, "EVENTKIT_REQUEST_TIMEOUT", 30*time.Second); err != nil {
		return nil, fmt.Errorf("--request-timeout: %w", err)
	}
	if cfg.bodyLimit, err = int64Or(*bodyLimit, "EVENTKIT_BODY_LIMIT", 1<<20); err != nil {
		return nil, fmt.Errorf("--body-limit: %w", err)
	}
	if cfg.rateLimitRPS, err = float64Or(*rateLimitRPS, "EVENTKIT_RATE_LIMIT_RPS", 0); err != nil {
		return nil, fmt.Errorf("--rate-limit-rps: %w", err)
	}
	if cfg.rateLimitBurst, err = intOr(*rateLimitBurst, "EVENTKIT_RATE_LIMIT_BURST", 0); err != nil {
		return nil, fmt.Errorf("--rate-limit-burst: %w", err)
	}
	if cfg.idempotencyTTL, err = durationOr(*idempotencyTTL, "EVENTKIT_IDEMPOTENCY_WINDOW", 0); err != nil {
		return nil, fmt.Errorf("--idempotency-window: %w", err)
	}
	if cfg.idempotencyCache, err = intOr(*idempotencyCache, "EVENTKIT_IDEMPOTENCY_CACHE", 0); err != nil {
		return nil, fmt.Errorf("--idempotency-cache: %w", err)
	}
	if cfg.logLevel, err = parseLogLevel(flagEnv(*logLevelStr, "EVENTKIT_LOG_LEVEL")); err != nil {
		return nil, fmt.Errorf("--log-level: %w", err)
	}
	return cfg, nil
}

func intOr(flagVal int, envVar string, def int) (int, error) {
	if flagVal > 0 {
		return flagVal, nil
	}
	if s := os.Getenv(envVar); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil {
			return 0, fmt.Errorf("env %s=%q: %w", envVar, s, err)
		}
		return n, nil
	}
	return def, nil
}

func float64Or(flagVal float64, envVar string, def float64) (float64, error) {
	if flagVal > 0 {
		return flagVal, nil
	}
	if s := os.Getenv(envVar); s != "" {
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return 0, fmt.Errorf("env %s=%q: %w", envVar, s, err)
		}
		return f, nil
	}
	return def, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func envBool(name string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func durationOr(flagVal time.Duration, envVar string, def time.Duration) (time.Duration, error) {
	if flagVal > 0 {
		return flagVal, nil
	}
	if s := os.Getenv(envVar); s != "" {
		d, err := time.ParseDuration(s)
		if err != nil {
			return 0, fmt.Errorf("env %s=%q: %w", envVar, s, err)
		}
		return d, nil
	}
	return def, nil
}

func int64Or(flagVal int64, envVar string, def int64) (int64, error) {
	if flagVal > 0 {
		return flagVal, nil
	}
	if s := os.Getenv(envVar); s != "" {
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("env %s=%q: %w", envVar, s, err)
		}
		return n, nil
	}
	return def, nil
}

func parseLogLevel(s string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error", "err":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("unknown level %q (want debug, info, warn, or error)", s)
	}
}

func run(cfg *binaryConfig, logger *slog.Logger) error {
	if !cfg.insecureBind {
		if err := enforceLoopbackBind(cfg.listen); err != nil {
			return fmt.Errorf("--listen %q is not loopback. Pass --insecure-bind to override (DEFENSE-IN-DEPTH WILL BE WEAKENED): %w", cfg.listen, err)
		}
	} else {
		logger.Warn("--insecure-bind set: server will accept non-loopback connections. The loopback middleware still rejects non-loopback Host headers, but you have removed one layer of defense.")
	}

	pol, err := loadPolicy(cfg.policyPath)
	if err != nil {
		return err
	}

	calClient, err := calendar.New()
	if err != nil {
		return fmt.Errorf("connect calendar bridge: %w", err)
	}
	remClient, err := reminders.New()
	if err != nil {
		return fmt.Errorf("connect reminders bridge: %w", err)
	}

	opts := []server.Option{
		server.WithCalendarBridge(calClient),
		server.WithRemindersBridge(remClient),
		server.WithLogger(logger),
		server.WithBodyLimit(cfg.bodyLimit),
		server.WithRequestTimeout(cfg.requestTimeout),
		server.WithRateLimit(cfg.rateLimitRPS, cfg.rateLimitBurst),
		server.WithIdempotency(cfg.idempotencyTTL, cfg.idempotencyCache),
	}
	if pol != nil {
		opts = append(opts, server.WithPolicy(pol))
	}
	if cfg.apiTitle != "" {
		opts = append(opts, server.WithAPITitle(cfg.apiTitle))
	}
	if cfg.publicURL != "" {
		opts = append(opts, server.WithPublicURL(cfg.publicURL))
	}
	opts = append(opts, server.WithAPIVersion(cfg.apiVersion))
	srv, err := server.New(opts...)
	if err != nil {
		return err
	}

	httpSrv := &http.Server{
		Addr:         cfg.listen,
		Handler:      srv.Handler(),
		ReadTimeout:  cfg.readTimeout,
		WriteTimeout: cfg.writeTimeout,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// SIGHUP — reload policy without dropping requests.
	go watchReloads(srv, cfg.policyPath, logger)

	// Graceful shutdown on SIGINT/SIGTERM.
	go func() {
		<-ctx.Done()
		logger.Info("shutdown signal received; closing")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutdownCtx)
	}()

	logger.Info("listening",
		slog.String("addr", cfg.listen),
		slog.String("policy", cfg.policyPath),
		slog.String("version", version.Version),
		slog.String("level", cfg.logLevel.String()),
	)
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// loadPolicy parses the YAML policy file at path. Returns (nil, nil) if
// path is empty — the caller will let server.New install the implicit
// safe default (only the system default calendar / list).
func loadPolicy(path string) (*policy.Policy, error) {
	if path == "" {
		return nil, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open policy file: %w", err)
	}
	defer f.Close()
	pol, err := policy.Load(f)
	if err != nil {
		return nil, fmt.Errorf("parse policy: %w", err)
	}
	return pol, nil
}

// watchReloads installs a SIGHUP handler that re-reads the policy file
// and swaps it in atomically. Parse errors leave the previous policy in
// place and are logged at WARN level.
func watchReloads(srv *server.Server, policyPath string, logger *slog.Logger) {
	if policyPath == "" {
		return // nothing to reload
	}
	hupCh := make(chan os.Signal, 1)
	signal.Notify(hupCh, syscall.SIGHUP)
	for range hupCh {
		pol, err := loadPolicy(policyPath)
		if err != nil {
			logger.Warn("policy reload failed; keeping previous policy",
				slog.String("policy", policyPath),
				slog.Any("err", err),
			)
			continue
		}
		srv.ReplacePolicy(pol)
		logger.Info("policy reloaded",
			slog.String("policy", policyPath),
			slog.Int("calendar_entries", len(pol.Calendar.Entries)),
			slog.Int("reminder_entries", len(pol.Reminders.Entries)),
		)
	}
}

// validatePublicURL requires an absolute http/https URL with a host and no
// path/query/fragment — it becomes the OpenAPI servers[0].url, so a bare
// origin is what consumers expect.
func validatePublicURL(s string) error {
	u, err := url.Parse(s)
	if err != nil {
		return fmt.Errorf("parse %q: %w", s, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%q must start with http:// or https://", s)
	}
	if u.Host == "" {
		return fmt.Errorf("%q has no host", s)
	}
	if u.Path != "" && u.Path != "/" {
		return fmt.Errorf("%q must be a bare origin (no path); got path %q", s, u.Path)
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("%q must not include a query or fragment", s)
	}
	return nil
}

func enforceLoopbackBind(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("parse address: %w", err)
	}
	host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	switch strings.ToLower(host) {
	case "", "localhost", "127.0.0.1", "::1":
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("%q is not a recognized loopback host", host)
	}
	if !ip.IsLoopback() {
		return fmt.Errorf("%s is not in the loopback range", host)
	}
	return nil
}
