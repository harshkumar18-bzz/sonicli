package player

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/harshkumar18-bzz/sonicli/internal/spotify"
)

type fakeController struct {
	active bool
	calls  [][]string
}

type staticTokens struct{}

func (staticTokens) AccessToken(context.Context, bool) (string, error) { return "token", nil }

func TestTargetConnectDevice(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.RequestURI())
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	client := spotify.New(staticTokens{})
	client.BaseURL = server.URL
	api := &API{Client: client, TargetDeviceID: "local-device"}
	ctx := context.Background()
	if err := api.Play(ctx, []string{"spotify:track:1"}, "", "old-device"); err != nil {
		t.Fatal(err)
	}
	if err := api.Pause(ctx, "old-device"); err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 || !strings.Contains(paths[0], "device_id=local-device") || !strings.Contains(paths[1], "device_id=local-device") {
		t.Fatalf("targeted requests = %#v", paths)
	}
	if err := api.Transfer(ctx, "chosen-device", false); err != nil {
		t.Fatal(err)
	}
	if err := api.Next(ctx, "local-device"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(paths[len(paths)-1], "device_id=chosen-device") {
		t.Fatalf("request after transfer = %q", paths[len(paths)-1])
	}
}

func TestWaitForDevice(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"devices":[{"id":"d1","name":"Sonicli","is_restricted":false}]}`)
	}))
	defer server.Close()
	client := spotify.New(staticTokens{})
	client.BaseURL = server.URL
	device, err := WaitForDevice(context.Background(), client, "Sonicli")
	if err != nil || device.ID != "d1" {
		t.Fatalf("WaitForDevice() = %#v, %v", device, err)
	}
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
