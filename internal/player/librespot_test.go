package player

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLibrespotArgumentsAndPairingState(t *testing.T) {
	root := t.TempDir()
	local := &Librespot{
		Binary:   "/usr/bin/librespot",
		DataDir:  filepath.Join(root, "data"),
		CacheDir: filepath.Join(root, "cache"),
		Name:     "Sonicli",
	}
	want := []string{
		"--name", "Sonicli", "--device-type", "computer", "--bitrate", "320",
		"--autoplay", "on",
		"--cache", local.CacheDir, "--system-cache", local.DataDir, "--enable-oauth",
	}
	if got := local.args(true); !reflect.DeepEqual(got, want) {
		t.Fatalf("args() = %#v, want %#v", got, want)
	}
	if local.Paired() {
		t.Fatal("new cache reported as paired")
	}
	if err := local.prepareDirs(); err != nil {
		t.Fatal(err)
	}
	credentials := filepath.Join(local.DataDir, "credentials.json")
	if err := os.WriteFile(credentials, []byte("credential"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !local.Paired() {
		t.Fatal("credential cache not detected")
	}
	if err := local.secureCredentials(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(credentials)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("credential permissions = %o", info.Mode().Perm())
	}
}
