package player

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const apiKeyEnv = "SOLOIST_API_KEY"

// Soloist supervises Spotify's official Linux headless player. The executable
// and API key are intentionally user-provided; Spotify prohibits redistribution
// of the binary and requires a per-user key.
type Soloist struct {
	Binary   string
	APIKey   string
	DataDir  string
	CacheDir string
	Name     string

	mu     sync.Mutex
	cmd    *exec.Cmd
	log    io.Closer
	active bool
}

func Discover() (*Soloist, error) {
	if runtime.GOOS != "linux" {
		return nil, fmt.Errorf("local playback requires Linux; %s continues to support Spotify Connect control", runtime.GOOS)
	}
	binary, err := exec.LookPath("soloist")
	if err != nil {
		return nil, errors.New("Spotify Soloist is not installed or is not in PATH")
	}
	dataRoot, err := dataHome()
	if err != nil {
		return nil, err
	}
	cacheRoot, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}
	return &Soloist{
		Binary:   binary,
		APIKey:   strings.TrimSpace(os.Getenv(apiKeyEnv)),
		DataDir:  filepath.Join(dataRoot, "sonicli", "soloist"),
		CacheDir: filepath.Join(cacheRoot, "sonicli", "soloist"),
		Name:     "Sonicli",
	}, nil
}

func (s *Soloist) Configured() bool { return s != nil && s.Binary != "" && s.APIKey != "" }
func (s *Soloist) Active() bool     { s.mu.Lock(); defer s.mu.Unlock(); return s.active }

func (s *Soloist) Pair(ctx context.Context, out io.Writer) error {
	if !s.Configured() {
		return fmt.Errorf("set %s to your Spotify Soloist API key", apiKeyEnv)
	}
	if err := s.prepareDirs(); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, s.Binary,
		"--device-name", s.Name,
		"--api-key", s.APIKey,
		"--data-dir", s.DataDir,
		"--cache-dir", s.CacheDir,
		"--pair",
	)
	cmd.Stdout, cmd.Stderr = out, out
	return cmd.Run()
}

func (s *Soloist) Start(ctx context.Context) error {
	if !s.Configured() {
		return fmt.Errorf("set %s to enable local playback", apiKeyEnv)
	}
	if err := s.prepareDirs(); err != nil {
		return err
	}
	if err := s.Status(ctx); err == nil {
		s.mu.Lock()
		s.active = true
		s.mu.Unlock()
		return nil
	}
	logPath := filepath.Join(s.DataDir, "sonicli-soloist.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, s.Binary,
		"--device-name", s.Name,
		"--api-key", s.APIKey,
		"--data-dir", s.DataDir,
		"--cache-dir", s.CacheDir,
		"--ws", "127.0.0.1:0",
	)
	cmd.Stdout, cmd.Stderr = logFile, logFile
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return err
	}
	s.mu.Lock()
	s.cmd, s.log = cmd, logFile
	s.mu.Unlock()
	done := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		s.finish()
		done <- err
	}()
	deadline := time.NewTimer(8 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case err := <-done:
			if err == nil {
				err = errors.New("Spotify Soloist exited during startup")
			}
			return fmt.Errorf("start Spotify Soloist (see %s): %w", logPath, err)
		case <-deadline.C:
			s.Stop()
			return fmt.Errorf("Spotify Soloist did not become ready; run `sonicli player status` and check %s", logPath)
		case <-ticker.C:
			statusCtx, cancel := context.WithTimeout(context.Background(), time.Second)
			err := s.Status(statusCtx)
			cancel()
			if err == nil {
				s.mu.Lock()
				s.active = true
				s.mu.Unlock()
				return nil
			}
		}
	}
}

func (s *Soloist) Stop() error {
	s.mu.Lock()
	cmd := s.cmd
	s.active = false
	s.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	err := cmd.Process.Signal(os.Interrupt)
	if err != nil && !errors.Is(err, os.ErrProcessDone) {
		_ = cmd.Process.Kill()
		return err
	}
	return nil
}

func (s *Soloist) Status(ctx context.Context) error {
	if s == nil || s.Binary == "" {
		return errors.New("Spotify Soloist is unavailable")
	}
	return s.runCtl(ctx, "status")
}

func (s *Soloist) Control(ctx context.Context, args ...string) error {
	if !s.Active() {
		return errors.New("local player is not running; use `sonicli player pair` first")
	}
	return s.runCtl(ctx, args...)
}

func (s *Soloist) runCtl(ctx context.Context, args ...string) error {
	commandArgs := []string{"ctl", "--data-dir", s.DataDir}
	commandArgs = append(commandArgs, args...)
	cmd := exec.CommandContext(ctx, s.Binary, commandArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("Spotify Soloist: %s", message)
	}
	return nil
}

func (s *Soloist) prepareDirs() error {
	for _, dir := range []string{s.DataDir, s.CacheDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
		if err := os.Chmod(dir, 0o700); err != nil {
			return err
		}
	}
	return nil
}

func (s *Soloist) finish() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.active = false
	s.cmd = nil
	if s.log != nil {
		_ = s.log.Close()
		s.log = nil
	}
}

func dataHome() (string, error) {
	if root := os.Getenv("XDG_DATA_HOME"); root != "" {
		return root, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share"), nil
}
