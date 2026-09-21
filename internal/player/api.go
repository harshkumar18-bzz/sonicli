package player

import (
	"context"
	"strconv"

	"github.com/harshkumar18-bzz/sonicli/internal/spotify"
)

type Controller interface {
	Active() bool
	Control(context.Context, ...string) error
}

// API keeps Spotify's catalog/state API and routes playback commands through
// the local Soloist process whenever it is active.
type API struct {
	*spotify.Client
	Local Controller
}

func (a *API) Play(ctx context.Context, uris []string, contextURI, deviceID string) error {
	if a.localActive() {
		uri := contextURI
		if uri == "" && len(uris) > 0 {
			uri = uris[0]
		}
		args := []string{"play"}
		if uri != "" {
			args = append(args, uri)
		}
		return a.Local.Control(ctx, args...)
	}
	return a.Client.Play(ctx, uris, contextURI, deviceID)
}

func (a *API) Pause(ctx context.Context, deviceID string) error {
	if a.localActive() {
		return a.Local.Control(ctx, "pause")
	}
	return a.Client.Pause(ctx, deviceID)
}

func (a *API) Next(ctx context.Context, deviceID string) error {
	if a.localActive() {
		return a.Local.Control(ctx, "next")
	}
	return a.Client.Next(ctx, deviceID)
}

func (a *API) Previous(ctx context.Context, deviceID string) error {
	if a.localActive() {
		return a.Local.Control(ctx, "prev")
	}
	return a.Client.Previous(ctx, deviceID)
}

func (a *API) Seek(ctx context.Context, position int, deviceID string) error {
	if a.localActive() {
		return a.Local.Control(ctx, "seek", strconv.Itoa(max(0, position)))
	}
	return a.Client.Seek(ctx, position, deviceID)
}

func (a *API) Volume(ctx context.Context, volume int, deviceID string) error {
	volume = min(100, max(0, volume))
	if a.localActive() {
		return a.Local.Control(ctx, "volume", strconv.Itoa(volume))
	}
	return a.Client.Volume(ctx, volume, deviceID)
}

func (a *API) Shuffle(ctx context.Context, enabled bool, deviceID string) error {
	if a.localActive() {
		return a.Local.Control(ctx, "shuffle", strconv.FormatBool(enabled))
	}
	return a.Client.Shuffle(ctx, enabled, deviceID)
}

func (a *API) Repeat(ctx context.Context, state, deviceID string) error {
	if a.localActive() {
		return a.Local.Control(ctx, "repeat", state)
	}
	return a.Client.Repeat(ctx, state, deviceID)
}

func (a *API) AddToQueue(ctx context.Context, uri, deviceID string) error {
	if a.localActive() {
		return a.Local.Control(ctx, "add-to-queue", uri)
	}
	return a.Client.AddToQueue(ctx, uri, deviceID)
}

func (a *API) localActive() bool { return a.Local != nil && a.Local.Active() }
