package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/harshkumar18-bzz/sonicli/internal/auth"
	"github.com/harshkumar18-bzz/sonicli/internal/config"
	"github.com/harshkumar18-bzz/sonicli/internal/player"
	"github.com/harshkumar18-bzz/sonicli/internal/spotify"
	"github.com/harshkumar18-bzz/sonicli/internal/tui"
)

var version = "dev"

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "sonicli:", err)
		os.Exit(1)
	}
}

func run(args []string, in io.Reader, out, errOut io.Writer) error {
	if len(args) > 0 {
		switch args[0] {
		case "-h", "--help", "help":
			printHelp(out)
			return nil
		case "-v", "--version", "version":
			fmt.Fprintf(out, "sonicli %s\n", version)
			return nil
		case "auth":
			return runAuth(args[1:], in, out)
		case "config":
			return runConfig(args[1:], in, out)
		case "completion":
			return runCompletion(args[1:], out)
		case "player":
			return runPlayer(args[1:], out)
		default:
			return fmt.Errorf("unknown command %q; use --help", args[0])
		}
	}
	cfg, manager, err := setup(in, out)
	if err != nil {
		return err
	}
	loggedIn, fallback, err := manager.Status()
	if err != nil {
		return err
	}
	if !loggedIn {
		fmt.Fprintln(out, "Connect your Spotify account to continue.")
		if err := manager.Login(context.Background()); err != nil {
			return err
		}
		fallback = manager.Store.UsingFallback()
	}
	if fallback {
		fmt.Fprintln(errOut, "Warning: system keychain unavailable; OAuth token is stored in a permission-restricted local file.")
	}
	client := spotify.New(manager)
	api, stopLocal, err := startLocalPlayer(cfg.Player, client, errOut)
	if err != nil {
		return err
	}
	defer stopLocal()
	program := tea.NewProgram(tui.New(api, cfg.Unicode), tea.WithAltScreen())
	_, err = program.Run()
	return err
}

func setup(in io.Reader, out io.Writer) (config.Config, *auth.Manager, error) {
	cfg, err := config.Load()
	if err != nil {
		return cfg, nil, err
	}
	if cfg.ClientID == "" {
		fmt.Fprintln(out, "Welcome to Sonicli.")
		fmt.Fprintln(out, "Create a Spotify developer app, add http://127.0.0.1:8989/callback as a redirect URI, then paste its client ID.")
		fmt.Fprint(out, "Spotify client ID: ")
		value, err := bufio.NewReader(in).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return cfg, nil, err
		}
		cfg.ClientID = strings.TrimSpace(value)
		if cfg.ClientID == "" {
			return cfg, nil, errors.New("client ID is required")
		}
		if err := config.Save(cfg); err != nil {
			return cfg, nil, err
		}
	}
	dataDir, err := config.DataDir()
	if err != nil {
		return cfg, nil, err
	}
	store := auth.NewSecureStore(cfg.ClientID, dataDir)
	return cfg, auth.NewManager(cfg.ClientID, store, out), nil
}

func runAuth(args []string, in io.Reader, out io.Writer) error {
	if len(args) != 1 {
		return errors.New("usage: sonicli auth login|logout|status")
	}
	_, manager, err := setup(in, out)
	if err != nil {
		return err
	}
	switch args[0] {
	case "login":
		if err := manager.Login(context.Background()); err != nil {
			return err
		}
		fmt.Fprintln(out, "Spotify account connected.")
		if manager.Store.UsingFallback() {
			fmt.Fprintln(out, "Note: token stored in a permission-restricted file because no system keychain was available.")
		}
	case "logout":
		if err := manager.Logout(); err != nil {
			return err
		}
		fmt.Fprintln(out, "Spotify account disconnected.")
	case "status":
		ok, fallback, err := manager.Status()
		if err != nil {
			return err
		}
		if !ok {
			fmt.Fprintln(out, "Not connected.")
			return nil
		}
		location := "system keychain"
		if fallback {
			location = "permission-restricted token file"
		}
		fmt.Fprintln(out, "Connected. Token storage:", location)
	default:
		return errors.New("usage: sonicli auth login|logout|status")
	}
	return nil
}

func runConfig(args []string, in io.Reader, out io.Writer) error {
	set := flag.NewFlagSet("config", flag.ContinueOnError)
	set.SetOutput(out)
	clientID := set.String("client-id", "", "Spotify developer client ID")
	theme := set.String("theme", "", "theme name (default)")
	unicode := set.String("unicode", "", "true or false")
	playback := set.String("player", "", "local player: auto, librespot, soloist, or connect")
	if err := set.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	changed := false
	if *clientID != "" {
		cfg.ClientID = strings.TrimSpace(*clientID)
		changed = true
	}
	if *theme != "" {
		if *theme != "default" {
			return errors.New("only the default theme is available in v1")
		}
		cfg.Theme = *theme
		changed = true
	}
	if *unicode != "" {
		switch *unicode {
		case "true":
			cfg.Unicode = true
		case "false":
			cfg.Unicode = false
		default:
			return errors.New("--unicode must be true or false")
		}
		changed = true
	}
	if *playback != "" {
		switch *playback {
		case "auto", "librespot", "soloist", "connect":
			cfg.Player = *playback
		default:
			return errors.New("--player must be auto, librespot, soloist, or connect")
		}
		changed = true
	}
	if changed {
		if err := config.Save(cfg); err != nil {
			return err
		}
		fmt.Fprintln(out, "Configuration saved.")
	}
	path, _ := config.Path()
	masked := cfg.ClientID
	if len(masked) > 8 {
		masked = masked[:4] + "…" + masked[len(masked)-4:]
	}
	fmt.Fprintf(out, "config: %s\nclient_id: %s\ntheme: %s\nunicode: %t\nplayer: %s\n", path, emptyAs(masked, "not set"), cfg.Theme, cfg.Unicode, cfg.Player)
	return nil
}

func printHelp(out io.Writer) {
	fmt.Fprintln(out, `Sonicli — a minimal Spotify terminal player

Usage:
  sonicli                         Launch the player
  sonicli auth login              Connect or reconnect Spotify
  sonicli auth logout             Remove saved Spotify credentials
  sonicli auth status             Show connection and token storage
  sonicli config [options]        Show or update settings
  sonicli player pair [BACKEND]   Pair librespot or Spotify Soloist
  sonicli player status           Show local-player readiness
  sonicli completion SHELL        Print bash, zsh, or fish completion
  sonicli --version               Print the version

Config options:
  --client-id ID                  Set the Spotify developer client ID
  --theme default                 Select the built-in theme
  --unicode true|false            Toggle Unicode UI symbols
  --player BACKEND                auto, librespot, soloist, or connect

Inside the player press ? for all keyboard shortcuts.`)
}

func runCompletion(args []string, out io.Writer) error {
	if len(args) != 1 {
		return errors.New("usage: sonicli completion bash|zsh|fish")
	}
	switch args[0] {
	case "bash":
		fmt.Fprint(out, `complete -W "auth config player completion help version" sonicli
`)
	case "zsh":
		fmt.Fprint(out, `#compdef sonicli
_arguments '1:command:(auth config player completion help version)' '*::arg:->args'
`)
	case "fish":
		fmt.Fprint(out, `complete -c sonicli -f -a "auth config player completion help version"
`)
	default:
		return errors.New("usage: sonicli completion bash|zsh|fish")
	}
	return nil
}

func runPlayer(args []string, out io.Writer) error {
	if len(args) < 1 || len(args) > 2 {
		return errors.New("usage: sonicli player pair [librespot|soloist] | status")
	}
	switch args[0] {
	case "pair":
		backend := "auto"
		if len(args) == 2 {
			backend = args[1]
		}
		if backend != "auto" && backend != "librespot" && backend != "soloist" {
			return errors.New("usage: sonicli player pair [librespot|soloist]")
		}
		return pairPlayer(backend, out)
	case "status":
		if len(args) != 1 {
			return errors.New("usage: sonicli player status")
		}
		showPlayerStatus(out)
		return nil
	default:
		return errors.New("usage: sonicli player pair [librespot|soloist] | status")
	}
}

func startLocalPlayer(preference string, client *spotify.Client, errOut io.Writer) (tui.API, func(), error) {
	noStop := func() {}
	if preference == "" {
		preference = "auto"
	}
	if preference == "connect" {
		return client, noStop, nil
	}
	if preference == "auto" || preference == "soloist" {
		local, err := player.DiscoverSoloist()
		if err == nil && local.Configured() {
			if err := local.Start(context.Background()); err == nil {
				return &player.API{Client: client, Local: local}, func() { _ = local.Stop() }, nil
			} else if preference == "soloist" {
				return nil, noStop, fmt.Errorf("start Spotify Soloist: %w", err)
			} else {
				fmt.Fprintln(errOut, "Spotify Soloist unavailable; trying another Connect backend:", err)
			}
		} else if preference == "soloist" {
			if err != nil {
				return nil, noStop, fmt.Errorf("Spotify Soloist unavailable: %w", err)
			}
			return nil, noStop, errors.New("Spotify Soloist is selected but SOLOIST_API_KEY is not set")
		}
	}
	if preference == "auto" || preference == "librespot" {
		local, err := player.DiscoverLibrespot()
		if err != nil {
			if preference == "librespot" {
				return nil, noStop, fmt.Errorf("librespot is selected but unavailable: %w; install it, then run `sonicli player pair librespot`", err)
			}
			return client, noStop, nil
		}
		if !local.Paired() {
			if preference == "librespot" {
				return nil, noStop, errors.New("librespot is selected but not paired; run `sonicli player pair librespot`")
			}
			fmt.Fprintln(errOut, "librespot is installed but not paired; using another Spotify Connect device")
			return client, noStop, nil
		}
		if err := local.Start(context.Background()); err == nil {
			waitCtx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			device, waitErr := player.WaitForDevice(waitCtx, client, local.Name)
			cancel()
			if waitErr == nil {
				return &player.API{Client: client, TargetDeviceID: device.ID}, func() { _ = local.Stop() }, nil
			}
			_ = local.Stop()
			if preference == "librespot" {
				return nil, noStop, fmt.Errorf("librespot started but its Spotify Connect device was unavailable: %w", waitErr)
			}
			fmt.Fprintln(errOut, "librespot unavailable; using another Spotify Connect device:", waitErr)
		} else if preference == "librespot" {
			return nil, noStop, fmt.Errorf("start librespot: %w", err)
		} else {
			fmt.Fprintln(errOut, "librespot unavailable; using another Spotify Connect device:", err)
		}
	}
	return client, noStop, nil
}

func pairPlayer(backend string, out io.Writer) error {
	if backend == "auto" || backend == "soloist" {
		local, err := player.DiscoverSoloist()
		if err == nil && local.Configured() {
			fmt.Fprintln(out, "Pairing Spotify Soloist. Open Spotify and select the ‘Sonicli’ device when it appears.")
			if err := local.Pair(context.Background(), out); err != nil {
				return err
			}
			fmt.Fprintln(out, "Spotify Soloist paired. Running `sonicli` will start it automatically.")
			return nil
		}
		if backend == "soloist" {
			if err != nil {
				return fmt.Errorf("%w; install it from https://developer.spotify.com/documentation/soloist", err)
			}
			return errors.New("set SOLOIST_API_KEY to your personal Spotify Soloist API key, then retry")
		}
	}
	local, err := player.DiscoverLibrespot()
	if err != nil {
		return fmt.Errorf("%w; install it from https://github.com/librespot-org/librespot", err)
	}
	if local.Paired() {
		fmt.Fprintln(out, "librespot is already paired. Running `sonicli` will start it automatically.")
		return nil
	}
	fmt.Fprintln(out, "Opening librespot’s Spotify login. Complete it in the browser; Sonicli will cache the pairing securely.")
	if err := local.Pair(context.Background(), out); err != nil {
		return err
	}
	fmt.Fprintln(out, "librespot paired. Running `sonicli` will now play selections on this computer.")
	return nil
}

func showPlayerStatus(out io.Writer) {
	if local, err := player.DiscoverSoloist(); err != nil {
		fmt.Fprintln(out, "Spotify Soloist: not installed")
	} else if !local.Configured() {
		fmt.Fprintln(out, "Spotify Soloist: installed, SOLOIST_API_KEY not set")
	} else if err := local.Status(context.Background()); err != nil {
		fmt.Fprintln(out, "Spotify Soloist: configured, not currently reachable")
	} else {
		fmt.Fprintln(out, "Spotify Soloist: running")
	}
	if local, err := player.DiscoverLibrespot(); err != nil {
		fmt.Fprintln(out, "librespot: not installed")
	} else if local.Paired() {
		fmt.Fprintln(out, "librespot: installed and paired")
	} else {
		fmt.Fprintln(out, "librespot: installed, pairing required")
	}
}

func emptyAs(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
