class EventkitMcp < Formula
  desc "MCP server exposing macOS Calendar + Reminders as AI tools"
  homepage "https://github.com/dillonbrowne/go-eventkit"
  url "https://github.com/dillonbrowne/go-eventkit/archive/refs/tags/v0.1.0.tar.gz"
  version "0.1.0"
  sha256 "REPLACE_ME_AFTER_TAGGING"
  license "MIT"
  head "git@github.com:dillonbrowne/go-eventkit.git", branch: "main"

  # eventkit-server is a RUNTIME companion (the MCP server proxies to it
  # over HTTP), not a build dependency — so it is intentionally not a
  # `depends_on`. Declaring it would force Homebrew to install the
  # *stable* eventkit-server formula even during a `--HEAD` install,
  # which is the wrong version to pair with a HEAD MCP build. Install
  # eventkit-server separately (see caveats).
  depends_on "go" => :build
  depends_on :macos

  def install
    ldflags = "-s -w -X github.com/dillonbrowne/go-eventkit/internal/version.Version=v#{version}"
    system "go", "build", *std_go_args(ldflags: ldflags, output: bin/"eventkit-mcp"),
                          "-trimpath", "./cmd/eventkit-mcp"
  end

  # `brew services start eventkit-mcp` writes a launchd agent that
  # invokes this command. The MCP server binds loopback by default and
  # has no auth — front with a tunnel (Cloudflare Tunnel, Tailscale
  # Funnel, etc.) if you need remote access.
  service do
    run [opt_bin/"eventkit-mcp",
         "--listen", "127.0.0.1:8766",
         "--api-base", "http://127.0.0.1:8765"]
    keep_alive true
    log_path var/"log/eventkit-mcp.log"
    error_log_path var/"log/eventkit-mcp.log"
  end

  def caveats
    <<~EOS
      The MCP server is loopback-only and has no built-in auth. Local
      MCP clients (Claude Desktop, Continue, Cline) can connect at
      http://127.0.0.1:8766 directly.

      For remote use (Claude web, ChatGPT custom connectors, etc.)
      put a tunnel + identity layer in front (Cloudflare Tunnel +
      Cloudflare Access, Tailscale Funnel, etc.).

      Install and start the REST server first — the MCP server is a
      translator in front of it:
        brew install --HEAD dillonbrowne/go-eventkit/eventkit-server
        brew services start eventkit-server
    EOS
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/eventkit-mcp --version")
    assert_match "listen", shell_output("#{bin}/eventkit-mcp --help 2>&1", 2)
  end
end
