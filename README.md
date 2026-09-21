# Sonicli

Sonicli is a compact, keyboard-first Spotify player for Linux and macOS terminals. It controls an existing Spotify Connect device: the Spotify desktop app, phone, web player, speaker, or another active device.

## Requirements

- A Spotify Premium account for playback controls.
- A Spotify developer application owned by a Premium account.
- At least one available Spotify Connect device.
- A terminal of at least 60×16 cells; 92 columns or wider enables side navigation.

Spotify development-mode applications support up to five allowlisted users. If somebody else uses your client ID, add their name and Spotify email under **Users Management** in the Spotify developer dashboard.

## Install from source

```sh
go install github.com/harshkumar18-bzz/sonicli/cmd/sonicli@latest
```

Release archives contain a single `sonicli` binary for Linux and macOS on AMD64 and ARM64.

## Spotify setup

1. Open the [Spotify developer dashboard](https://developer.spotify.com/dashboard) and create an app.
2. Add `http://127.0.0.1/callback` to its redirect URIs. Use the numeric loopback address exactly; Spotify does not allow `localhost`.
3. Copy the app's client ID. A client secret is neither requested nor needed.
4. Run `sonicli`. On first launch, paste the client ID and approve the browser login.

Sonicli uses Authorization Code with PKCE and a temporary local callback. Tokens are stored in the operating-system keychain. On headless systems without a usable keychain, Sonicli falls back to a file readable only by your user and displays a warning.

## Commands

```text
sonicli
sonicli auth login|logout|status
sonicli config [--client-id ID] [--theme default] [--unicode true|false]
sonicli completion bash|zsh|fish
sonicli --help
sonicli --version
```

## Keys

| Key | Action |
| --- | --- |
| `Tab` / `Shift+Tab` | Change view |
| `j` / `k`, arrows | Move selection |
| `Enter` | Play, open, or choose |
| `/` | Search Spotify |
| `Space` | Play or pause |
| `n` / `p` | Next or previous track |
| `h` / `l` | Seek backward or forward 10 seconds |
| `+` / `-` | Change volume |
| `s` / `r` | Toggle shuffle or cycle repeat |
| `a` | Add selection to queue |
| `f` | Save or remove selection from the library |
| `d` | Choose a playback device |
| `[` / `]` | Change library section |
| `?` | Help |
| `q` | Quit |

Set `NO_COLOR=1` to disable color. Set `unicode = false` with `sonicli config --unicode false` for an ASCII-only interface.

## Development

```sh
go test ./...
go vet ./...
go run ./cmd/sonicli
```

The Spotify client is intentionally implemented directly against the current Web API instead of wrapping an older SDK. API tests use local HTTP servers and never require Spotify credentials.

## Limitations

Sonicli does not stream audio itself, show album art, edit playlists, display lyrics, or support Windows in v1. Spotify may restrict the contents of playlists that the current user does not own or collaborate on; Sonicli reports that restriction without closing the player.
