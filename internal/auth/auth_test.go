package auth

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

type memoryStore struct {
	mu    sync.Mutex
	token *oauth2.Token
}

func (s *memoryStore) Load() (*oauth2.Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.token == nil {
		return nil, ErrNoToken
	}
	copy := *s.token
	return &copy, nil
}
func (s *memoryStore) Save(token *oauth2.Token) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	copy := *token
	s.token = &copy
	return nil
}
func (s *memoryStore) Delete() error       { s.mu.Lock(); defer s.mu.Unlock(); s.token = nil; return nil }
func (s *memoryStore) UsingFallback() bool { return false }

func TestLoginPKCE(t *testing.T) {
	var gotVerifier string
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		gotVerifier = r.Form.Get("code_verifier")
		if r.Form.Get("client_id") != "client-id" || r.Form.Get("code") != "test-code" {
			t.Errorf("unexpected token form: %v", r.Form)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"access_token":"access","refresh_token":"refresh","token_type":"Bearer","expires_in":3600}`)
	}))
	defer tokenServer.Close()

	store := &memoryStore{}
	m := NewManager("client-id", store, io.Discard)
	m.AuthURL = "https://accounts.example/authorize"
	m.TokenURL = tokenServer.URL
	m.OpenURL = func(raw string) error {
		u, err := url.Parse(raw)
		if err != nil {
			return err
		}
		if u.Query().Get("code_challenge_method") != "S256" || u.Query().Get("code_challenge") == "" {
			t.Errorf("PKCE parameters missing: %s", raw)
		}
		callback := u.Query().Get("redirect_uri") + "?code=test-code&state=" + url.QueryEscape(u.Query().Get("state"))
		go func() {
			resp, err := http.Get(callback)
			if err == nil {
				resp.Body.Close()
			}
		}()
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := m.Login(ctx); err != nil {
		t.Fatal(err)
	}
	if gotVerifier == "" {
		t.Fatal("token exchange omitted code_verifier")
	}
	token, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if token.AccessToken != "access" || token.RefreshToken != "refresh" {
		t.Fatalf("unexpected token: %#v", token)
	}
}

func TestLoginRejectsStateMismatch(t *testing.T) {
	m := NewManager("client-id", &memoryStore{}, io.Discard)
	m.AuthURL = "https://accounts.example/authorize"
	m.OpenURL = func(raw string) error {
		u, _ := url.Parse(raw)
		go http.Get(u.Query().Get("redirect_uri") + "?code=nope&state=wrong")
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := m.Login(ctx)
	if err == nil || !strings.Contains(err.Error(), "state") {
		t.Fatalf("Login() error = %v", err)
	}
}

func TestAccessTokenRefreshes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"access_token":"new-access","token_type":"Bearer","expires_in":3600}`)
	}))
	defer server.Close()
	store := &memoryStore{token: &oauth2.Token{AccessToken: "old", RefreshToken: "refresh", Expiry: time.Now().Add(time.Hour)}}
	m := NewManager("client-id", store, io.Discard)
	m.TokenURL = server.URL
	token, err := m.AccessToken(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if token != "new-access" {
		t.Fatalf("AccessToken() = %q", token)
	}
}
