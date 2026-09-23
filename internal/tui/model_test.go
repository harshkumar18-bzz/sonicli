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
	"github.com/harshkumar18-bzz/sonicli/internal/spotify"
)

type fakeAPI struct{}

func (fakeAPI) Playback(context.Context) (spotify.Playback, error) {
	return spotify.Playback{Playing: true, Progress: 10_000, Device: spotify.Device{ID: "d1", Name: "Desk", Volume: 45}, Item: &spotify.Track{URI: "spotify:track:1", Name: "Night Drive", Duration: 200_000, Artists: []spotify.Artist{{Name: "Example"}}, Album: spotify.Album{URI: "spotify:album:1", Name: "Signals"}}}, nil
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

func TestLikedLabelHasNoHeartGlyph(t *testing.T) {
	m := New(fakeAPI{}, false)
	m.width, m.height = 100, 28
	m.loading = false
	m.playback, _ = fakeAPI{}.Playback(context.Background())
	m.saved[m.playback.Item.URI] = true
	got := m.View()
	if !strings.Contains(got, "[LIKED]") || strings.Contains(got, "♥") {
		t.Fatalf("liked label is incorrect:\n%s", got)
	}
}

func TestNavigationAndSearchFocus(t *testing.T) {
	m := New(fakeAPI{}, false)
	m.width, m.height = 100, 28
	m.loading = false
	m.playback, _ = fakeAPI{}.Playback(context.Background())
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

type recordingAPI struct {
	fakeAPI
	queued       []string
	saved        []string
	removed      []string
	playedURIs   [][]string
	playContexts []string
	paused       int
	volume       int
	next         int
	contains     bool
}

func (f *recordingAPI) Play(_ context.Context, uris []string, contextURI, _ string) error {
	f.playedURIs = append(f.playedURIs, append([]string(nil), uris...))
	f.playContexts = append(f.playContexts, contextURI)
	return nil
}
func (f *recordingAPI) Pause(context.Context, string) error {
	f.paused++
	return nil
}

func (f *recordingAPI) AddToQueue(_ context.Context, uri, _ string) error {
	f.queued = append(f.queued, uri)
	return nil
}
func (f *recordingAPI) SaveLibrary(_ context.Context, uris []string) error {
	f.saved = append(f.saved, uris...)
	return nil
}
func (f *recordingAPI) RemoveLibrary(_ context.Context, uris []string) error {
	f.removed = append(f.removed, uris...)
	return nil
}
func (f *recordingAPI) ContainsLibrary(_ context.Context, uris []string) ([]bool, error) {
	values := make([]bool, len(uris))
	for i := range values {
		values[i] = f.contains
	}
	return values, nil
}
func (f *recordingAPI) Volume(_ context.Context, volume int, _ string) error {
	f.volume = volume
	return nil
}
func (f *recordingAPI) Next(context.Context, string) error {
	f.next++
	return nil
}

func TestLikeAndQueueCurrentOrSelectedTrack(t *testing.T) {
	api := &recordingAPI{}
	m := New(api, false)
	m.loading = false
	m.playback, _ = api.Playback(context.Background())
	m.queue = []spotify.Track{{URI: "spotify:track:2", Name: "Afterglow"}}

	updated, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = updated.(Model)
	msg := cmd()
	updated, _ = m.Update(msg)
	m = updated.(Model)
	if len(api.queued) != 1 || api.queued[0] != "spotify:track:1" || !strings.Contains(m.status, "Night Drive") {
		t.Fatalf("queue current = %#v, status %q", api.queued, m.status)
	}

	updated, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = updated.(Model)
	updated, cmd = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = updated.(Model)
	_ = cmd()
	if len(api.queued) != 2 || api.queued[1] != "spotify:track:2" {
		t.Fatalf("queue selection = %#v", api.queued)
	}

	m.selected = 0
	updated, cmd = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	m = updated.(Model)
	msg = cmd()
	updated, _ = m.Update(msg)
	m = updated.(Model)
	if len(api.saved) != 1 || api.saved[0] != "spotify:track:1" || !m.saved["spotify:track:1"] {
		t.Fatalf("liked current = %#v, state %#v", api.saved, m.saved)
	}

	api.contains = true
	updated, cmd = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	m = updated.(Model)
	msg = cmd()
	updated, _ = m.Update(msg)
	m = updated.(Model)
	if len(api.removed) != 1 || m.saved["spotify:track:1"] || !strings.Contains(m.status, "Removed") {
		t.Fatalf("unliked current = %#v, state %#v, status %q", api.removed, m.saved, m.status)
	}
}

func TestTrackOnlyActionsAndBasicControls(t *testing.T) {
	api := &recordingAPI{}
	m := New(api, false)
	m.loading = false
	m.playback, _ = api.Playback(context.Background())
	m.view = Search
	m.searchResults = []searchItem{{kind: "album", name: "Signals", uri: "spotify:album:1"}}
	updated, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = updated.(Model)
	if cmd != nil || !strings.Contains(m.status, "Choose a track") || len(api.queued) != 0 {
		t.Fatalf("album queue action: cmd=%v status=%q queue=%v", cmd != nil, m.status, api.queued)
	}

	m.view = NowPlaying
	updated, cmd = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m = updated.(Model)
	_ = cmd()
	if api.volume != 0 || m.lastVolume != 45 {
		t.Fatalf("mute volume=%d last=%d", api.volume, m.lastVolume)
	}
	m.playback.Device.Volume = 0
	updated, cmd = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m = updated.(Model)
	_ = cmd()
	if api.volume != 45 {
		t.Fatalf("unmute volume=%d", api.volume)
	}

	m.view = Library
	updated, cmd = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	m = updated.(Model)
	if m.view != NowPlaying || m.selected != 0 || cmd == nil {
		t.Fatalf("go now playing: %s", m.DebugState())
	}
}

func TestSpacebarPausesAndResumesPlayback(t *testing.T) {
	api := &recordingAPI{}
	m := New(api, false)
	m.loading = false
	m.playback, _ = api.Playback(context.Background())

	updated, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("spacebar did not create a pause command")
	}
	_ = cmd()
	if api.paused != 1 {
		t.Fatalf("Pause() calls = %d", api.paused)
	}

	m.playback.Playing = false
	updated, cmd = m.handleKey(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("spacebar did not create a resume command")
	}
	_ = cmd()
	if len(api.playedURIs) != 1 || len(api.playedURIs[0]) != 0 || api.playContexts[0] != "" {
		t.Fatalf("resume Play() calls = uris %#v, contexts %#v", api.playedURIs, api.playContexts)
	}
}

func TestRemoveQueuedTrackIsHiddenAndSkippedWhenReached(t *testing.T) {
	api := &recordingAPI{}
	m := New(api, false)
	m.loading = false
	m.playback, _ = api.Playback(context.Background())
	m.playback.Item.ID = "1"
	m.playback.Timestamp = 100
	m.queue = []spotify.Track{{ID: "2", URI: "spotify:track:2", Name: "Afterglow"}}
	m.selected = 1

	updated, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	m = updated.(Model)
	if cmd != nil || len(m.queue) != 0 || m.skipQueue["spotify:track:2"] != 1 || !m.skipNext {
		t.Fatalf("remove queued state: queue=%#v pending=%#v next=%t", m.queue, m.skipQueue, m.skipNext)
	}

	updated, _ = m.Update(queueMsg{value: spotify.Queue{Items: []spotify.Track{{ID: "2", URI: "spotify:track:2", Name: "Afterglow"}}}})
	m = updated.(Model)
	if len(m.queue) != 0 {
		t.Fatalf("removed track returned to visible queue: %#v", m.queue)
	}

	nextPlayback := spotify.Playback{Playing: true, Timestamp: 200, Device: spotify.Device{ID: "d1"}, Item: &spotify.Track{ID: "2", URI: "spotify:track:2", Name: "Afterglow", Duration: 100_000}}
	updated, cmd = m.Update(playbackMsg{value: nextPlayback})
	m = updated.(Model)
	if m.skipQueue["spotify:track:2"] != 0 || cmd == nil {
		t.Fatalf("pending removal not consumed: %#v", m.skipQueue)
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatalf("remove playback command type = %T", cmd())
	}
	for _, child := range batch {
		if child != nil {
			_ = child()
		}
	}
	if api.next != 1 {
		t.Fatalf("Next() calls = %d", api.next)
	}
}

func TestRemoveQueueRejectsAmbiguousDuplicate(t *testing.T) {
	api := &recordingAPI{}
	m := New(api, false)
	m.loading = false
	m.playback, _ = api.Playback(context.Background())
	m.queue = []spotify.Track{
		{ID: "2", URI: "spotify:track:2", Name: "Afterglow"},
		{ID: "2", URI: "spotify:track:2", Name: "Afterglow"},
	}
	m.selected = 1
	updated, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	m = updated.(Model)
	if cmd != nil || len(m.skipQueue) != 0 || !m.statusError || !strings.Contains(m.status, "identical") {
		t.Fatalf("duplicate removal: pending=%#v status=%q", m.skipQueue, m.status)
	}
}

func TestFrameTickKeepsProgressRenderingResponsive(t *testing.T) {
	if frameInterval != 100*time.Millisecond {
		t.Fatalf("frame interval = %s", frameInterval)
	}
	m := New(fakeAPI{}, false)
	updated, cmd := m.Update(frameMsg(time.Now()))
	if _, ok := updated.(Model); !ok || cmd == nil {
		t.Fatal("frame tick did not schedule the next render")
	}
}

func TestEmptyPlaybackPreservesLastUsableSession(t *testing.T) {
	m := New(fakeAPI{}, false)
	m.width, m.height = 100, 28
	m.loading = false
	m.playback, _ = fakeAPI{}.Playback(context.Background())
	m.playback.Item.ID = "track-1"
	m.playback.FetchedAt = time.Now().Add(-time.Second)
	m.queue = []spotify.Track{{ID: "track-2", URI: "spotify:track:2", Name: "Afterglow"}}

	updated, _ := m.Update(playbackMsg{value: spotify.Playback{FetchedAt: time.Now()}})
	m = updated.(Model)
	if m.playback.Item == nil || m.playback.Item.ID != "track-1" || m.playback.Playing || !m.sessionIdle {
		t.Fatalf("empty playback erased session: playback=%#v idle=%t", m.playback, m.sessionIdle)
	}
	if m.playback.Progress < 10_900 {
		t.Fatalf("frozen progress = %d", m.playback.Progress)
	}
	view := m.View()
	for _, want := range []string{"IDLE · showing last session", "SESSION IDLE", "Night Drive", "LAST QUEUE", "Afterglow"} {
		if !strings.Contains(view, want) {
			t.Fatalf("idle view missing %q:\n%s", want, view)
		}
	}

	updated, _ = m.Update(queueMsg{value: spotify.Queue{}})
	m = updated.(Model)
	if len(m.queue) != 1 || m.queue[0].ID != "track-2" {
		t.Fatalf("empty queue erased last snapshot: %#v", m.queue)
	}

	recovered := spotify.Playback{
		Playing:  true,
		Progress: 500,
		Device:   spotify.Device{ID: "d2", Name: "Phone"},
		Item:     &spotify.Track{ID: "track-3", URI: "spotify:track:3", Name: "Back Again", Duration: 100_000},
	}
	updated, _ = m.Update(playbackMsg{value: recovered})
	m = updated.(Model)
	if m.sessionIdle || m.playback.Item == nil || m.playback.Item.ID != "track-3" || !m.playback.Playing {
		t.Fatalf("real playback did not replace idle snapshot: playback=%#v idle=%t", m.playback, m.sessionIdle)
	}
}

func TestInitialEmptyPlaybackRemainsEmpty(t *testing.T) {
	m := New(fakeAPI{}, false)
	updated, _ := m.Update(playbackMsg{value: spotify.Playback{FetchedAt: time.Now()}})
	m = updated.(Model)
	if m.playback.Item != nil || m.sessionIdle || m.loading {
		t.Fatalf("initial empty playback = %#v idle=%t loading=%t", m.playback, m.sessionIdle, m.loading)
	}
}

func TestFlattenSearchHandlesNullPlaylist(t *testing.T) {
	result := spotify.SearchResults{Tracks: spotify.Page[spotify.Track]{Items: []spotify.Track{{Name: "Track", Album: spotify.Album{URI: "spotify:album:1"}}}}, Playlists: spotify.Page[*spotify.Playlist]{Items: []*spotify.Playlist{nil}}}
	items := flattenSearch(result)
	if len(items) != 1 || items[0].name != "Track" || items[0].contextURI != "spotify:album:1" {
		t.Fatalf("items = %#v", items)
	}
}

func TestSelectedTracksPreserveSpotifyContext(t *testing.T) {
	api := &recordingAPI{}
	m := New(api, false)
	m.loading = false
	m.playback, _ = api.Playback(context.Background())

	m.view = Search
	m.searchResults = []searchItem{{kind: "track", name: "Found", uri: "spotify:track:2", contextURI: "spotify:album:2"}}
	_ = m.activateSelected()()

	m.view = Library
	m.openPlaylist = &spotify.Playlist{URI: "spotify:playlist:3", Name: "Mix"}
	m.playlistItems = []spotify.PlaylistItem{{Item: spotify.Track{URI: "spotify:track:3"}}}
	_ = m.activateSelected()()

	m.openPlaylist = nil
	m.libraryTab = LibraryTracks
	m.tracks = []spotify.SavedTrack{{Track: spotify.Track{URI: "spotify:track:4", Album: spotify.Album{URI: "spotify:album:4"}}}}
	_ = m.activateSelected()()

	want := []string{"spotify:album:2", "spotify:playlist:3", "spotify:album:4"}
	if fmt.Sprint(api.playContexts) != fmt.Sprint(want) {
		t.Fatalf("play contexts = %#v, want %#v", api.playContexts, want)
	}
	for i, uris := range api.playedURIs {
		if len(uris) != 1 || uris[0] == "" {
			t.Fatalf("play call %d URIs = %#v", i, uris)
		}
	}
}

func TestContentWidthAccountsForNavigationRail(t *testing.T) {
	m := New(fakeAPI{}, false)
	m.width = 100
	if got := m.contentWidth(); got != 74 {
		t.Fatalf("wide content width = %d", got)
	}
	m.width = 70
	if got := m.contentWidth(); got != 66 {
		t.Fatalf("narrow content width = %d", got)
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
		{"idle", func(m *Model) { m.playback.Playing = false; m.sessionIdle = true }},
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
