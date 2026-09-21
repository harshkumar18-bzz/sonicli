# Sonicli

Sonicli is a compact, keyboard-first Spotify player for Linux and macOS terminals. It can control an existing Spotify Connect device or play directly through the current computer using a separately installed local playback backend.

## Requirements

- A Spotify Premium account for playback controls.
- A Spotify developer application owned by a Premium account.
- At least one available Spotify Connect device, librespot, or Spotify Soloist for direct local audio playback.
- A terminal of at least 60×16 cells; 92 columns or wider enables side navigation.

Spotify development-mode applications support up to five allowlisted users. If somebody else uses your client ID, add their name and Spotify email under **Users Management** in the Spotify developer dashboard.

## Install from source

```sh
go install github.com/harshkumar18-bzz/sonicli/cmd/sonicli@latest
export PATH="$PATH:$(go env GOPATH)/bin"
sonicli --help
```

Add the `PATH` export to `~/.bashrc`, `~/.zshrc`, or the equivalent file for your shell so that `sonicli` remains available after opening a new terminal. If installation succeeds but the shell says `sonicli: command not found`, the missing `PATH` entry is normally the cause.

Release archives contain a single `sonicli` binary for Linux and macOS on AMD64 and ARM64.

## Quick start with local playback on Ubuntu or Debian

This complete setup installs Sonicli and librespot, connects both logins, and makes selected music play through the current computer:

```sh
# Install Sonicli.
go install github.com/harshkumar18-bzz/sonicli/cmd/sonicli@latest

# Install librespot's Linux build dependencies and librespot itself.
sudo apt-get update
sudo apt-get install -y build-essential libasound2-dev
cargo install librespot --locked

# Make both installed commands visible in this shell.
export PATH="$PATH:$(go env GOPATH)/bin:$HOME/.cargo/bin"

# Pair local playback and select it as Sonicli's backend.
sonicli player pair librespot
sonicli config --player librespot

# Confirm the setup, then launch.
sonicli player status
sonicli
```

The first `sonicli` login requests the Spotify developer client ID described below. The librespot pairing is a separate browser login that creates a reusable local playback credential.

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
| `m` | Mute or restore the previous volume |
| `s` / `r` | Toggle shuffle or cycle repeat |
| `a` | Add the current or selected track to the queue |
| `x` | Remove an upcoming track from Sonicli's session queue |
| `f` | Add or remove the current or selected track from Liked Songs |
| `g` | Return to Now Playing |
| `u` | Refresh the current view |
| `d` | Choose a playback device |
| `[` / `]` | Change library section |
| `?` | Help |
| `q` | Quit |

Set `NO_COLOR=1` to disable color. Set `unicode = false` with `sonicli config --unicode false` for an ASCII-only interface.

The current track is selected by default in Now Playing. Press `f` to toggle it in Spotify's **Liked Songs**, or `a` to add it to the end of the queue. Move onto an upcoming track with `j`/`k` to apply the same actions to that selection. Liked tracks display a clean `[LIKED]` label, and the Library's **Liked Songs** section refreshes after every change.

Selecting a track from search starts its album at that exact track; selecting a track inside a playlist starts that playlist at the selected row. This preserves Spotify's continuing context instead of creating a one-track playback request. Sonicli-managed librespot also starts with autoplay enabled, so Spotify can continue with recommendations after the selected album or playlist ends. The queue view refreshes as Spotify supplies the next window of tracks.

Spotify's Web API can read the playback queue and append to it, but it does not provide an endpoint to delete or reorder individual live queue entries. When you press `x`, Sonicli immediately hides that occurrence and automatically skips it when Spotify reaches it. This removal is active only while the current Sonicli session remains open; it does not mutate Spotify's server-side queue.

Now Playing renders progress locally ten times per second for a responsive elapsed/remaining counter while retaining the slower Spotify polling interval needed to avoid unnecessary API traffic. When a session-local removal is about to reach the front of the queue, Sonicli temporarily polls more quickly so the unwanted track is skipped promptly.

## Easy local playback with librespot

Sonicli can manage a user-installed [librespot](https://github.com/librespot-org/librespot) process on Linux or macOS. librespot turns the computer into a Spotify Connect receiver, while Sonicli continues to use the official Web API for search, library, queue, and selecting music. A Spotify Premium account is required.

1. Install the current librespot release and make sure `librespot` is in `PATH`. On Debian or Ubuntu, install the default Rodio/ALSA build dependencies first, as required by the upstream project:

   ```sh
   sudo apt-get update
   sudo apt-get install -y build-essential libasound2-dev
   cargo install librespot --locked
   export PATH="$PATH:$(go env GOPATH)/bin:$HOME/.cargo/bin"
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

## Troubleshooting

### `sonicli: command not found`

Confirm where Go installed the binary and add that directory to `PATH`:

```sh
ls "$(go env GOPATH)/bin/sonicli"
export PATH="$PATH:$(go env GOPATH)/bin"
```

### `librespot is selected but unavailable`

Sonicli does not bundle librespot. Confirm that the executable is installed and visible:

```sh
command -v librespot
librespot --version
sonicli player status
```

If `command -v librespot` produces no output, follow the librespot installation steps above. If Cargo installed it but it is still not found, add `$HOME/.cargo/bin` to `PATH`.

### `No active device found`

This means Spotify has no reachable playback target. For direct local playback, verify and repair the librespot setup:

```sh
sonicli player status
sonicli player pair librespot
sonicli config --player librespot
sonicli
```

For ordinary Spotify Connect playback, open Spotify on a phone, browser, desktop app, or speaker, start that device, and select it from Sonicli's **Devices** view by pressing `d`.

### librespot starts but does not appear

Inspect its log for audio, authentication, or network errors:

```sh
tail -n 100 ~/.local/share/sonicli/librespot/sonicli-librespot.log
```

Then confirm that the Spotify Web API login and local-player pairing are both ready:

```sh
sonicli auth status
sonicli player status
```

### Disable local playback

To use only another Spotify Connect device without starting librespot or Soloist:

```sh
sonicli config --player connect
```

## Development

```sh
go test ./...
go vet ./...
go run ./cmd/sonicli
```

The Spotify client is intentionally implemented directly against the current Web API instead of wrapping an older SDK. API tests use local HTTP servers and never require Spotify credentials.

Contributors and coding agents should read [AGENT.md](AGENT.md) after cloning. It documents repository structure, security constraints, safe modification workflows, required tests, cross-platform checks, and the release process.

## Limitations

Sonicli delegates direct audio decoding to a separately installed playback backend rather than implementing or redistributing Spotify's playback engine. It does not show album art, edit playlists, display lyrics, or support Windows in v1. Spotify may restrict the contents of playlists that the current user does not own or collaborate on; Sonicli reports that restriction without closing the player.
