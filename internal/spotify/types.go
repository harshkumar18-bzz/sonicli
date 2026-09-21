package spotify

import "time"

type Image struct {
	URL    string `json:"url"`
	Height int    `json:"height"`
	Width  int    `json:"width"`
}

type Artist struct {
	ID   string `json:"id"`
	URI  string `json:"uri"`
	Name string `json:"name"`
}

type Album struct {
	ID      string   `json:"id"`
	URI     string   `json:"uri"`
	Name    string   `json:"name"`
	Artists []Artist `json:"artists"`
	Images  []Image  `json:"images"`
}

type Track struct {
	ID       string   `json:"id"`
	URI      string   `json:"uri"`
	Name     string   `json:"name"`
	Duration int      `json:"duration_ms"`
	Explicit bool     `json:"explicit"`
	Artists  []Artist `json:"artists"`
	Album    Album    `json:"album"`
}

func (t Track) ArtistNames() string {
	var result string
	for i, artist := range t.Artists {
		if i > 0 {
			result += ", "
		}
		result += artist.Name
	}
	return result
}

type Playlist struct {
	ID          string  `json:"id"`
	URI         string  `json:"uri"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Images      []Image `json:"images"`
	Owner       struct {
		DisplayName string `json:"display_name"`
	} `json:"owner"`
}

type Device struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Active      bool   `json:"is_active"`
	Restricted  bool   `json:"is_restricted"`
	Volume      int    `json:"volume_percent"`
	SupportsVol bool   `json:"supports_volume"`
}

type Playback struct {
	Device    Device    `json:"device"`
	Repeat    string    `json:"repeat_state"`
	Shuffle   bool      `json:"shuffle_state"`
	Playing   bool      `json:"is_playing"`
	Progress  int       `json:"progress_ms"`
	Item      *Track    `json:"item"`
	Timestamp int64     `json:"timestamp"`
	FetchedAt time.Time `json:"-"`
}

func (p Playback) CurrentProgress(now time.Time) int {
	progress := p.Progress
	if p.Playing && !p.FetchedAt.IsZero() {
		progress += int(now.Sub(p.FetchedAt).Milliseconds())
	}
	if p.Item != nil && progress > p.Item.Duration {
		return p.Item.Duration
	}
	if progress < 0 {
		return 0
	}
	return progress
}

type Queue struct {
	CurrentlyPlaying *Track  `json:"currently_playing"`
	Items            []Track `json:"queue"`
}

type Page[T any] struct {
	Items  []T    `json:"items"`
	Next   string `json:"next"`
	Offset int    `json:"offset"`
	Limit  int    `json:"limit"`
	Total  int    `json:"total"`
}

type SavedTrack struct {
	AddedAt time.Time `json:"added_at"`
	Track   Track     `json:"track"`
}

type SavedAlbum struct {
	AddedAt time.Time `json:"added_at"`
	Album   Album     `json:"album"`
}

type PlaylistItem struct {
	AddedAt time.Time `json:"added_at"`
	Item    Track     `json:"item"`
}

type SearchResults struct {
	Tracks    Page[Track]     `json:"tracks"`
	Albums    Page[Album]     `json:"albums"`
	Artists   Page[Artist]    `json:"artists"`
	Playlists Page[*Playlist] `json:"playlists"`
}

type User struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Product     string `json:"product"`
}
