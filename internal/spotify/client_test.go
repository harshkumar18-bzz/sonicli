package spotify

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeTokens struct {
	mu    sync.Mutex
	calls []bool
}

func (f *fakeTokens) AccessToken(_ context.Context, force bool) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, force)
	if force {
		return "fresh", nil
	}
	return "stale", nil
}

func TestClientOperations(t *testing.T) {
	seen := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen[r.Method+" "+r.URL.Path]++
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Error("missing bearer token")
		}
		if r.URL.Path == "/v1/me/library" && r.URL.Query().Get("uris") != "spotify:track:1" {
			t.Errorf("library uris = %q", r.URL.Query().Get("uris"))
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/me/player/devices":
			io.WriteString(w, `{"devices":[]}`)
		case "/v1/me/player/queue":
			if r.Method == http.MethodGet {
				io.WriteString(w, `{"queue":[]}`)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		case "/v1/search":
			if r.URL.Query().Get("limit") != "10" {
				t.Errorf("search limit = %q", r.URL.Query().Get("limit"))
			}
			io.WriteString(w, `{"tracks":{"items":[]},"albums":{"items":[]},"artists":{"items":[]},"playlists":{"items":[]}}`)
		case "/v1/me/tracks", "/v1/me/albums", "/v1/me/playlists", "/v1/playlists/p1/items":
			io.WriteString(w, `{"items":[]}`)
		case "/v1/me/library/contains":
			io.WriteString(w, `[true]`)
		case "/v1/me", "/v1/me/player":
			if r.Method == http.MethodGet {
				io.WriteString(w, `{}`)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer server.Close()
	c := New(&fakeTokens{})
	c.BaseURL = server.URL + "/v1"
	ctx := context.Background()
	if _, err := c.Me(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Playback(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Queue(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Devices(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Search(ctx, "jazz"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.SavedTracks(ctx, 25, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := c.SavedAlbums(ctx, 25, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Playlists(ctx, 25, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := c.PlaylistItems(ctx, "p1", 25, 0); err != nil {
		t.Fatal(err)
	}
	operations := []func() error{
		func() error { return c.Play(ctx, []string{"spotify:track:1"}, "", "d1") },
		func() error { return c.Pause(ctx, "d1") }, func() error { return c.Next(ctx, "d1") }, func() error { return c.Previous(ctx, "d1") },
		func() error { return c.Seek(ctx, 10_000, "d1") }, func() error { return c.Volume(ctx, 60, "d1") },
		func() error { return c.Shuffle(ctx, true, "d1") }, func() error { return c.Repeat(ctx, "track", "d1") },
		func() error { return c.AddToQueue(ctx, "spotify:track:1", "d1") }, func() error { return c.Transfer(ctx, "d1", false) },
		func() error { return c.SaveLibrary(ctx, []string{"spotify:track:1"}) }, func() error { return c.RemoveLibrary(ctx, []string{"spotify:track:1"}) },
	}
	for i, operation := range operations {
		if err := operation(); err != nil {
			t.Fatalf("operation %d: %v", i, err)
		}
	}
	contains, err := c.ContainsLibrary(ctx, []string{"spotify:track:1"})
	if err != nil || len(contains) != 1 || !contains[0] {
		t.Fatalf("ContainsLibrary() = %v, %v", contains, err)
	}
	for _, key := range []string{"PUT /v1/me/player/play", "PUT /v1/me/player/pause", "POST /v1/me/player/next", "POST /v1/me/player/previous", "PUT /v1/me/library", "DELETE /v1/me/library"} {
		if seen[key] == 0 {
			t.Errorf("endpoint not called: %s", key)
		}
	}
}

func TestClientRefreshesOnceOnUnauthorized(t *testing.T) {
	tokens := &fakeTokens{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer stale" {
			http.Error(w, `{"error":{"status":401,"message":"expired"}}`, http.StatusUnauthorized)
			return
		}
		io.WriteString(w, `{}`)
	}))
	defer server.Close()
	c := New(tokens)
	c.BaseURL = server.URL
	if _, err := c.Playback(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(tokens.calls) != 2 || tokens.calls[0] || !tokens.calls[1] {
		t.Fatalf("token calls = %v", tokens.calls)
	}
}

func TestAPIErrorTranslation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After", "7")
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"status": 429, "message": "slow down"}})
	}))
	defer server.Close()
	c := New(&fakeTokens{})
	c.BaseURL = server.URL
	_, err := c.Queue(context.Background())
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.RetryAfter != 7*time.Second || !strings.Contains(err.Error(), "7s") {
		t.Fatalf("error = %#v", err)
	}
}

func TestNoActiveDeviceErrorTranslation(t *testing.T) {
	err := (&APIError{Status: http.StatusNotFound, Message: "Player command failed: No active device found"}).Error()
	if !strings.Contains(err, "player status") || !strings.Contains(err, "pair librespot") {
		t.Fatalf("translated error = %q", err)
	}
}

func TestPlaybackProgress(t *testing.T) {
	p := Playback{Playing: true, Progress: 1000, Item: &Track{Duration: 5000}, FetchedAt: time.Now().Add(-2 * time.Second)}
	got := p.CurrentProgress(time.Now())
	if got < 2900 || got > 3200 {
		t.Fatalf("CurrentProgress() = %d", got)
	}
	p.Progress = 4900
	p.FetchedAt = time.Now().Add(-time.Second)
	if got := p.CurrentProgress(time.Now()); got != 5000 {
		t.Fatalf("clamped progress = %d", got)
	}
}
