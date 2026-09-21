package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/harsh/sonicli/internal/spotify"
)

type fakeAPI struct{}

func (fakeAPI) Playback(context.Context) (spotify.Playback, error) {
	return spotify.Playback{Playing: true, Progress: 10_000, Device: spotify.Device{ID: "d1", Name: "Desk", Volume: 45}, Item: &spotify.Track{URI: "spotify:track:1", Name: "Night Drive", Duration: 200_000, Artists: []spotify.Artist{{Name: "Example"}}, Album: spotify.Album{Name: "Signals"}}}, nil
}
func (fakeAPI) Queue(context.Context) (spotify.Queue, error) {
	return spotify.Queue{Items: []spotify.Track{{URI: "spotify:track:2", Name: "Afterglow", Artists: []spotify.Artist{{Name: "Example"}}}}}, nil
}
func (fakeAPI) Devices(context.Context) ([]spotify.Device, error) {
	return []spotify.Device{{ID: "d1", Name: "Desk", Active: true}}, nil
}
func (fakeAPI) Search(context.Context, string) (spotify.SearchResults, error) {
	return spotify.SearchResults{}, nil
}
func (fakeAPI) SavedTracks(context.Context, int, int) (spotify.Page[spotify.SavedTrack], error) {
	return spotify.Page[spotify.SavedTrack]{}, nil
}
func (fakeAPI) SavedAlbums(context.Context, int, int) (spotify.Page[spotify.SavedAlbum], error) {
	return spotify.Page[spotify.SavedAlbum]{}, nil
}
func (fakeAPI) Playlists(context.Context, int, int) (spotify.Page[spotify.Playlist], error) {
	return spotify.Page[spotify.Playlist]{}, nil
}
func (fakeAPI) PlaylistItems(context.Context, string, int, int) (spotify.Page[spotify.PlaylistItem], error) {
	return spotify.Page[spotify.PlaylistItem]{}, nil
}
func (fakeAPI) Play(context.Context, []string, string, string) error      { return nil }
func (fakeAPI) Pause(context.Context, string) error                       { return nil }
func (fakeAPI) Next(context.Context, string) error                        { return nil }
func (fakeAPI) Previous(context.Context, string) error                    { return nil }
func (fakeAPI) Seek(context.Context, int, string) error                   { return nil }
func (fakeAPI) Volume(context.Context, int, string) error                 { return nil }
func (fakeAPI) Shuffle(context.Context, bool, string) error               { return nil }
func (fakeAPI) Repeat(context.Context, string, string) error              { return nil }
func (fakeAPI) AddToQueue(context.Context, string, string) error          { return nil }
func (fakeAPI) Transfer(context.Context, string, bool) error              { return nil }
func (fakeAPI) SaveLibrary(context.Context, []string) error               { return nil }
func (fakeAPI) RemoveLibrary(context.Context, []string) error             { return nil }
func (fakeAPI) ContainsLibrary(context.Context, []string) ([]bool, error) { return []bool{false}, nil }

func TestResponsiveViews(t *testing.T) {
	m := New(fakeAPI{}, false)
	m.width, m.height = 100, 28
	m.loading = false
	p, _ := fakeAPI{}.Playback(context.Background())
	q, _ := fakeAPI{}.Queue(context.Background())
	m.playback, m.queue = p, q.Items
	wide := m.View()
	for _, want := range []string{"SONICLI", "Now Playing", "Night Drive", "Afterglow", "Spotify Connect"} {
		if !strings.Contains(wide, want) {
			t.Errorf("wide view missing %q\n%s", want, wide)
		}
	}
	m.width = 70
	narrow := m.View()
	if !strings.Contains(narrow, "Night Drive") {
		t.Fatal("narrow view omitted player content")
	}
	m.width, m.height = 50, 10
	if got := m.View(); !strings.Contains(got, "at least 60") {
		t.Fatalf("small view = %q", got)
	}
}

func TestNavigationAndSearchFocus(t *testing.T) {
	m := New(fakeAPI{}, false)
	m.width, m.height = 100, 28
	m.loading = false
	m.queue = []spotify.Track{{Name: "one"}, {Name: "two"}}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = updated.(Model)
	if m.selected != 1 {
		t.Fatalf("selected = %d", m.selected)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m = updated.(Model)
	if m.view != Search || !m.search.Focused() {
		t.Fatalf("search state: %s", m.DebugState())
	}
}

func TestFlattenSearchHandlesNullPlaylist(t *testing.T) {
	result := spotify.SearchResults{Tracks: spotify.Page[spotify.Track]{Items: []spotify.Track{{Name: "Track"}}}, Playlists: spotify.Page[*spotify.Playlist]{Items: []*spotify.Playlist{nil}}}
	items := flattenSearch(result)
	if len(items) != 1 || items[0].name != "Track" {
		t.Fatalf("items = %#v", items)
	}
}

func TestPollBackoff(t *testing.T) {
	if got := nextBackoff(4*time.Second, errors.New("offline")); got != 8*time.Second {
		t.Fatalf("backoff = %s", got)
	}
	apiErr := &spotify.APIError{Status: 429, RetryAfter: 37 * time.Second}
	if got := nextBackoff(time.Second, apiErr); got != 37*time.Second {
		t.Fatalf("Retry-After backoff = %s", got)
	}
	if got := nextBackoff(time.Minute, errors.New("offline")); got != time.Minute {
		t.Fatalf("capped backoff = %s", got)
	}
}

func TestGoldenRenderingStates(t *testing.T) {
	base := New(fakeAPI{}, false)
	base.width, base.height = 70, 18
	base.loading = false
	base.playback, _ = fakeAPI{}.Playback(context.Background())
	queue, _ := fakeAPI{}.Queue(context.Background())
	base.queue = queue.Items

	states := []struct {
		name string
		edit func(*Model)
	}{
		{"normal", func(*Model) {}},
		{"narrow", func(m *Model) { m.width = 60 }},
		{"no_color", func(m *Model) { m.noColor = true }},
		{"empty", func(m *Model) { m.playback.Item = nil; m.queue = nil }},
		{"loading", func(m *Model) { m.loading = true }},
		{"offline", func(m *Model) { m.offline = true }},
		{"error", func(m *Model) { m.setStatus("Spotify rate limit reached", true) }},
	}
	var got strings.Builder
	for _, state := range states {
		model := base
		state.edit(&model)
		fmt.Fprintf(&got, "--- %s ---\n%s\n", state.name, normalizeGolden(model.View()))
	}
	want, err := os.ReadFile(filepath.Join("testdata", "views.golden"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(got.String()) != strings.TrimSpace(string(want)) {
		_ = os.WriteFile(filepath.Join(os.TempDir(), "sonicli-views.golden"), []byte(got.String()), 0o600)
		t.Fatalf("rendering differs from golden file\nGOT:\n%s", got.String())
	}
}

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;:]*[A-Za-z]`)

func normalizeGolden(value string) string {
	value = ansiRE.ReplaceAllString(value, "")
	lines := strings.Split(value, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " ")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
