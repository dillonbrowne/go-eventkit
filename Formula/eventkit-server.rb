class EventkitServer < Formula
  desc "Localhost REST API + MCP foundation for macOS Calendar and Reminders"
  homepage "https://github.com/dillonbrowne/go-eventkit"
  # Pin to a tagged release for stability; switch to `head` block below
  # for the latest main.
  url "https://github.com/dillonbrowne/go-eventkit/archive/refs/tags/v0.1.0.tar.gz"
  version "0.1.0"
  sha256 "REPLACE_ME_AFTER_TAGGING"
  license "MIT"
  head "https://github.com/dillonbrowne/go-eventkit.git", branch: "main"

  depends_on "go" => :build
  depends_on :macos

  def install
    # Build directly from the cmd/ entrypoint. cgo is required (EventKit
    # bridge), so a non-darwin build will fail — depends_on :macos above
    # makes the formula skip non-darwin platforms. The version is
    # injected into internal/version.Version so /openapi.json and
    # /healthz report the release tag.
    ldflags = "-s -w -X github.com/dillonbrowne/go-eventkit/internal/version.Version=v#{version}"
    system "go", "build", *std_go_args(ldflags: ldflags, output: bin/"eventkit-server"),
                          "-trimpath", "./cmd/eventkit-server"
    system "go", "build", *std_go_args(ldflags: ldflags, output: bin/"bundle-app"),
                          "-trimpath", "./cmd/bundle-app"

    # Example policy: shipped as docs only; the server runs without one
    # by default (restricted to EventKit's default calendar/list).
    (etc/"eventkit-server").install "policy.example.yaml"
  end

  # `brew services start eventkit-server` writes a launchd agent that
  # invokes this command. Edit the args if you want a different port or
  # to point at a real allowlist policy file.
  service do
    run [
      opt_bin/"eventkit-server",
      "--listen", "127.0.0.1:8765",
      # "--policy", "#{Dir.home}/Library/Application Support/eventkit-server/policy.yaml",
    ]
    keep_alive true
    run_at_load false
    log_path var/"log/eventkit-server.log"
    error_log_path var/"log/eventkit-server.log"
    working_dir var
    environment_variables PATH: std_service_path_env
  end

  def caveats
    <<~EOS
      The first time the service runs, macOS will pop two TCC prompts
      (Calendar, Reminders). Click Allow on both. The prompts attribute
      to "eventkit-server" because the binary is not wrapped in a .app
      bundle — use `cmd/bundle-app` in the source repo if you'd rather
      see a proper app name in the dialog.

      Default policy: with no --policy flag, the server allows only
      EventKit's default calendar and default reminders list (the ones
      Calendar.app / Reminders.app create new items in when you don't
      pick a list). To scope access explicitly, copy and edit:
        #{etc}/eventkit-server/policy.example.yaml
      then point the service at it by editing the service block:
        brew services edit eventkit-server

      Endpoints once the service is running:
        curl http://127.0.0.1:8765/healthz
        open http://127.0.0.1:8765/docs              # Scalar API Reference
    EOS
  end

  test do
    # Smoke-check the binary's --version (no port bind, no TCC dialog).
    assert_match version.to_s, shell_output("#{bin}/eventkit-server --version")
    # Help text should mention the major flags.
    output = shell_output("#{bin}/eventkit-server --help 2>&1", 2)
    assert_match "policy", output
    assert_match "listen", output
  end
end
