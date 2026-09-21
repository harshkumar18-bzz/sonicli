package main

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/harshkumar18-bzz/sonicli/internal/spotify"
)

func TestHelpVersionAndCompletion(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"--help"}, "minimal Spotify terminal player"},
		{[]string{"--version"}, "sonicli dev"},
		{[]string{"completion", "bash"}, "player"},
		{[]string{"completion", "zsh"}, "#compdef sonicli"},
		{[]string{"completion", "fish"}, "complete -c sonicli"},
	}
	for _, tt := range tests {
		var out bytes.Buffer
		if err := run(tt.args, strings.NewReader(""), &out, &out); err != nil {
			t.Fatalf("run(%v): %v", tt.args, err)
		}
		if !strings.Contains(out.String(), tt.want) {
			t.Errorf("run(%v) output %q does not contain %q", tt.args, out.String(), tt.want)
		}
	}
}

func TestSelectedLibrespotFailsBeforeOpeningTUIWhenMissing(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	api, stop, err := startLocalPlayer("librespot", spotify.New(nil), io.Discard)
	if stop != nil {
		stop()
	}
	if err == nil || api != nil || !strings.Contains(err.Error(), "install it") {
		t.Fatalf("startLocalPlayer() = %T, %v", api, err)
	}
}

func TestInvalidCommand(t *testing.T) {
	if err := run([]string{"wat"}, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
		t.Fatal("unknown command succeeded")
	}
}
