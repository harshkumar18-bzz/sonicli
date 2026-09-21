package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/harsh/sonicli/internal/spotify"
)

type palette struct {
	accent, text, muted, border, error lipgloss.Color
}

func (m Model) colors() palette {
	if m.noColor {
		return palette{}
	}
	return palette{
		accent: lipgloss.Color("42"), text: lipgloss.Color("252"), muted: lipgloss.Color("242"),
		border: lipgloss.Color("238"), error: lipgloss.Color("203"),
	}
}

func (m Model) View() string {
	if m.width == 0 {
		return "Starting Sonicli…"
	}
	if m.width < 60 || m.height < 16 {
		return lipgloss.NewStyle().Padding(1, 2).Render(fmt.Sprintf("Sonicli needs at least 60×16.\nCurrent terminal: %d×%d", m.width, m.height))
	}
	if m.showHelp {
		return m.helpView()
	}
	p := m.colors()
	header := m.headerView(p)
	footer := m.footerView(p)
	bodyHeight := max(5, m.height-lipgloss.Height(header)-lipgloss.Height(footer)-1)
	var body string
	if m.width >= 92 {
		nav := m.navView(p, bodyHeight)
		content := lipgloss.NewStyle().Width(m.width-24).Height(bodyHeight).Padding(0, 1).Render(m.contentView(p, bodyHeight))
		body = lipgloss.JoinHorizontal(lipgloss.Top, nav, content)
	} else {
		body = lipgloss.NewStyle().Width(m.width-2).Height(bodyHeight).Padding(0, 1).Render(m.contentView(p, bodyHeight))
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
}

func (m Model) headerView(p palette) string {
	brand := lipgloss.NewStyle().Bold(true).Foreground(p.accent).Render("SONICLI")
	view := lipgloss.NewStyle().Foreground(p.text).Render(viewNames[m.view])
	right := "Spotify Connect"
	if m.offline {
		right = "OFFLINE · showing last state"
	}
	space := max(1, m.width-lipgloss.Width(brand)-lipgloss.Width(view)-lipgloss.Width(right)-8)
	return lipgloss.NewStyle().Width(m.width).Padding(0, 2).BorderBottom(true).BorderStyle(lipgloss.NormalBorder()).BorderForeground(p.border).Render(brand + "  " + view + strings.Repeat(" ", space) + lipgloss.NewStyle().Foreground(p.muted).Render(right))
}

func (m Model) navView(p palette, height int) string {
	lines := make([]string, len(viewNames))
	for i, name := range viewNames {
		prefix := "  "
		style := lipgloss.NewStyle().Foreground(p.muted)
		if View(i) == m.view {
			prefix = m.symbol("› ", "> ")
			style = style.Foreground(p.accent).Bold(true)
		}
		lines[i] = style.Render(prefix + name)
	}
	return lipgloss.NewStyle().Width(22).Height(height).Padding(1, 1).BorderRight(true).BorderStyle(lipgloss.NormalBorder()).BorderForeground(p.border).Render(strings.Join(lines, "\n\n"))
}

func (m Model) contentView(p palette, height int) string {
	switch m.view {
	case Search:
		return m.searchView(p, height)
	case Library:
		return m.libraryView(p, height)
	case Devices:
		return m.devicesView(p, height)
	default:
		return m.nowPlayingView(p, height)
	}
}

func (m Model) nowPlayingView(p palette, height int) string {
	if m.loading {
		return "\n  Loading playback…"
	}
	if m.playback.Item == nil {
		return "\n  Nothing is playing. Open Spotify on a device, then press d."
	}
	t := *m.playback.Item
	play := m.symbol("▶", ">")
	if !m.playback.Playing {
		play = m.symbol("Ⅱ", "||")
	}
	explicit := ""
	if t.Explicit {
		explicit = "  E"
	}
	title := lipgloss.NewStyle().Bold(true).Foreground(p.text).Render(t.Name)
	artist := lipgloss.NewStyle().Foreground(p.muted).Render(t.ArtistNames() + " · " + t.Album.Name + explicit)
	progress := m.progressBar(t.Duration, m.playback.CurrentProgress(time.Now()), max(20, min(m.width-34, 58)), p)
	meta := fmt.Sprintf("%s  %s  vol %d%%  shuffle %s  repeat %s", play, m.playback.Device.Name, m.playback.Device.Volume, onOff(m.playback.Shuffle), m.playback.Repeat)
	lines := []string{"", "  " + title, "  " + artist, "", "  " + progress, "  " + lipgloss.NewStyle().Foreground(p.muted).Render(meta), "", lipgloss.NewStyle().Bold(true).Render("  Up next")}
	limit := max(1, height-len(lines)-2)
	lines = append(lines, m.trackRows(m.queue, limit, p)...)
	return strings.Join(lines, "\n")
}

func (m Model) searchView(p palette, height int) string {
	lines := []string{"", m.search.View(), ""}
	if m.searching {
		lines = append(lines, "  Searching Spotify…")
	} else if len(m.searchResults) == 0 {
		lines = append(lines, "  Press / to search Spotify.")
	} else {
		limit := max(1, height-len(lines)-1)
		start, end := window(m.selected, len(m.searchResults), limit)
		for i := start; i < end; i++ {
			item := m.searchResults[i]
			name := fmt.Sprintf("%-9s %s", strings.ToUpper(item.kind), item.name)
			lines = append(lines, m.row(i, name, item.subtitle, p))
		}
	}
	return strings.Join(lines, "\n")
}

func (m Model) libraryView(p palette, height int) string {
	if m.openPlaylist != nil {
		lines := []string{"", lipgloss.NewStyle().Bold(true).Foreground(p.text).Render("  " + m.openPlaylist.Name), lipgloss.NewStyle().Foreground(p.muted).Render("  esc back · Enter play"), ""}
		tracks := make([]spotify.Track, len(m.playlistItems))
		for i := range m.playlistItems {
			tracks[i] = m.playlistItems[i].Item
		}
		return strings.Join(append(lines, m.trackRows(tracks, max(1, height-len(lines)), p)...), "\n")
	}
	tabs := []string{"Tracks", "Albums", "Playlists"}
	for i := range tabs {
		if LibraryTab(i) == m.libraryTab {
			tabs[i] = lipgloss.NewStyle().Foreground(p.accent).Bold(true).Render("[" + tabs[i] + "]")
		}
	}
	lines := []string{"", "  " + strings.Join(tabs, "   ") + "   " + lipgloss.NewStyle().Foreground(p.muted).Render("[ / ] change"), ""}
	limit := max(1, height-len(lines)-1)
	switch m.libraryTab {
	case LibraryTracks:
		tracks := make([]spotify.Track, len(m.tracks))
		for i := range m.tracks {
			tracks[i] = m.tracks[i].Track
		}
		lines = append(lines, m.trackRows(tracks, limit, p)...)
	case LibraryAlbums:
		start, end := window(m.selected, len(m.albums), limit)
		for i := start; i < end; i++ {
			a := m.albums[i].Album
			lines = append(lines, m.row(i, a.Name, artistNames(a.Artists), p))
		}
	case LibraryPlaylists:
		start, end := window(m.selected, len(m.playlists), limit)
		for i := start; i < end; i++ {
			pl := m.playlists[i]
			lines = append(lines, m.row(i, pl.Name, pl.Owner.DisplayName, p))
		}
	}
	if m.itemCount() == 0 {
		lines = append(lines, "  No items found.")
	}
	return strings.Join(lines, "\n")
}

func (m Model) devicesView(p palette, height int) string {
	lines := []string{"", lipgloss.NewStyle().Bold(true).Render("  Available devices"), lipgloss.NewStyle().Foreground(p.muted).Render("  Enter transfers playback"), ""}
	limit := max(1, height-len(lines)-1)
	start, end := window(m.selected, len(m.devices), limit)
	for i := start; i < end; i++ {
		d := m.devices[i]
		state := d.Type + fmt.Sprintf(" · %d%%", d.Volume)
		if d.Active {
			state += " · active"
		}
		if d.Restricted {
			state += " · restricted"
		}
		lines = append(lines, m.row(i, d.Name, state, p))
	}
	if len(m.devices) == 0 {
		lines = append(lines, "  No Spotify devices found. Open Spotify on a phone, computer, browser, or speaker.")
	}
	return strings.Join(lines, "\n")
}

func (m Model) trackRows(tracks []spotify.Track, limit int, p palette) []string {
	start, end := window(m.selected, len(tracks), limit)
	rows := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		t := tracks[i]
		suffix := t.ArtistNames()
		if t.Explicit {
			suffix += " · E"
		}
		rows = append(rows, m.row(i, t.Name, suffix, p))
	}
	if len(rows) == 0 {
		rows = append(rows, "  Queue is empty.")
	}
	return rows
}

func (m Model) row(index int, primary, secondary string, p palette) string {
	prefix := "  "
	style := lipgloss.NewStyle().Foreground(p.text)
	if index == m.selected {
		prefix = m.symbol("› ", "> ")
		style = style.Foreground(p.accent).Bold(true)
	}
	maxPrimary := max(12, m.width/2)
	line := prefix + truncate(primary, maxPrimary)
	if secondary != "" {
		line += lipgloss.NewStyle().Foreground(p.muted).Render("  " + truncate(secondary, max(10, m.width-maxPrimary-10)))
	}
	return style.Render(line)
}

func (m Model) footerView(p palette) string {
	text := "tab views   / search   space play/pause   n/p skip   ? help"
	statusActive := m.status != "" && time.Now().Before(m.statusUntil)
	if statusActive {
		text = m.status
	}
	style := lipgloss.NewStyle().Width(m.width).Padding(0, 2).Foreground(p.muted).BorderTop(true).BorderStyle(lipgloss.NormalBorder()).BorderForeground(p.border)
	if statusActive && m.statusError {
		style = style.Foreground(p.error)
	}
	return style.Render(truncate(text, max(10, m.width-4)))
}

func (m Model) helpView() string {
	keys := []string{
		"SONICLI · KEYBOARD", "", "tab / shift+tab   next / previous view", "j k / arrows       move selection", "enter              play, open, or select", "/                  search", "space              play / pause", "n / p              next / previous track", "h / l              seek -10s / +10s", "+ / -              volume", "s / r              shuffle / repeat", "a                  add selection to queue", "f                  save / remove selection", "d                  choose playback device", "[ / ]              library section", "esc                back / close", "q                  quit", "", "Press ? or q to close help.",
	}
	return lipgloss.NewStyle().Width(m.width).Height(m.height).Padding(1, 3).Render(strings.Join(keys, "\n"))
}

func (m Model) progressBar(duration, progress, width int, p palette) string {
	if duration <= 0 {
		duration = 1
	}
	filled := min(width, max(0, int(float64(progress)/float64(duration)*float64(width))))
	full, empty := "━", "─"
	if !m.unicode {
		full, empty = "=", "-"
	}
	bar := lipgloss.NewStyle().Foreground(p.accent).Render(strings.Repeat(full, filled)) + lipgloss.NewStyle().Foreground(p.border).Render(strings.Repeat(empty, width-filled))
	return fmt.Sprintf("%s  %s / %s", bar, formatDuration(progress), formatDuration(duration))
}

func (m Model) symbol(unicode, ascii string) string {
	if m.unicode {
		return unicode
	}
	return ascii
}
func onOff(value bool) string {
	if value {
		return "on"
	}
	return "off"
}
func formatDuration(ms int) string {
	if ms < 0 {
		ms = 0
	}
	d := time.Duration(ms) * time.Millisecond
	return fmt.Sprintf("%d:%02d", int(d.Minutes()), int(d.Seconds())%60)
}
func artistNames(values []spotify.Artist) string {
	names := make([]string, len(values))
	for i := range values {
		names[i] = values[i].Name
	}
	return strings.Join(names, ", ")
}
func truncate(value string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	r := []rune(value)
	if len(r) <= maxWidth {
		return value
	}
	if maxWidth <= 1 {
		return string(r[:maxWidth])
	}
	return string(r[:maxWidth-1]) + "…"
}
func window(selected, total, limit int) (int, int) {
	if limit <= 0 || total == 0 {
		return 0, 0
	}
	start := max(0, selected-limit/2)
	if start+limit > total {
		start = max(0, total-limit)
	}
	return start, min(total, start+limit)
}
