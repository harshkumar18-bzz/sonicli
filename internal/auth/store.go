package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/zalando/go-keyring"
	"golang.org/x/oauth2"
)

var ErrNoToken = errors.New("not logged in")

type Store interface {
	Load() (*oauth2.Token, error)
	Save(*oauth2.Token) error
	Delete() error
	UsingFallback() bool
}

// SecureStore uses the platform keychain and falls back to a mode-0600 file on
// systems without a usable keyring (common on headless Linux machines).
type SecureStore struct {
	service  string
	account  string
	path     string
	mu       sync.Mutex
	fallback bool
}

func NewSecureStore(clientID, dataDir string) *SecureStore {
	return &SecureStore{
		service: "sonicli",
		account: "spotify:" + clientID,
		path:    filepath.Join(dataDir, "token.json"),
	}
}

func (s *SecureStore) Load() (*oauth2.Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if raw, err := keyring.Get(s.service, s.account); err == nil {
		return decodeToken([]byte(raw))
	}
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNoToken
	}
	if err != nil {
		return nil, err
	}
	s.fallback = true
	return decodeToken(b)
}

func (s *SecureStore) Save(token *oauth2.Token) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := json.Marshal(token)
	if err != nil {
		return err
	}
	if err := keyring.Set(s.service, s.account, string(b)); err == nil {
		_ = os.Remove(s.path)
		s.fallback = false
		return nil
	}
	s.fallback = true
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(s.path, b, 0o600); err != nil {
		return fmt.Errorf("write token fallback: %w", err)
	}
	return os.Chmod(s.path, 0o600)
}

func (s *SecureStore) Delete() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, statErr := os.Stat(s.path)
	hadFallback := statErr == nil
	keyErr := keyring.Delete(s.service, s.account)
	fileErr := os.Remove(s.path)
	if errors.Is(keyErr, keyring.ErrNotFound) {
		keyErr = nil
	}
	if errors.Is(fileErr, os.ErrNotExist) {
		fileErr = nil
	}
	if hadFallback {
		keyErr = nil
	}
	return errors.Join(keyErr, fileErr)
}

func (s *SecureStore) UsingFallback() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.fallback
}

func decodeToken(b []byte) (*oauth2.Token, error) {
	var token oauth2.Token
	if err := json.Unmarshal(b, &token); err != nil {
		return nil, fmt.Errorf("decode token: %w", err)
	}
	if token.AccessToken == "" {
		return nil, ErrNoToken
	}
	return &token, nil
}
