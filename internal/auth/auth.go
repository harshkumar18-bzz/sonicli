package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
)

const (
	authorizeURL = "https://accounts.spotify.com/authorize"
	tokenURL     = "https://accounts.spotify.com/api/token"
	redirectURI  = "http://127.0.0.1:8989/callback"
)

var DefaultScopes = []string{
	"user-read-private",
	"user-read-playback-state",
	"user-read-currently-playing",
	"user-modify-playback-state",
	"user-library-read",
	"user-library-modify",
	"user-follow-read",
	"user-follow-modify",
	"playlist-read-private",
	"playlist-read-collaborative",
	"playlist-modify-public",
	"playlist-modify-private",
}

type Manager struct {
	ClientID string
	Store    Store
	Out      io.Writer
	HTTP     *http.Client
	OpenURL  func(string) error
	AuthURL  string
	TokenURL string

	mu sync.Mutex
}

func NewManager(clientID string, store Store, out io.Writer) *Manager {
	return &Manager{
		ClientID: clientID,
		Store:    store,
		Out:      out,
		HTTP:     http.DefaultClient,
		OpenURL:  OpenBrowser,
		AuthURL:  authorizeURL,
		TokenURL: tokenURL,
	}
}

func (m *Manager) Login(ctx context.Context) error {
	listener, err := net.Listen("tcp", "127.0.0.1:8989")
	if err != nil {
		return fmt.Errorf("start OAuth callback on 127.0.0.1:8989 (is another process using it?): %w", err)
	}
	defer listener.Close()

	cfg := m.oauthConfig(redirectURI)
	state, err := randomURLString(32)
	if err != nil {
		return err
	}
	verifier, err := randomURLString(64)
	if err != nil {
		return err
	}
	challengeBytes := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(challengeBytes[:])
	authURL := cfg.AuthCodeURL(state,
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
		oauth2.SetAuthURLParam("code_challenge", challenge),
		oauth2.SetAuthURLParam("show_dialog", "true"),
	)

	type result struct{ code, err string }
	resultCh := make(chan result, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			http.Error(w, "Invalid OAuth state. You can close this tab.", http.StatusBadRequest)
			select {
			case resultCh <- result{err: "invalid OAuth state"}:
			default:
			}
			return
		}
		if denied := r.URL.Query().Get("error"); denied != "" {
			fmt.Fprint(w, "Spotify login was cancelled. You can close this tab.")
			select {
			case resultCh <- result{err: denied}:
			default:
			}
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "Missing authorization code.", http.StatusBadRequest)
			select {
			case resultCh <- result{err: "missing authorization code"}:
			default:
			}
			return
		}
		fmt.Fprint(w, "Sonicli is connected to Spotify. You can close this tab.")
		select {
		case resultCh <- result{code: code}:
		default:
		}
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = server.Serve(listener) }()
	defer server.Shutdown(context.Background())

	if m.Out != nil {
		fmt.Fprintf(m.Out, "Open this URL to connect Spotify:\n%s\n", authURL)
	}
	if m.OpenURL != nil {
		_ = m.OpenURL(authURL) // Printed URL is always available as fallback.
	}

	loginCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	select {
	case <-loginCtx.Done():
		return fmt.Errorf("Spotify login timed out: %w", loginCtx.Err())
	case res := <-resultCh:
		if res.err != "" {
			return errors.New(res.err)
		}
		tokenCtx := context.WithValue(loginCtx, oauth2.HTTPClient, m.HTTP)
		token, err := cfg.Exchange(tokenCtx, res.code, oauth2.SetAuthURLParam("code_verifier", verifier))
		if err != nil {
			return fmt.Errorf("exchange authorization code: %w", err)
		}
		return m.Store.Save(token)
	}
}

// AccessToken returns a current bearer token. force requests a refresh even if
// the cached access token has not yet expired; SpotifyClient uses it after 401.
func (m *Manager) AccessToken(ctx context.Context, force bool) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	token, err := m.Store.Load()
	if err != nil {
		return "", err
	}
	if force {
		token.Expiry = time.Now().Add(-time.Minute)
	}
	cfg := m.oauthConfig("")
	tokenCtx := context.WithValue(ctx, oauth2.HTTPClient, m.HTTP)
	refreshed, err := cfg.TokenSource(tokenCtx, token).Token()
	if err != nil {
		return "", fmt.Errorf("refresh Spotify login: %w", err)
	}
	if refreshed.AccessToken != token.AccessToken || !refreshed.Expiry.Equal(token.Expiry) {
		if err := m.Store.Save(refreshed); err != nil {
			return "", err
		}
	}
	return refreshed.AccessToken, nil
}

func (m *Manager) Status() (bool, bool, error) {
	_, err := m.Store.Load()
	if errors.Is(err, ErrNoToken) {
		return false, m.Store.UsingFallback(), nil
	}
	return err == nil, m.Store.UsingFallback(), err
}

func (m *Manager) Logout() error { return m.Store.Delete() }

func (m *Manager) oauthConfig(redirect string) oauth2.Config {
	return oauth2.Config{
		ClientID:    m.ClientID,
		RedirectURL: redirect,
		Scopes:      DefaultScopes,
		Endpoint:    oauth2.Endpoint{AuthURL: m.AuthURL, TokenURL: m.TokenURL, AuthStyle: oauth2.AuthStyleInParams},
	}
}

func randomURLString(size int) (string, error) {
	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func OpenBrowser(rawURL string) error {
	if _, err := url.ParseRequestURI(rawURL); err != nil {
		return err
	}
	var name string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		name, args = "open", []string{rawURL}
	case "linux":
		name, args = "xdg-open", []string{rawURL}
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
	if strings.TrimSpace(rawURL) == "" {
		return errors.New("empty URL")
	}
	return exec.Command(name, args...).Start()
}
