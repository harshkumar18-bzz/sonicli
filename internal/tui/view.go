package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/harshkumar18-bzz/sonicli/internal/spotify"
)

type palette struct {
	accent, text, muted, border, surface, error lipgloss.Color
}

func (m Model) colors() palette {
	if m.noColor {
		return palette{}
	}
	return palette{
		accent: lipgloss.Color("42"), text: lipgloss.Color("252"), muted: lipgloss.Color("242"),
		border: lipgloss.Color("238"), surface: lipgloss.Color("235"), error: lipgloss.Color("203"),
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
	} else if m.sessionIdle {
		right = "IDLE · showing last session"
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

// contentWidth is the usable width inside the content pane. The navigation
// rail consumes part of wide terminals, so full terminal width would make
// progress bars and long rows wrap.
func (m Model) contentWidth() int {
	if m.width >= 92 {
		return max(20, m.width-26)
	}
	return max(20, m.width-4)
}

func (m Model) nowPlayingView(p palette, height int) string {
	if m.loading {
		return "\n  Loading playback…"
	}
	if m.playback.Item == nil {
		return "\n  Nothing is playing\n\n  Press / to find music or d to choose a Spotify Connect device."
	}
	if m.contentWidth() >= 90 {
		return m.nowPlayingWideView(p, height)
	}
	return m.nowPlayingCompactView(p, height)
}

func (m Model) nowPlayingCompactView(p palette, height int) string {
	t := *m.playback.Item
	play := m.playbackState()
	badgeText := m.trackBadges(t)
	contentWidth := m.contentWidth()
	titlePrefix := "  "
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(p.text)
	if m.selected == 0 {
		titlePrefix = m.symbol("› ", "> ")
		titleStyle = titleStyle.Foreground(p.accent)
	}
	titleWidth := max(8, contentWidth-lipgloss.Width(badgeText)-4)
	title := titleStyle.Render(truncate(t.Name, titleWidth)) + lipgloss.NewStyle().Foreground(p.muted).Render(badgeText)
	separator := m.symbol("  •  ", "  |  ")
	metadata := strings.Join(nonEmpty([]string{t.ArtistNames(), t.Album.Name}), separator)
	metadata = lipgloss.NewStyle().Foreground(p.muted).Render(truncate(metadata, contentWidth-4))
	progress := m.progressBar(t.Duration, m.playback.CurrentProgress(time.Now()), max(12, min(contentWidth-20, 54)), p)
	transport := strings.Join([]string{
		emptyAs(m.playback.Device.Name, "No active device"),
		fmt.Sprintf("%d%% volume", m.playback.Device.Volume),
		"shuffle " + onOff(m.playback.Shuffle),
		"repeat " + emptyAs(m.playback.Repeat, "off"),
	}, separator)
	stateStyle := lipgloss.NewStyle().Bold(true).Foreground(p.accent)
	if !m.playback.Playing {
		stateStyle = stateStyle.Foreground(p.muted)
	}
	section := lipgloss.NewStyle().Foreground(p.border).Render("  " + strings.Repeat(m.symbol("─", "-"), max(8, min(contentWidth-4, 64))))
	queueTitle := "UP NEXT"
	if m.sessionIdle {
		queueTitle = "LAST QUEUE"
	}
	queueLabel := fmt.Sprintf("  %s  %d", queueTitle, len(m.queue))
	queueHint := "  Enter play  ·  a add  ·  x remove"
	lines := []string{
		"",
		"  " + lipgloss.NewStyle().Foreground(p.muted).Render("CURRENT TRACK") + "  " + stateStyle.Render(play),
		titlePrefix + title,
		"  " + metadata,
		"",
		"  " + progress,
		"  " + lipgloss.NewStyle().Foreground(p.muted).Render(truncate(transport, contentWidth-4)),
		"",
		section,
		lipgloss.NewStyle().Bold(true).Render(queueLabel),
		lipgloss.NewStyle().Foreground(p.muted).Render(queueHint),
	}
	limit := max(1, height-len(lines))
	lines = append(lines, m.trackRows(m.queue, limit, 1, "  Queue is empty.", p)...)
	return strings.Join(lines, "\n")
}

func (m Model) nowPlayingWideView(p palette, height int) string {
	t := *m.playback.Item
	contentWidth := m.contentWidth()
	gap := 2
	queueWidth := min(38, max(30, contentWidth/3))
	currentWidth := max(44, contentWidth-queueWidth-gap)
	queueWidth = max(26, contentWidth-currentWidth-gap)

	play := m.playbackState()
	stateStyle := lipgloss.NewStyle().Bold(true).Foreground(p.accent)
	if !m.playback.Playing {
		stateStyle = stateStyle.Foreground(p.muted)
	}
	label := lipgloss.NewStyle().Foreground(p.muted).Render("CURRENT TRACK") + "  " + stateStyle.Render(play)

	badgeText := m.trackBadges(t)
	titlePrefix := "  "
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(p.text)
	if m.selected == 0 {
		titlePrefix = m.symbol("› ", "> ")
		titleStyle = titleStyle.Foreground(p.accent)
	}
	titleWidth := max(12, currentWidth-lipgloss.Width(badgeText)-6)
	title := titlePrefix + titleStyle.Render(truncate(t.Name, titleWidth))
	if badgeText != "" {
		title += lipgloss.NewStyle().Foreground(p.muted).Render(badgeText)
	}

	separator := m.symbol("  •  ", "  |  ")
	metadata := strings.Join(nonEmpty([]string{t.ArtistNames(), t.Album.Name}), separator)
	metadata = lipgloss.NewStyle().Foreground(p.muted).Render(truncate(metadata, currentWidth-4))
	progress := m.progressBar(t.Duration, m.playback.CurrentProgress(time.Now()), max(12, currentWidth-18), p)
	transport := strings.Join([]string{
		emptyAs(m.playback.Device.Name, "No active device"),
		fmt.Sprintf("%d%% volume", m.playback.Device.Volume),
		"shuffle " + onOff(m.playback.Shuffle),
		"repeat " + emptyAs(m.playback.Repeat, "off"),
	}, separator)
	transport = lipgloss.NewStyle().Foreground(p.muted).Render(truncate(transport, currentWidth-4))

	currentLines := []string{
		label,
		"",
		title,
		"  " + metadata,
		"",
		progress,
		"",
		transport,
		"",
		m.shortcutRows(p, currentWidth-2),
	}
	currentPanel := lipgloss.NewStyle().Width(currentWidth).Height(height).Padding(1, 1).Render(strings.Join(currentLines, "\n"))
	queuePanel := m.queuePanelView(p, height, queueWidth)
	spacer := lipgloss.NewStyle().Width(gap).Height(height).Render("")
	return lipgloss.JoinHorizontal(lipgloss.Top, currentPanel, spacer, queuePanel)
}

func (m Model) queuePanelView(p palette, height, width int) string {
	title := "UP NEXT"
	if m.sessionIdle {
		title = "LAST QUEUE"
	}
	header := spreadText(
		lipgloss.NewStyle().Bold(true).Foreground(p.text).Render(title),
		lipgloss.NewStyle().Bold(true).Foreground(p.accent).Render(fmt.Sprintf("%d", len(m.queue))),
		max(1, width-4),
	)
	lines := []string{header, ""}
	if len(m.queue) == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(p.muted).Render("Queue is empty."))
	} else {
		limit := max(1, (height-5)/2)
		start, end := window(max(0, m.selected-1), len(m.queue), limit)
		rowWidth := max(12, width-4)
		for i := start; i < end; i++ {
			track := m.queue[i]
			prefix := "  "
			rowStyle := lipgloss.NewStyle().Foreground(p.text)
			if m.selected == i+1 {
				prefix = m.symbol("› ", "> ")
				rowStyle = rowStyle.Foreground(p.accent).Bold(true)
			}
			artist := track.ArtistNames()
			name := track.Name
			if m.saved[track.URI] {
				name += " [LIKED]"
			}
			if track.Explicit {
				name += " [E]"
			}
			available := max(14, rowWidth-lipgloss.Width(prefix)-1)
			artistWidth := min(lipgloss.Width(artist), max(6, available/2))
			nameWidth := max(8, available-artistWidth)
			row := prefix + spreadText(
				truncate(name, nameWidth),
				lipgloss.NewStyle().Foreground(p.muted).Render(truncate(artist, artistWidth)),
				rowWidth-lipgloss.Width(prefix),
			)
			lines = append(lines, rowStyle.Render(row))
			if i < end-1 {
				lines = append(lines, lipgloss.NewStyle().Foreground(p.border).Render(strings.Repeat(m.symbol("─", "-"), rowWidth)))
			}
		}
	}
	return lipgloss.NewStyle().Width(width).Height(height).Padding(1, 2).Background(p.surface).BorderLeft(true).BorderStyle(lipgloss.NormalBorder()).BorderForeground(p.border).Render(strings.Join(lines, "\n"))
}

func (m Model) shortcutRows(p palette, width int) string {
	type shortcut struct{ key, label string }
	shortcuts := []shortcut{
		{"space", "play/pause"},
		{"n/p", "skip"},
		{"h/l", "seek 10s"},
		{"+/-", "volume"},
	}
	rows := make([]string, 0, 2)
	line := ""
	for _, item := range shortcuts {
		chip := m.shortcutChip(item.key, item.label, p)
		candidate := chip
		if line != "" {
			candidate = line + " " + chip
		}
		if line != "" && lipgloss.Width(candidate) > width {
			rows = append(rows, line)
			line = chip
		} else {
			line = candidate
		}
	}
	if line != "" {
		rows = append(rows, line)
	}
	return strings.Join(rows, "\n")
}

func (m Model) shortcutChip(key, label string, p palette) string {
	bracket := lipgloss.NewStyle().Foreground(p.border)
	return bracket.Render("[") +
		lipgloss.NewStyle().Bold(true).Foreground(p.accent).Render(key) + " " +
		lipgloss.NewStyle().Foreground(p.muted).Render(label) + bracket.Render("]")
}

func (m Model) playbackState() string {
	if m.sessionIdle {
		return "SESSION IDLE"
	}
	if !m.playback.Playing {
		return "PAUSED"
	}
	return "PLAYING"
}

func (m Model) trackBadges(t spotify.Track) string {
	badges := make([]string, 0, 2)
	if m.saved[t.URI] {
		badges = append(badges, "[LIKED]")
	}
	if t.Explicit {
		badges = append(badges, "[E]")
	}
	if len(badges) == 0 {
		return ""
	}
	return "  " + strings.Join(badges, " ")
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
			name := item.name
			if item.kind == "track" && m.saved[item.uri] {
				name += "  [LIKED]"
			}
			name = fmt.Sprintf("%-9s %s", strings.ToUpper(item.kind), name)
			lines = append(lines, m.row(i, name, item.subtitle, p))
		}
	}
	return strings.Join(lines, "\n")
}

func (m Model) libraryView(p palette, height int) string {
	if m.openPlaylist != nil {
		lines := []string{"", lipgloss.NewStyle().Bold(true).Foreground(p.text).Render("  " + m.openPlaylist.Name), lipgloss.NewStyle().Foreground(p.muted).Render("  esc back · Enter play · a queue · f like"), ""}
		tracks := make([]spotify.Track, len(m.playlistItems))
		for i := range m.playlistItems {
			tracks[i] = m.playlistItems[i].Item
		}
		return strings.Join(append(lines, m.trackRows(tracks, max(1, height-len(lines)), 0, "  Playlist is empty.", p)...), "\n")
	}
	tabs := []string{"Liked Songs", "Albums", "Playlists"}
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
		lines = append(lines, m.trackRows(tracks, limit, 0, "  No liked songs yet.", p)...)
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
	if m.itemCount() == 0 && m.libraryTab != LibraryTracks {
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

func (m Model) trackRows(tracks []spotify.Track, limit, selectionOffset int, empty string, p palette) []string {
	selected := m.selected - selectionOffset
	start, end := window(selected, len(tracks), limit)
	rows := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		t := tracks[i]
		suffix := t.ArtistNames()
		if t.Explicit {
			suffix += " · E"
		}
		name := t.Name
		if m.saved[t.URI] {
			name += "  [LIKED]"
		}
		rows = append(rows, m.row(i+selectionOffset, name, suffix, p))
	}
	if len(rows) == 0 {
		rows = append(rows, empty)
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
	contentWidth := m.contentWidth()
	maxPrimary := max(12, contentWidth/2)
	line := prefix + truncate(primary, maxPrimary)
	if secondary != "" {
		line += lipgloss.NewStyle().Foreground(p.muted).Render("  " + truncate(secondary, max(10, contentWidth-maxPrimary-6)))
	}
	return style.Render(line)
}

func (m Model) footerView(p palette) string {
	text := "a queue   x remove   f like   space play/pause   ? help"
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
		"SONICLI · KEYBOARD", "", "tab / shift+tab   next / previous view", "j k / arrows       move selection", "enter              play, open, or select", "/                  search", "space              play / pause", "n / p              next / previous track", "h / l              seek -10s / +10s", "+ / -              volume", "m                  mute / unmute", "s / r              shuffle / repeat", "a                  add track to queue", "x                  remove upcoming track*", "f                  like / unlike track", "g                  go to now playing", "u                  refresh current view", "d                  choose playback device", "[ / ]              library section", "esc                back / close", "q                  quit", "", "*Session-local: Sonicli auto-skips it when reached.", "Press ? or q to close help.",
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
	remaining := max(0, duration-progress)
	return fmt.Sprintf("%s  %s  -%s", formatDuration(progress), bar, formatDuration(remaining))
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
func emptyAs(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
func nonEmpty(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			result = append(result, value)
		}
	}
	return result
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
func spreadText(left, right string, width int) string {
	gap := max(1, width-lipgloss.Width(left)-lipgloss.Width(right))
	return left + strings.Repeat(" ", gap) + right
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
