package player

import (
	"context"
	"reflect"
	"testing"
)

type fakeController struct {
	active bool
	calls  [][]string
}

func (f *fakeController) Active() bool { return f.active }
func (f *fakeController) Control(_ context.Context, args ...string) error {
	f.calls = append(f.calls, append([]string(nil), args...))
	return nil
}

func TestLocalPlaybackCommands(t *testing.T) {
	local := &fakeController{active: true}
	api := &API{Local: local}
	ctx := context.Background()
	operations := []func() error{
		func() error { return api.Play(ctx, []string{"spotify:track:1"}, "", "") },
		func() error { return api.Play(ctx, nil, "spotify:playlist:2", "") },
		func() error { return api.Play(ctx, nil, "", "") },
		func() error { return api.Pause(ctx, "") },
		func() error { return api.Next(ctx, "") },
		func() error { return api.Previous(ctx, "") },
		func() error { return api.Seek(ctx, 1234, "") },
		func() error { return api.Volume(ctx, 125, "") },
		func() error { return api.Shuffle(ctx, true, "") },
		func() error { return api.Repeat(ctx, "track", "") },
		func() error { return api.AddToQueue(ctx, "spotify:track:3", "") },
	}
	for i, operation := range operations {
		if err := operation(); err != nil {
			t.Fatalf("operation %d: %v", i, err)
		}
	}
	want := [][]string{
		{"play", "spotify:track:1"}, {"play", "spotify:playlist:2"}, {"play"}, {"pause"},
		{"next"}, {"prev"}, {"seek", "1234"}, {"volume", "100"}, {"shuffle", "true"},
		{"repeat", "track"}, {"add-to-queue", "spotify:track:3"},
	}
	if !reflect.DeepEqual(local.calls, want) {
		t.Fatalf("commands = %#v, want %#v", local.calls, want)
	}
}
