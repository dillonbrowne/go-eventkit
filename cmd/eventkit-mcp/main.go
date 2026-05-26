// Command eventkit-mcp serves the eventkit REST API as a Streamable
// HTTP MCP server. Its only job is to translate tool calls from MCP
// clients (Claude Desktop, ChatGPT, Claude web/mobile, Continue, Cline,
// etc.) into REST requests against a running eventkit-server.
//
// Configuration precedence (highest wins): explicit flag → env var →
// built-in default. Every flag has an EVENTKIT_MCP_<NAME> env-var
// counterpart documented in its help text.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/dillonbrowne/go-eventkit/internal/version"
	"github.com/dillonbrowne/go-eventkit/mcp"
)

func main() {
	cfg, err := parseConfig(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "eventkit-mcp:", err)
		os.Exit(2)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.logLevel}))

	if err := run(cfg, logger); err != nil {
		logger.Error("server failed", slog.Any("err", err))
		os.Exit(1)
	}
}

type binaryConfig struct {
	listen         string
	apiBase        string
	apiTimeout     time.Duration
	insecureBind   bool
	logLevel       slog.Level
	rateLimitRPS   float64
	rateLimitBurst int
}

func parseConfig(args []string) (*binaryConfig, error) {
	fs := flag.NewFlagSet("eventkit-mcp", flag.ContinueOnError)

	listen := fs.String("listen", "", "host:port to bind. Loopback only unless --insecure-bind. Env: EVENTKIT_MCP_LISTEN. Default: 127.0.0.1:8766.")
	apiBase := fs.String("api-base", "", "URL of the eventkit-server REST API. Env: EVENTKIT_MCP_API_BASE. Default: http://127.0.0.1:8765.")
	apiTimeout := fs.Duration("api-timeout", 0, "HTTP timeout for REST calls. Env: EVENTKIT_MCP_API_TIMEOUT. Default: 30s.")
	insecureBind := fs.Bool("insecure-bind", false, "Permit binding to a non-loopback address. Env: EVENTKIT_MCP_INSECURE_BIND=1. WARNING: removes the localhost-only perimeter.")
	logLevelStr := fs.String("log-level", "", "Log level: debug|info|warn|error. Env: EVENTKIT_MCP_LOG_LEVEL. Default: info.")
	rateLimitRPS := fs.Float64("rate-limit-rps", -1, "Token-bucket refill rate. Env: EVENTKIT_MCP_RATE_LIMIT_RPS. Default: 5.")
	rateLimitBurst := fs.Int("rate-limit-burst", -1, "Token-bucket burst capacity. Env: EVENTKIT_MCP_RATE_LIMIT_BURST. Default: 20.")
	showVersion := fs.Bool("version", false, "Print version and exit.")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if *showVersion {
		fmt.Println(version.Version)
		os.Exit(0)
	}

	cfg := &binaryConfig{
		listen:       firstNonEmpty(*listen, os.Getenv("EVENTKIT_MCP_LISTEN"), "127.0.0.1:8766"),
		apiBase:      firstNonEmpty(*apiBase, os.Getenv("EVENTKIT_MCP_API_BASE"), "http://127.0.0.1:8765"),
		insecureBind: *insecureBind || envBool("EVENTKIT_MCP_INSECURE_BIND"),
	}

	var err error
	if cfg.apiTimeout, err = durationOr(*apiTimeout, "EVENTKIT_MCP_API_TIMEOUT", 30*time.Second); err != nil {
		return nil, fmt.Errorf("--api-timeout: %w", err)
	}
	if cfg.logLevel, err = parseLogLevel(firstNonEmpty(*logLevelStr, os.Getenv("EVENTKIT_MCP_LOG_LEVEL"), "info")); err != nil {
		return nil, fmt.Errorf("--log-level: %w", err)
	}
	if cfg.rateLimitRPS, err = float64Or(*rateLimitRPS, "EVENTKIT_MCP_RATE_LIMIT_RPS", 5); err != nil {
		return nil, fmt.Errorf("--rate-limit-rps: %w", err)
	}
	if cfg.rateLimitBurst, err = intOr(*rateLimitBurst, "EVENTKIT_MCP_RATE_LIMIT_BURST", 20); err != nil {
		return nil, fmt.Errorf("--rate-limit-burst: %w", err)
	}
	return cfg, nil
}

func run(cfg *binaryConfig, logger *slog.Logger) error {
	if !cfg.insecureBind {
		if err := enforceLoopbackBind(cfg.listen); err != nil {
			return fmt.Errorf("--listen %q is not loopback. Pass --insecure-bind to override: %w", cfg.listen, err)
		}
	} else {
		logger.Warn("--insecure-bind set: server will accept non-loopback connections.")
	}

	srv := mcp.New(
		mcp.WithAPIBase(cfg.apiBase),
		mcp.WithAPITimeout(cfg.apiTimeout),
		mcp.WithLogger(logger),
		mcp.WithRateLimit(cfg.rateLimitRPS, cfg.rateLimitBurst),
	)

	httpSrv := &http.Server{
		Addr:              cfg.listen,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		logger.Info("shutdown signal received; closing")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutdownCtx)
	}()

	logger.Info("listening",
		slog.String("addr", cfg.listen),
		slog.String("api_base", cfg.apiBase),
		slog.String("version", version.Version),
	)
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// ---- flag / env-var helpers ----

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

func intOr(flagVal int, envVar string, def int) (int, error) {
	if flagVal >= 0 {
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
	if flagVal >= 0 {
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
