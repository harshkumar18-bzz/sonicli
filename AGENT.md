# Sonicli Agent Guide

This file is the working guide for coding agents and contributors modifying
Sonicli. Follow it together with `README.md`; the README is the user-facing
installation guide, while this file describes repository structure,
invariants, and verification expectations.

## Project goal

Sonicli is a minimal, keyboard-first Spotify terminal application for Linux
and macOS. It uses Spotify's Web API for authentication, catalog, library,
queue, device discovery, and playback control. Audio is produced by an
existing Spotify Connect device or a separately installed local backend:

- Spotify Soloist: official, Linux-only, user-provided binary and API key.
- librespot: unofficial, Linux/macOS, user-installed and explicitly paired.

Do not add an embedded audio engine or redistribute either playback backend.

## Repository guardrails

- Treat any `sources/` directory as read-only reference material. Never edit,
  rename, move, or delete its contents.
- Never commit Spotify client IDs, OAuth tokens, librespot credentials,
  Soloist API keys, keychain data, log files, or other user-specific state.
- Preserve unrelated user changes in a dirty working tree.
- Use `gofmt` for Go files and keep the project free of generated build
  artifacts. The `.gitignore` already excludes `/sonicli`, `/dist`, test
  binaries, coverage output, and macOS metadata.
- Keep Linux and macOS support working on both AMD64 and ARM64.
- Windows support, embedded streaming, lyrics, album art protocols, and
  playlist editing are outside the current v1 scope.

## Prerequisites

- Go 1.24 or newer.
- Git.
- Linux or macOS.
- Network access only when downloading modules or manually testing Spotify.
- Spotify Premium for playback control and all local playback backends.

Optional local-playback prerequisites are documented in `README.md`.
Automated tests must not require a real Spotify account, Premium subscription,
client ID, browser, keychain, Soloist installation, or librespot installation.

## Bootstrap after cloning

```sh
git clone https://github.com/harshkumar18-bzz/sonicli.git
cd sonicli
go mod download
go test ./...
go run ./cmd/sonicli --help
```

Install the checked-out source for the current user with:

```sh
go install ./cmd/sonicli
export PATH="$PATH:$(go env GOPATH)/bin"
sonicli --version
```

Do not start an interactive login during automated setup. A human can follow
the Spotify and local-player onboarding steps in `README.md` afterward.

## Architecture map

- `cmd/sonicli/`: CLI parsing, onboarding, backend selection, application
  startup, completions command, and version output.
- `internal/auth/`: Authorization Code with PKCE, loopback callback server,
  refresh-token handling, keychain storage, and protected file fallback.
- `internal/config/`: non-secret TOML configuration and platform-specific
  config/data directory resolution.
- `internal/spotify/`: thin typed Spotify Web API client and shared data
  types. Avoid adding an external Spotify wrapper.
- `internal/player/`: playback adapters and lifecycle supervision for Soloist
  and librespot.
- `internal/tui/`: Bubble Tea update model, asynchronous commands, keyboard
  behavior, responsive rendering, polling, and status messages.
- `completions/`: checked-in bash, zsh, and fish completion files included in
  releases.
- `.github/workflows/ci.yml`: Linux/macOS race tests, vet, and build checks.
- `.goreleaser.yaml`: Linux/macOS AMD64/ARM64 archives, checksums,
  completions, and draft GitHub releases.

## Critical behavior to preserve

### Authentication and secrets

- Use Authorization Code with PKCE; never request or embed a client secret.
- The redirect URI is exactly `http://127.0.0.1:8989/callback`. Do not replace
  the numeric loopback address with `localhost`.
- Validate OAuth state and maintain callback timeout/denial behavior.
- Store OAuth tokens in the OS keychain when possible. The fallback file and
  containing directories must remain user-only (`0600` files, `0700`
  directories), with a visible warning.
- Never print access tokens, refresh tokens, authorization codes, librespot
  credentials, or `SOLOIST_API_KEY`.

### Spotify client

- Keep endpoint support typed and small. New endpoints belong in
  `internal/spotify/client.go`; JSON shapes belong in `types.go`.
- Search requests are intentionally limited to 10 results per Spotify's
  current limit.
- Library mutations use the generic `/me/library` endpoints.
- Treat successful `204 No Content` responses as valid.
- Refresh once and retry after `401`; honor `Retry-After` on `429`.
- Translate errors into actions a terminal user can take. In particular,
  missing Premium, expired login, insufficient scope, rate limiting, and no
  active device must not appear only as raw HTTP errors.
- Spotify exposes queue read and append operations but no live-queue delete or
  reorder endpoint. Keep queue removal labeled as a session-local Sonicli
  action that hides an occurrence and auto-skips it when reached; never claim
  that it mutates Spotify's server-side queue.
- When a selected track has an album or playlist context, start that context
  with the track URI as the offset. Do not reduce contextual playback to a
  one-item `uris` request; Spotify needs the context to continue its queue.
- Preserve the last usable TUI state during transient offline failures.
- Spotify can return an empty successful playback response after a Connect
  session has been paused for a while. Preserve the last valid playback and
  queue snapshot, mark it session-idle, and replace it only when a real
  playback object arrives.

### Local playback

- `player = "auto"` prefers configured Soloist, then paired librespot, then an
  ordinary Spotify Connect device.
- `player = "soloist"` or `player = "librespot"` is an explicit user choice.
  If that backend is unavailable, fail before opening the alternate-screen
  TUI with a clear setup instruction; do not silently fall back.
- Never pass Sonicli's Web API access token to librespot. librespot uses its
  own interactive OAuth pairing and cached `credentials.json`.
- Keep librespot data and credential permissions restricted. Sonicli starts
  the process for the TUI session, waits for its `Sonicli` Connect device,
  targets playback to that device, enables autoplay for continued playback,
  and stops the child process on exit.
- Soloist remains optional and must continue to use the user-provided binary
  and `SOLOIST_API_KEY`; do not download or redistribute it.
- Selecting another device in the Devices view must update subsequent Web API
  playback commands to the newly selected device.

### Terminal UI

- Keep Bubble Tea's `Update` path non-blocking. Network and process work must
  be returned as commands/messages rather than executed synchronously inside
  input handling.
- Preserve both arrow keys and `j`/`k`, plus the documented shortcuts.
- Keep the minimum-size message, narrow single-pane layout, normal side-nav
  layout, wide two-panel Now Playing view, `NO_COLOR` behavior, and ASCII
  fallback working.
- Poll playback approximately every four seconds while playing and less often
  while paused. Continue interpolating progress locally and render 100 ms
  frame ticks independently so the timer can update smoothly without extra
  API calls.
- Status and error notices should not block navigation or close the TUI.

## Making a change

1. Read the relevant package and its tests before editing.
2. Search with `rg` rather than assuming a symbol is used in only one place.
3. Make the smallest coherent change and keep package boundaries intact.
4. Add or update tests for behavior, error handling, and edge cases.
5. Run `gofmt` on every changed Go file.
6. Run focused tests while iterating, then the full checks below.
7. Update `README.md`, command help, and all three completion files when a
   user-visible command or setup step changes.
8. Inspect `git diff --check` and `git status --short` before committing.

When adding configuration, give new fields a safe default so older
`config.toml` files continue to load. Configuration contains no secrets.

## Adding a Spotify operation

1. Add the typed request/response behavior to `internal/spotify`.
2. Cover the exact method, path, query, request body, response body, `204`,
   and relevant error responses with a local HTTP mock.
3. Add the method to `tui.API` only if the UI needs it.
4. Invoke it through a Bubble Tea command, then refresh affected state.
5. Confirm that both normal Connect playback and the targeted librespot path
   still use the correct device ID.

## Adding or changing a TUI action

1. Update the model in `internal/tui/model.go`.
2. Keep rendering in `view.go`; do not perform I/O from view functions.
3. Test navigation and state transitions with Bubble Tea messages.
4. Update the help overlay, `README.md` key table, and any status text.
5. Verify normal, narrow, empty, loading, offline, error, `NO_COLOR`, and
   ASCII-only render states when the change affects layout.

## Required verification

Run before every proposed commit:

```sh
gofmt -w <changed-go-files>
go test -race ./...
go vet ./...
go build ./cmd/sonicli
git diff --check
```

The tests use loopback HTTP servers for OAuth and Spotify mocks. A restricted
sandbox may require explicit permission for local listener sockets; do not
replace those tests with live Spotify calls.

For changes involving build tags, OS behavior, process management, or release
packaging, also cross-compile all supported targets:

```sh
for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; do
  GOOS="${target%/*}" GOARCH="${target#*/}" CGO_ENABLED=0 \
    go build -o "/tmp/sonicli-${target%/*}-${target#*/}" ./cmd/sonicli
done
```

Do not commit files produced under `/tmp` or `dist/`.

## Manual verification

Changes to authentication, playback, or device handling also require a human
smoke test with a Premium test account:

- login, refresh, status, and logout;
- search and play a track, album, artist, and playlist;
- queue, save/remove, seek, volume, shuffle, and repeat;
- transfer between at least two Connect devices;
- pair/start/stop whichever local backend was changed;
- restart Sonicli to confirm credentials and configuration persist;
- verify the TUI remains usable when the network or backend disappears.

Never use a contributor's personal Spotify credentials in automated tests or
record them in issue output.

## Release workflow

1. Ensure `main` is clean and CI passes on Linux and macOS.
2. Confirm the README, CLI help, completion files, and changelog-worthy
   behavior agree.
3. Run a local snapshot without publishing:

   ```sh
   goreleaser release --snapshot --clean
   ```

4. Create a semantic version tag only after review. GoReleaser builds Linux
   and macOS archives for AMD64/ARM64, includes completions, writes
   `checksums.txt`, and creates a draft GitHub release.
5. Never publish from an unreviewed or dirty worktree.

## Documentation expectations

- Keep commands copy-pasteable and specify whether they apply to Linux,
  macOS, or both.
- Prefer actionable troubleshooting: show a status command, the expected
  state, and the relevant log location.
- Clearly distinguish Spotify Web API login from librespot pairing.
- Keep the Premium requirement and librespot's unofficial status visible.
- Link to primary Spotify and librespot documentation when behavior depends
  on an external service or current upstream requirement.
