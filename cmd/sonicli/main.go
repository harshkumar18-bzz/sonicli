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
	var api tui.API = client
	local, localErr := player.Discover()
	if localErr == nil && local.Configured() {
		if err := local.Start(context.Background()); err != nil {
			fmt.Fprintln(errOut, "Local playback unavailable:", err)
		} else {
			defer local.Stop()
			api = &player.API{Client: client, Local: local}
		}
	}
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
	fmt.Fprintf(out, "config: %s\nclient_id: %s\ntheme: %s\nunicode: %t\n", path, emptyAs(masked, "not set"), cfg.Theme, cfg.Unicode)
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
  sonicli player pair|status      Configure Linux local playback
  sonicli completion SHELL        Print bash, zsh, or fish completion
  sonicli --version               Print the version

Config options:
  --client-id ID                  Set the Spotify developer client ID
  --theme default                 Select the built-in theme
  --unicode true|false            Toggle Unicode UI symbols

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
	if len(args) != 1 {
		return errors.New("usage: sonicli player pair|status")
	}
	local, err := player.Discover()
	if err != nil {
		return fmt.Errorf("%w; install it from https://developer.spotify.com/documentation/soloist", err)
	}
	switch args[0] {
	case "pair":
		if !local.Configured() {
			return errors.New("set SOLOIST_API_KEY to your personal Spotify Soloist API key, then retry")
		}
		fmt.Fprintln(out, "Pairing the local Sonicli player. Open Spotify and select the ‘Sonicli’ device when it appears.")
		if err := local.Pair(context.Background(), out); err != nil {
			return err
		}
		fmt.Fprintln(out, "Local playback paired. Running `sonicli` will now start it automatically.")
		return nil
	case "status":
		if err := local.Status(context.Background()); err != nil {
			return err
		}
		fmt.Fprintln(out, "Spotify Soloist is running and reachable.")
		return nil
	default:
		return errors.New("usage: sonicli player pair|status")
	}
}

func emptyAs(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
