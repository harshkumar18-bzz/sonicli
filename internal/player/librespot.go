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
	"sync"
	"time"

	"github.com/harshkumar18-bzz/sonicli/internal/spotify"
)

// Librespot supervises a user-installed librespot process. It deliberately
// uses librespot's own OAuth credential cache instead of passing Sonicli's Web
// API token to an unofficial client.
type Librespot struct {
	Binary   string
	DataDir  string
	CacheDir string
	Name     string

	mu     sync.Mutex
	cmd    *exec.Cmd
	log    io.Closer
	active bool
}

func DiscoverLibrespot() (*Librespot, error) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		return nil, fmt.Errorf("librespot local playback is unsupported on %s", runtime.GOOS)
	}
	binary, err := exec.LookPath("librespot")
	if err != nil {
		return nil, errors.New("librespot is not installed or is not in PATH")
	}
	dataRoot, err := dataHome()
	if err != nil {
		return nil, err
	}
	cacheRoot, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}
	return &Librespot{
		Binary:   binary,
		DataDir:  filepath.Join(dataRoot, "sonicli", "librespot"),
		CacheDir: filepath.Join(cacheRoot, "sonicli", "librespot"),
		Name:     "Sonicli",
	}, nil
}

func (l *Librespot) Active() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.active
}

func (l *Librespot) Paired() bool {
	if l == nil {
		return false
	}
	info, err := os.Stat(filepath.Join(l.DataDir, "credentials.json"))
	return err == nil && info.Mode().IsRegular() && info.Size() > 0
}

// Pair runs librespot's interactive OAuth flow and stops the temporary player
// once the reusable credentials file has been created.
func (l *Librespot) Pair(ctx context.Context, out io.Writer) error {
	if l == nil || l.Binary == "" {
		return errors.New("librespot is unavailable")
	}
	if l.Paired() {
		return l.secureCredentials()
	}
	if err := l.prepareDirs(); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, l.Binary, l.args(true)...)
	cmd.Stdout, cmd.Stderr = out, out
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start librespot OAuth: %w", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case err := <-done:
			if l.Paired() {
				return l.secureCredentials()
			}
			if err == nil {
				err = errors.New("librespot exited before login completed")
			}
			return fmt.Errorf("librespot OAuth: %w", err)
		case <-ticker.C:
			if !l.Paired() {
				continue
			}
			_ = cmd.Process.Signal(os.Interrupt)
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				_ = cmd.Process.Kill()
				<-done
			}
			return l.secureCredentials()
		case <-ctx.Done():
			_ = cmd.Process.Kill()
			<-done
			return ctx.Err()
		}
	}
}

func (l *Librespot) Start(ctx context.Context) error {
	if l == nil || l.Binary == "" {
		return errors.New("librespot is unavailable")
	}
	if !l.Paired() {
		return errors.New("librespot is not paired; run `sonicli player pair librespot`")
	}
	if err := l.prepareDirs(); err != nil {
		return err
	}
	logPath := filepath.Join(l.DataDir, "sonicli-librespot.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, l.Binary, l.args(false)...)
	cmd.Stdout, cmd.Stderr = logFile, logFile
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return fmt.Errorf("start librespot: %w", err)
	}
	l.mu.Lock()
	l.cmd, l.log = cmd, logFile
	l.mu.Unlock()
	done := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		l.finish()
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			err = errors.New("librespot exited during startup")
		}
		return fmt.Errorf("start librespot (see %s): %w", logPath, err)
	case <-time.After(1200 * time.Millisecond):
		l.mu.Lock()
		l.active = true
		l.mu.Unlock()
		return nil
	case <-ctx.Done():
		_ = l.Stop()
		return ctx.Err()
	}
}

func (l *Librespot) Stop() error {
	l.mu.Lock()
	cmd := l.cmd
	l.active = false
	l.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	if err := cmd.Process.Signal(os.Interrupt); err != nil && !errors.Is(err, os.ErrProcessDone) {
		_ = cmd.Process.Kill()
		return err
	}
	return nil
}

func (l *Librespot) args(oauth bool) []string {
	args := []string{
		"--name", l.Name,
		"--device-type", "computer",
		"--bitrate", "320",
		"--cache", l.CacheDir,
		"--system-cache", l.DataDir,
	}
	if oauth {
		args = append(args, "--enable-oauth")
	}
	return args
}

func (l *Librespot) prepareDirs() error {
	for _, dir := range []string{l.DataDir, l.CacheDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
		if err := os.Chmod(dir, 0o700); err != nil {
			return err
		}
	}
	return nil
}

func (l *Librespot) secureCredentials() error {
	return os.Chmod(filepath.Join(l.DataDir, "credentials.json"), 0o600)
}

func (l *Librespot) finish() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.active = false
	l.cmd = nil
	if l.log != nil {
		_ = l.log.Close()
		l.log = nil
	}
}

// WaitForDevice waits until Spotify exposes the newly started receiver as a
// Connect device. The caller controls the deadline through ctx.
func WaitForDevice(ctx context.Context, client *spotify.Client, name string) (spotify.Device, error) {
	var lastErr error
	for {
		devices, err := client.Devices(ctx)
		if err == nil {
			for _, device := range devices {
				if device.Name == name && !device.Restricted {
					return device, nil
				}
			}
		} else {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			if lastErr != nil {
				return spotify.Device{}, fmt.Errorf("wait for %s Connect device: %w", name, lastErr)
			}
			return spotify.Device{}, fmt.Errorf("%s did not appear as a Spotify Connect device: %w", name, ctx.Err())
		case <-time.After(time.Second):
		}
	}
}
