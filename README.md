# Sonicli

Sonicli is a compact, keyboard-first Spotify player for Linux and macOS terminals. It controls an existing Spotify Connect device: the Spotify desktop app, phone, web player, speaker, or another active device.

## Requirements

- A Spotify Premium account for playback controls.
- A Spotify developer application owned by a Premium account.
- At least one available Spotify Connect device, or Spotify Soloist for direct Linux audio playback.
- A terminal of at least 60×16 cells; 92 columns or wider enables side navigation.

Spotify development-mode applications support up to five allowlisted users. If somebody else uses your client ID, add their name and Spotify email under **Users Management** in the Spotify developer dashboard.

## Install from source

```sh
go install github.com/harshkumar18-bzz/sonicli/cmd/sonicli@latest
```

Release archives contain a single `sonicli` binary for Linux and macOS on AMD64 and ARM64.

## Spotify setup

1. Open the [Spotify developer dashboard](https://developer.spotify.com/dashboard) and create an app.
2. Add `http://127.0.0.1:8989/callback` to its redirect URIs and click **Add**. Use the numeric loopback address and port exactly; Spotify does not allow `localhost`.
3. Copy the app's client ID. A client secret is neither requested nor needed.
4. Run `sonicli`. On first launch, paste the client ID and approve the browser login.

Sonicli uses Authorization Code with PKCE and a temporary callback listener on `127.0.0.1:8989`. Tokens are stored in the operating-system keychain. On headless systems without a usable keychain, Sonicli falls back to a file readable only by your user and displays a warning.

## Commands

```text
sonicli
sonicli auth login|logout|status
sonicli player pair|status
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

## Direct local playback on Linux

Sonicli can play selected music through the Linux computer's default PipeWire or PulseAudio output using [Spotify Soloist](https://developer.spotify.com/documentation/soloist), Spotify's official headless player. macOS continues to use Spotify Connect control.

1. Generate your personal **Spotify Soloist API Key** and download the current Soloist build from [Spotify's Soloist instructions](https://developer.spotify.com/documentation/soloist). Soloist builds expire after 90 days and must be updated from Spotify.
2. Put the `soloist` executable somewhere in `PATH`, for example `~/.local/bin/soloist`.
3. Keep the API key private and expose it to Sonicli:

   ```sh
   export SOLOIST_API_KEY='your-personal-key'
   ```

   Add that export to a private shell configuration or secret manager if you want it available after reboot. Do not commit the key.
4. Pair the local player once:

   ```sh
   sonicli player pair
   ```

   Open Spotify and select the **Sonicli** device when prompted.
5. Run `sonicli`. It starts Soloist for the TUI session and routes selected tracks, albums, playlists, queue actions, and player controls directly to the computer's audio output.

If Soloist is missing, unpaired, expired, or not configured, Sonicli prints a warning and safely falls back to controlling another Spotify Connect device. Player data is stored with user-only permissions under `~/.local/share/sonicli/soloist`; logs are written there when startup fails.

## Development

```sh
go test ./...
go vet ./...
go run ./cmd/sonicli
```

The Spotify client is intentionally implemented directly against the current Web API instead of wrapping an older SDK. API tests use local HTTP servers and never require Spotify credentials.

## Limitations

Sonicli delegates direct audio decoding to Spotify Soloist rather than implementing or redistributing Spotify's playback engine. It does not show album art, edit playlists, display lyrics, or support Windows in v1. Spotify may restrict the contents of playlists that the current user does not own or collaborate on; Sonicli reports that restriction without closing the player.
