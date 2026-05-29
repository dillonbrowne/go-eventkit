package mcp

import (
	"log/slog"
	"net/http"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dillonbrowne/go-eventkit/internal/version"
	"github.com/dillonbrowne/go-eventkit/mcp/client"
	"github.com/dillonbrowne/go-eventkit/mcp/tools"
	"github.com/dillonbrowne/go-eventkit/server/middleware"
)

// Server is a Streamable HTTP MCP server that translates each tool call
// into a REST request against eventkit-server.
type Server struct {
	handler http.Handler
	mcp     *sdkmcp.Server
	client  *client.Client
	cfg     *config
}

// New constructs the MCP server and its HTTP handler.
func New(opts ...Option) *Server {
	cfg := &config{
		apiBase:        "http://127.0.0.1:8765",
		apiTimeout:     30 * time.Second,
		rateLimitRPS:   5,
		rateLimitBurst: 20,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	c := client.New(cfg.apiBase, cfg.apiTimeout)

	mcpServer := sdkmcp.NewServer(
		&sdkmcp.Implementation{Name: "eventkit-mcp", Version: version.Version},
		&sdkmcp.ServerOptions{
			Instructions: "Tools for the user's macOS Calendar and Reminders. " + tools.UserDataNotice,
		},
	)
	tools.Register(mcpServer, c)

	streamable := sdkmcp.NewStreamableHTTPHandler(func(*http.Request) *sdkmcp.Server {
		return mcpServer
	}, &sdkmcp.StreamableHTTPOptions{
		// The SDK auto-enables DNS-rebinding protection for loopback-bound
		// servers: it 403s any request whose Host header isn't localhost.
		// This server is *designed* to be reached through an operator-managed
		// tunnel (Cloudflare Tunnel, Tailscale Funnel, etc.), where the
		// inbound Host is the public hostname — so that check rejects every
		// legitimate tunneled request. We disable it; the perimeter is the
		// loopback bind plus whatever identity layer the operator fronts the
		// tunnel with. See docs/operator-playbook.md.
		DisableLocalhostProtection: true,
	})

	// Outer middleware chain — reusable, transport-agnostic pieces from
	// server/middleware. Order matters; see server.go for the rationale.
	h := http.Handler(streamable)
	h = middleware.Audit(cfg.logger)(h)
	h = middleware.RateLimit(cfg.rateLimitRPS, cfg.rateLimitBurst)(h)
	h = middleware.RequestID(h)
	h = middleware.StripServerHeader(h)
	h = middleware.Recover(cfg.logger)(h)

	if cfg.logger == nil {
		cfg.logger = slog.Default()
	}

	return &Server{handler: h, mcp: mcpServer, client: c, cfg: cfg}
}

// Handler returns the http.Handler ready to serve.
func (s *Server) Handler() http.Handler { return s.handler }
