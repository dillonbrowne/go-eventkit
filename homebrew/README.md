# Homebrew distribution

This repo doubles as its own Homebrew tap — the formula lives at the repo root in [`Formula/eventkit-server.rb`](../Formula/eventkit-server.rb) so `brew tap` can use this repo directly without a separate `homebrew-tap` repo.

## Install on any Mac

```sh
brew tap dillonbrowne/go-eventkit https://github.com/dillonbrowne/go-eventkit.git
brew install eventkit-server
brew services start eventkit-server
```

`brew tap` takes an explicit URL as its second argument, which lets a non-`homebrew-*`-named repo be used as a tap.

The first start pops the Calendar and Reminders TCC dialogs — click Allow on each. After that, launchd keeps the process running and restarts it on crash.

## Run from this checkout (no tap needed)

To validate the formula locally without publishing:

```sh
brew install --build-from-source ./Formula/eventkit-server.rb
brew services start eventkit-server
brew services info eventkit-server      # status + log path
brew services stop  eventkit-server
```

Use `--HEAD` to build from the current `main` branch without a tarball SHA:

```sh
brew install --HEAD --build-from-source ./Formula/eventkit-server.rb
```

## Updating the formula on release

`goreleaser` is configured to update `Formula/eventkit-server.rb` (URL, version, sha256) on every tag push and commit the change back to this same repo. Manual updates require:

```sh
# 1. Tag the new release
git tag v0.1.0 && git push origin v0.1.0

# 2. Compute the tarball SHA256
curl -sL https://github.com/dillonbrowne/go-eventkit/archive/refs/tags/v0.1.0.tar.gz | shasum -a 256

# 3. Update Formula/eventkit-server.rb — the `url`, `version`, and `sha256` lines
```

## Service tuning

Edit the launchd plist that `brew services` writes:

```sh
brew services edit eventkit-server
```

Common edits:

- **Change the port**: replace `127.0.0.1:8765` in the `ProgramArguments` list.
- **Attach an allowlist policy**: add `--policy /Users/you/Library/Application Support/eventkit-server/policy.yaml` to the args (the formula stages an example at `$(brew --prefix)/etc/eventkit-server/policy.example.yaml`).
- **Disable auto-restart**: change `<key>KeepAlive</key><true/>` to `<false/>`.

After editing run `brew services restart eventkit-server`.

## TCC notes

A bare `go build` binary has no `.app` bundle and no `Info.plist` usage descriptions. macOS still prompts for Calendar / Reminders access — the prompts are attributed to the binary path (`/opt/homebrew/bin/eventkit-server`) rather than to a friendly app name. The Allow decision is keyed to the binary's code signature; the formula does not codesign, so every reinstall under a different signature can re-prompt.

If you want the "Swift-style" attribution dialog with a proper app name, build a bundle via `cmd/bundle-app` in the source repo and run that instead of (or alongside) the service.
