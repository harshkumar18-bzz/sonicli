# Sonicli

Sonicli is a compact, keyboard-first Spotify player for Linux and macOS terminals. It controls an existing Spotify Connect device: the Spotify desktop app, phone, web player, speaker, or another active device.

## Requirements

- A Spotify Premium account for playback controls.
- A Spotify developer application owned by a Premium account.
- At least one available Spotify Connect device, librespot, or Spotify Soloist for direct local audio playback.
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
sonicli player pair [librespot|soloist]
sonicli player status
sonicli config [--client-id ID] [--theme default] [--unicode true|false]
               [--player auto|librespot|soloist|connect]
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

## Easy local playback with librespot

Sonicli can manage a user-installed [librespot](https://github.com/librespot-org/librespot) process on Linux or macOS. librespot turns the computer into a Spotify Connect receiver, while Sonicli continues to use the official Web API for search, library, queue, and selecting music. A Spotify Premium account is required.

1. Install the current librespot release and make sure `librespot` is in `PATH`. On Debian or Ubuntu, install the default Rodio/ALSA build dependencies first, as required by the upstream project:

   ```sh
   sudo apt-get install build-essential libasound2-dev
   cargo install librespot --locked
   export PATH="$PATH:$HOME/.cargo/bin"
   ```

   On macOS, `cargo install librespot --locked` is normally sufficient. Distribution packages can also be used when they provide a recent version with a working audio backend.
2. Pair it once using librespot's own browser login:

   ```sh
   sonicli player pair librespot
   ```

3. Prefer it for local playback:

   ```sh
   sonicli config --player librespot
   sonicli
   ```

Sonicli starts librespot for the TUI session, waits for its **Sonicli** Connect device, and directs selected tracks, albums, and playlists to it automatically. Choosing another device from the Devices view switches subsequent controls to that device. The reusable librespot credential is stored with user-only permissions under `~/.local/share/sonicli/librespot` and is never copied from Sonicli's Web API token.

librespot is an unofficial, reverse-engineered Spotify client. Its upstream project warns that using it may violate Spotify's terms and that compatibility can change when Spotify changes its service. Install and enable it only if you accept that tradeoff. Use `sonicli config --player connect` to use only ordinary Spotify Connect devices, or use the official Soloist option below on Linux.

## Official local playback on Linux

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
   sonicli player pair soloist
   ```

   Open Spotify and select the **Sonicli** device when prompted.
5. Run `sonicli`. It starts Soloist for the TUI session and routes selected tracks, albums, playlists, queue actions, and player controls directly to the computer's audio output.

With the default `player = "auto"` setting, Sonicli prefers a configured Soloist installation, then a paired librespot installation, then an existing Spotify Connect device. Select a backend explicitly with `sonicli config --player BACKEND`. Player data is stored with user-only permissions under `~/.local/share/sonicli`; backend logs are written there when startup fails.

## Development

```sh
go test ./...
go vet ./...
go run ./cmd/sonicli
```

The Spotify client is intentionally implemented directly against the current Web API instead of wrapping an older SDK. API tests use local HTTP servers and never require Spotify credentials.

## Limitations

Sonicli delegates direct audio decoding to a separately installed playback backend rather than implementing or redistributing Spotify's playback engine. It does not show album art, edit playlists, display lyrics, or support Windows in v1. Spotify may restrict the contents of playlists that the current user does not own or collaborate on; Sonicli reports that restriction without closing the player.
