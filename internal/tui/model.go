package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/harshkumar18-bzz/sonicli/internal/spotify"
)

type API interface {
	Playback(context.Context) (spotify.Playback, error)
	Queue(context.Context) (spotify.Queue, error)
	Devices(context.Context) ([]spotify.Device, error)
	Search(context.Context, string) (spotify.SearchResults, error)
	SavedTracks(context.Context, int, int) (spotify.Page[spotify.SavedTrack], error)
	SavedAlbums(context.Context, int, int) (spotify.Page[spotify.SavedAlbum], error)
	Playlists(context.Context, int, int) (spotify.Page[spotify.Playlist], error)
	PlaylistItems(context.Context, string, int, int) (spotify.Page[spotify.PlaylistItem], error)
	Play(context.Context, []string, string, string) error
	Pause(context.Context, string) error
	Next(context.Context, string) error
	Previous(context.Context, string) error
	Seek(context.Context, int, string) error
	Volume(context.Context, int, string) error
	Shuffle(context.Context, bool, string) error
	Repeat(context.Context, string, string) error
	AddToQueue(context.Context, string, string) error
	Transfer(context.Context, string, bool) error
	SaveLibrary(context.Context, []string) error
	RemoveLibrary(context.Context, []string) error
	ContainsLibrary(context.Context, []string) ([]bool, error)
}

type View int

const (
	NowPlaying View = iota
	Search
	Library
	Devices
)

var viewNames = []string{"Now Playing", "Search", "Library", "Devices"}

type LibraryTab int

const (
	LibraryTracks LibraryTab = iota
	LibraryAlbums
	LibraryPlaylists
)

type searchItem struct {
	kind, name, subtitle, uri, id string
	contextURI                    string
}

const (
	frameInterval      = 100 * time.Millisecond
	commandRefreshWait = 100 * time.Millisecond
)

type Model struct {
	api           API
	width, height int
	view          View
	previousView  View
	selected      int
	libraryTab    LibraryTab
	playback      spotify.Playback
	queue         []spotify.Track
	devices       []spotify.Device
	tracks        []spotify.SavedTrack
	albums        []spotify.SavedAlbum
	playlists     []spotify.Playlist
	playlistItems []spotify.PlaylistItem
	openPlaylist  *spotify.Playlist
	searchResults []searchItem
	search        textinput.Model
	searching     bool
	loading       bool
	offline       bool
	sessionIdle   bool
	showHelp      bool
	status        string
	statusError   bool
	statusUntil   time.Time
	saved         map[string]bool
	skipQueue     map[string]int
	skipNext      bool
	lastVolume    int
	unicode       bool
	noColor       bool
	pollDelay     time.Duration
}

func New(api API, unicode bool) Model {
	input := textinput.New()
	input.Placeholder = "Search tracks, albums, artists, playlists"
	input.CharLimit = 100
	input.Prompt = "/ "
	return Model{
		api:          api,
		view:         NowPlaying,
		previousView: NowPlaying,
		search:       input,
		loading:      true,
		saved:        make(map[string]bool),
		skipQueue:    make(map[string]int),
		unicode:      unicode && os.Getenv("TERM") != "dumb",
		noColor:      os.Getenv("NO_COLOR") != "",
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(loadPlayback(m.api, true), loadQueue(m.api), loadLibrary(m.api), frameAfter())
}

type tickMsg time.Time
type frameMsg time.Time
type playbackMsg struct {
	value spotify.Playback
	err   error
	poll  bool
}
type queueMsg struct {
	value spotify.Queue
	err   error
}
type devicesMsg struct {
	value []spotify.Device
	err   error
}
type searchMsg struct {
	value spotify.SearchResults
	err   error
}
type libraryMsg struct {
	tracks    []spotify.SavedTrack
	albums    []spotify.SavedAlbum
	playlists []spotify.Playlist
	err       error
}
type playlistMsg struct {
	playlist spotify.Playlist
	items    []spotify.PlaylistItem
	err      error
}
type actionMsg struct {
	message      string
	err          error
	refresh      bool
	refreshQueue bool
}
type savedMsg struct {
	uri   string
	saved bool
	err   error
}
type savedStateMsg struct {
	uris   []string
	values []bool
	err    error
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.search.Width = max(16, m.contentWidth()-4)
	case tickMsg:
		cmds = append(cmds, loadPlayback(m.api, true))
	case frameMsg:
		cmds = append(cmds, frameAfter())
	case playbackMsg:
		m.loading = false
		previousTrack := ""
		if m.playback.Item != nil {
			previousTrack = m.playback.Item.ID
		}
		if msg.err != nil {
			m.offline = true
			m.setStatus(msg.err.Error(), true)
			m.pollDelay = nextBackoff(m.pollDelay, msg.err)
		} else {
			wasIdle := m.sessionIdle
			m.offline = false
			if msg.value.Item == nil && m.playback.Item != nil {
				// Spotify may return 204 No Content after a Connect session has
				// been paused for a while. Keep the last usable snapshot instead
				// of making Now Playing suddenly blank.
				m.playback.Progress = m.playback.CurrentProgress(time.Now())
				m.playback.Playing = false
				m.playback.FetchedAt = msg.value.FetchedAt
				m.sessionIdle = true
				if !wasIdle {
					m.setStatus("Spotify session idle · keeping the last track visible", false)
				}
			} else {
				m.playback = msg.value
				m.sessionIdle = false
			}
			m.pollDelay = 12 * time.Second
			if m.playback.Playing {
				m.pollDelay = 4 * time.Second
			}
			if !m.sessionIdle && m.skipNext && m.playback.Item != nil && m.playback.Item.Duration-m.playback.Progress <= 8_000 {
				m.pollDelay = 500 * time.Millisecond
			}
			playbackAdvanced := previousTrack != "" && m.playback.Item != nil && m.playback.Item.ID != previousTrack
			if m.skipNext && playbackAdvanced && m.playback.Item != nil && m.skipQueue[m.playback.Item.URI] > 0 {
				m.skipQueue[m.playback.Item.URI]--
				if m.skipQueue[m.playback.Item.URI] == 0 {
					delete(m.skipQueue, m.playback.Item.URI)
				}
				m.setStatus("Removed queued track reached playback; skipping it", false)
				cmds = append(cmds, action(func(ctx context.Context) error {
					return m.api.Next(ctx, m.deviceID())
				}, "Removed track skipped", true))
			}
			if m.playback.Item != nil && m.playback.Item.ID != previousTrack {
				cmds = append(cmds, loadQueue(m.api))
				cmds = append(cmds, checkSaved(m.api, []string{m.playback.Item.URI}))
			}
		}
		if msg.poll {
			cmds = append(cmds, tickAfter(m.pollDelay))
		}
	case queueMsg:
		if msg.err == nil {
			if len(msg.value.Items) == 0 && msg.value.CurrentlyPlaying == nil && len(m.queue) > 0 && (m.sessionIdle || !m.playback.Playing) {
				break
			}
			m.skipNext = len(msg.value.Items) > 0 && m.skipQueue[msg.value.Items[0].URI] > 0
			m.queue = m.visibleQueue(msg.value.Items)
			if m.selected >= m.itemCount() {
				m.selected = max(0, m.itemCount()-1)
			}
			cmds = append(cmds, checkSaved(m.api, trackURIs(m.queue)))
		}
	case devicesMsg:
		if msg.err != nil {
			m.setStatus(msg.err.Error(), true)
		} else {
			m.devices = msg.value
		}
	case searchMsg:
		m.searching = false
		if msg.err != nil {
			m.setStatus(msg.err.Error(), true)
		} else {
			m.searchResults = flattenSearch(msg.value)
			m.selected = 0
			cmds = append(cmds, checkSaved(m.api, searchTrackURIs(m.searchResults)))
		}
	case libraryMsg:
		if msg.err != nil {
			m.setStatus(msg.err.Error(), true)
		} else {
			m.tracks, m.albums, m.playlists = msg.tracks, msg.albums, msg.playlists
			for _, item := range msg.tracks {
				m.saved[item.Track.URI] = true
			}
		}
	case playlistMsg:
		if msg.err != nil {
			m.setStatus("Playlist unavailable: "+msg.err.Error(), true)
		} else {
			m.openPlaylist, m.playlistItems = &msg.playlist, msg.items
			m.selected = 0
			tracks := make([]spotify.Track, len(msg.items))
			for i := range msg.items {
				tracks[i] = msg.items[i].Item
			}
			cmds = append(cmds, checkSaved(m.api, trackURIs(tracks)))
		}
	case actionMsg:
		if msg.err != nil {
			m.setStatus(msg.err.Error(), true)
		} else {
			m.setStatus(msg.message, false)
		}
		if msg.refresh {
			cmds = append(cmds, delayedRefresh(m.api))
		}
		if msg.refreshQueue {
			cmds = append(cmds, delayedQueueRefresh(m.api))
		}
	case savedMsg:
		if msg.err != nil {
			m.setStatus(msg.err.Error(), true)
		} else {
			m.saved[msg.uri] = msg.saved
			if msg.saved {
				m.setStatus("Added to Liked Songs", false)
			} else {
				m.setStatus("Removed from Liked Songs", false)
			}
			cmds = append(cmds, loadLibrary(m.api))
		}
	case savedStateMsg:
		if msg.err == nil {
			for i, uri := range msg.uris {
				if i < len(msg.values) {
					m.saved[uri] = msg.values[i]
				}
			}
		}
	case tea.KeyMsg:
		if m.search.Focused() {
			switch msg.String() {
			case "esc":
				m.search.Blur()
			case "enter":
				query := strings.TrimSpace(m.search.Value())
				m.search.Blur()
				if query != "" {
					m.searching = true
					cmds = append(cmds, searchCmd(m.api, query))
				}
			default:
				var cmd tea.Cmd
				m.search, cmd = m.search.Update(msg)
				cmds = append(cmds, cmd)
			}
			return m, tea.Batch(cmds...)
		}
		return m.handleKey(msg)
	}
	return m, tea.Batch(cmds...)
}

func (m Model) handleKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "q", "ctrl+c":
		if m.showHelp {
			m.showHelp = false
			return m, nil
		}
		return m, tea.Quit
	case "?":
		m.showHelp = !m.showHelp
	case "tab":
		m.openPlaylist = nil
		m.view = View((int(m.view) + 1) % len(viewNames))
		m.selected = 0
		return m, m.loadView()
	case "shift+tab":
		m.openPlaylist = nil
		m.view = View((int(m.view) + len(viewNames) - 1) % len(viewNames))
		m.selected = 0
		return m, m.loadView()
	case "/":
		m.previousView, m.view = m.view, Search
		m.search.Focus()
		return m, textinput.Blink
	case "d":
		m.previousView, m.view, m.selected = m.view, Devices, 0
		return m, loadDevices(m.api)
	case "g":
		m.openPlaylist = nil
		m.previousView, m.view, m.selected = m.view, NowPlaying, 0
		return m, m.loadView()
	case "u":
		m.setStatus("Refreshing…", false)
		return m, m.refreshView()
	case "esc":
		if m.openPlaylist != nil {
			m.openPlaylist = nil
			m.playlistItems = nil
			m.selected = 0
		} else if m.view == Devices {
			m.view = m.previousView
		}
	case "up", "k":
		m.move(-1)
	case "down", "j":
		m.move(1)
	case "[":
		if m.view == Library {
			m.libraryTab = LibraryTab((int(m.libraryTab) + 2) % 3)
			m.selected = 0
		}
	case "]":
		if m.view == Library {
			m.libraryTab = LibraryTab((int(m.libraryTab) + 1) % 3)
			m.selected = 0
		}
	case "enter":
		return m, m.activateSelected()
	case " ", "space":
		return m, action(func(ctx context.Context) error {
			if m.playback.Playing {
				return m.api.Pause(ctx, m.deviceID())
			}
			return m.api.Play(ctx, nil, "", m.deviceID())
		}, "Playback updated", true)
	case "n":
		return m, action(func(ctx context.Context) error { return m.api.Next(ctx, m.deviceID()) }, "Skipped forward", true)
	case "p":
		return m, action(func(ctx context.Context) error { return m.api.Previous(ctx, m.deviceID()) }, "Skipped back", true)
	case "h":
		return m, action(func(ctx context.Context) error {
			return m.api.Seek(ctx, m.playback.CurrentProgress(time.Now())-10_000, m.deviceID())
		}, "Rewound 10 seconds", true)
	case "l":
		return m, action(func(ctx context.Context) error {
			return m.api.Seek(ctx, m.playback.CurrentProgress(time.Now())+10_000, m.deviceID())
		}, "Forwarded 10 seconds", true)
	case "+", "=":
		return m, action(func(ctx context.Context) error { return m.api.Volume(ctx, m.playback.Device.Volume+5, m.deviceID()) }, "Volume raised", true)
	case "-":
		return m, action(func(ctx context.Context) error { return m.api.Volume(ctx, m.playback.Device.Volume-5, m.deviceID()) }, "Volume lowered", true)
	case "m":
		volume, message := 0, "Muted"
		if m.playback.Device.Volume == 0 {
			volume, message = m.lastVolume, "Unmuted"
			if volume <= 0 {
				volume = 50
			}
		} else {
			m.lastVolume = m.playback.Device.Volume
		}
		return m, action(func(ctx context.Context) error { return m.api.Volume(ctx, volume, m.deviceID()) }, message, true)
	case "s":
		return m, action(func(ctx context.Context) error { return m.api.Shuffle(ctx, !m.playback.Shuffle, m.deviceID()) }, "Shuffle toggled", true)
	case "r":
		next := map[string]string{"off": "context", "context": "track", "track": "off"}[m.playback.Repeat]
		if next == "" {
			next = "off"
		}
		return m, action(func(ctx context.Context) error { return m.api.Repeat(ctx, next, m.deviceID()) }, "Repeat: "+next, true)
	case "a":
		if track, ok := m.selectedTrack(); ok {
			return m, queueAction(m.api, track.URI, m.deviceID(), track.Name)
		}
		m.setStatus("Choose a track before adding to the queue", true)
	case "x":
		if m.view != NowPlaying || m.selected <= 0 || m.selected-1 >= len(m.queue) {
			m.setStatus("Select an upcoming track to remove from the queue", true)
			return m, nil
		}
		track := m.queue[m.selected-1]
		if m.playback.Item != nil && track.URI == m.playback.Item.URI {
			m.setStatus("Cannot safely remove a queued duplicate of the current track", true)
			return m, nil
		}
		occurrences := 0
		for _, queued := range m.queue {
			if queued.URI == track.URI {
				occurrences++
			}
		}
		if occurrences > 1 {
			m.setStatus("Cannot safely remove one of several identical queued tracks", true)
			return m, nil
		}
		m.skipQueue[track.URI]++
		m.skipNext = m.selected == 1
		m.queue = append(m.queue[:m.selected-1], m.queue[m.selected:]...)
		if m.selected >= m.itemCount() {
			m.selected = max(0, m.itemCount()-1)
		}
		m.setStatus("Removed "+track.Name+" · Sonicli will skip it when reached", false)
		return m, nil
	case "f":
		if track, ok := m.selectedTrack(); ok {
			return m, toggleSaved(m.api, track.URI)
		}
		m.setStatus("Choose a track to add to Liked Songs", true)
	}
	return m, nil
}

func (m *Model) move(delta int) {
	count := m.itemCount()
	if count == 0 {
		m.selected = 0
		return
	}
	m.selected = (m.selected + delta + count) % count
}

func (m Model) itemCount() int {
	switch m.view {
	case NowPlaying:
		if m.playback.Item != nil {
			return len(m.queue) + 1
		}
		return 0
	case Search:
		return len(m.searchResults)
	case Devices:
		return len(m.devices)
	case Library:
		if m.openPlaylist != nil {
			return len(m.playlistItems)
		}
		switch m.libraryTab {
		case LibraryTracks:
			return len(m.tracks)
		case LibraryAlbums:
			return len(m.albums)
		default:
			return len(m.playlists)
		}
	}
	return 0
}

func (m Model) selectedTrack() (spotify.Track, bool) {
	if m.selected < 0 || m.selected >= m.itemCount() {
		return spotify.Track{}, false
	}
	switch m.view {
	case NowPlaying:
		if m.selected == 0 && m.playback.Item != nil {
			return *m.playback.Item, true
		}
		if m.selected > 0 && m.selected-1 < len(m.queue) {
			return m.queue[m.selected-1], true
		}
	case Search:
		item := m.searchResults[m.selected]
		if item.kind == "track" {
			return spotify.Track{ID: item.id, URI: item.uri, Name: item.name}, true
		}
	case Library:
		if m.openPlaylist != nil {
			return m.playlistItems[m.selected].Item, true
		}
		if m.libraryTab == LibraryTracks {
			return m.tracks[m.selected].Track, true
		}
	}
	return spotify.Track{}, false
}

func (m Model) activateSelected() tea.Cmd {
	if m.selected < 0 || m.selected >= m.itemCount() {
		return nil
	}
	device := m.deviceID()
	switch m.view {
	case Devices:
		id := m.devices[m.selected].ID
		return action(func(ctx context.Context) error { return m.api.Transfer(ctx, id, false) }, "Playback transferred", true)
	case NowPlaying:
		track, ok := m.selectedTrack()
		if !ok {
			return nil
		}
		uri := track.URI
		return action(func(ctx context.Context) error { return m.api.Play(ctx, []string{uri}, "", device) }, "Playing "+track.Name, true)
	case Search:
		item := m.searchResults[m.selected]
		if item.kind == "track" {
			return action(func(ctx context.Context) error { return m.api.Play(ctx, []string{item.uri}, item.contextURI, device) }, "Playing "+item.name, true)
		}
		if item.kind == "album" || item.kind == "artist" || item.kind == "playlist" {
			return action(func(ctx context.Context) error { return m.api.Play(ctx, nil, item.uri, device) }, "Playing "+item.name, true)
		}
	case Library:
		if m.openPlaylist != nil {
			uri := m.playlistItems[m.selected].Item.URI
			contextURI := m.openPlaylist.URI
			return action(func(ctx context.Context) error { return m.api.Play(ctx, []string{uri}, contextURI, device) }, "Playing track", true)
		}
		switch m.libraryTab {
		case LibraryTracks:
			track := m.tracks[m.selected].Track
			return action(func(ctx context.Context) error { return m.api.Play(ctx, []string{track.URI}, track.Album.URI, device) }, "Playing track", true)
		case LibraryAlbums:
			uri := m.albums[m.selected].Album.URI
			return action(func(ctx context.Context) error { return m.api.Play(ctx, nil, uri, device) }, "Playing album", true)
		case LibraryPlaylists:
			playlist := m.playlists[m.selected]
			return loadPlaylist(m.api, playlist)
		}
	}
	return nil
}

func (m Model) loadView() tea.Cmd {
	switch m.view {
	case NowPlaying:
		return tea.Batch(loadPlayback(m.api, false), loadQueue(m.api))
	case Devices:
		return loadDevices(m.api)
	case Library:
		return loadLibrary(m.api)
	}
	return nil
}

func (m Model) refreshView() tea.Cmd {
	switch m.view {
	case Search:
		if query := strings.TrimSpace(m.search.Value()); query != "" {
			return searchCmd(m.api, query)
		}
	case Library:
		if m.openPlaylist != nil {
			return loadPlaylist(m.api, *m.openPlaylist)
		}
		return loadLibrary(m.api)
	case Devices:
		return loadDevices(m.api)
	default:
		return tea.Batch(loadPlayback(m.api, false), loadQueue(m.api))
	}
	return nil
}

func (m Model) visibleQueue(tracks []spotify.Track) []spotify.Track {
	pending := make(map[string]int, len(m.skipQueue))
	for uri, count := range m.skipQueue {
		pending[uri] = count
	}
	visible := make([]spotify.Track, 0, len(tracks))
	for _, track := range tracks {
		if pending[track.URI] > 0 {
			pending[track.URI]--
			continue
		}
		visible = append(visible, track)
	}
	return visible
}

func (m Model) deviceID() string { return m.playback.Device.ID }

func (m *Model) setStatus(message string, isError bool) {
	m.status, m.statusError, m.statusUntil = message, isError, time.Now().Add(5*time.Second)
}

func tickAfter(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return tickMsg(t) })
}
func frameAfter() tea.Cmd {
	return tea.Tick(frameInterval, func(t time.Time) tea.Msg { return frameMsg(t) })
}
func loadPlayback(api API, poll bool) tea.Cmd {
	return func() tea.Msg { v, err := api.Playback(context.Background()); return playbackMsg{v, err, poll} }
}
func loadQueue(api API) tea.Cmd {
	return func() tea.Msg { v, err := api.Queue(context.Background()); return queueMsg{v, err} }
}
func loadDevices(api API) tea.Cmd {
	return func() tea.Msg { v, err := api.Devices(context.Background()); return devicesMsg{v, err} }
}
func searchCmd(api API, query string) tea.Cmd {
	return func() tea.Msg { v, err := api.Search(context.Background(), query); return searchMsg{v, err} }
}
func loadLibrary(api API) tea.Cmd {
	return func() tea.Msg {
		tracks, e1 := api.SavedTracks(context.Background(), 50, 0)
		albums, e2 := api.SavedAlbums(context.Background(), 50, 0)
		playlists, e3 := api.Playlists(context.Background(), 50, 0)
		return libraryMsg{tracks.Items, albums.Items, playlists.Items, errors.Join(e1, e2, e3)}
	}
}
func loadPlaylist(api API, p spotify.Playlist) tea.Cmd {
	return func() tea.Msg {
		items, err := api.PlaylistItems(context.Background(), p.ID, 50, 0)
		return playlistMsg{p, items.Items, err}
	}
}
func delayedRefresh(api API) tea.Cmd {
	return tea.Tick(commandRefreshWait, func(time.Time) tea.Msg {
		v, err := api.Playback(context.Background())
		return playbackMsg{v, err, false}
	})
}
func delayedQueueRefresh(api API) tea.Cmd {
	return tea.Tick(commandRefreshWait, func(time.Time) tea.Msg {
		v, err := api.Queue(context.Background())
		return queueMsg{v, err}
	})
}

func nextBackoff(current time.Duration, err error) time.Duration {
	var apiErr *spotify.APIError
	if errors.As(err, &apiErr) && apiErr.RetryAfter > 0 {
		return min(apiErr.RetryAfter, 5*time.Minute)
	}
	if current < 4*time.Second {
		return 4 * time.Second
	}
	return min(current*2, time.Minute)
}
func action(fn func(context.Context) error, message string, refresh bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		return actionMsg{message: message, err: fn(ctx), refresh: refresh}
	}
}
func queueAction(api API, uri, deviceID, name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		message := "Added to queue"
		if name != "" {
			message = "Queued " + name
		}
		return actionMsg{message: message, err: api.AddToQueue(ctx, uri, deviceID), refreshQueue: true}
	}
}
func toggleSaved(api API, uri string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		contains, err := api.ContainsLibrary(ctx, []string{uri})
		if err != nil {
			return savedMsg{uri: uri, err: err}
		}
		saved := len(contains) > 0 && contains[0]
		if saved {
			err = api.RemoveLibrary(ctx, []string{uri})
		} else {
			err = api.SaveLibrary(ctx, []string{uri})
		}
		return savedMsg{uri: uri, saved: !saved, err: err}
	}
}

func checkSaved(api API, uris []string) tea.Cmd {
	if len(uris) == 0 {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		values, err := api.ContainsLibrary(ctx, uris)
		return savedStateMsg{uris: uris, values: values, err: err}
	}
}

func trackURIs(tracks []spotify.Track) []string {
	uris := make([]string, 0, len(tracks))
	seen := make(map[string]bool, len(tracks))
	for _, track := range tracks {
		if track.URI != "" && !seen[track.URI] {
			seen[track.URI] = true
			uris = append(uris, track.URI)
		}
	}
	return uris
}

func searchTrackURIs(items []searchItem) []string {
	uris := make([]string, 0, len(items))
	for _, item := range items {
		if item.kind == "track" && item.uri != "" {
			uris = append(uris, item.uri)
		}
	}
	return uris
}

func flattenSearch(result spotify.SearchResults) []searchItem {
	items := make([]searchItem, 0, 40)
	for _, track := range result.Tracks.Items {
		items = append(items, searchItem{kind: "track", name: track.Name, subtitle: track.ArtistNames(), uri: track.URI, id: track.ID, contextURI: track.Album.URI})
	}
	for _, album := range result.Albums.Items {
		items = append(items, searchItem{kind: "album", name: album.Name, subtitle: artists(album.Artists), uri: album.URI, id: album.ID})
	}
	for _, artist := range result.Artists.Items {
		items = append(items, searchItem{kind: "artist", name: artist.Name, subtitle: "Artist", uri: artist.URI, id: artist.ID})
	}
	for _, playlist := range result.Playlists.Items {
		if playlist != nil {
			items = append(items, searchItem{kind: "playlist", name: playlist.Name, subtitle: playlist.Owner.DisplayName, uri: playlist.URI, id: playlist.ID})
		}
	}
	return items
}

func artists(values []spotify.Artist) string {
	names := make([]string, len(values))
	for i := range values {
		names[i] = values[i].Name
	}
	return strings.Join(names, ", ")
}

func (m Model) DebugState() string {
	return fmt.Sprintf("view=%d selected=%d items=%d", m.view, m.selected, m.itemCount())
}
