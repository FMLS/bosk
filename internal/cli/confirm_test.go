package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeStdin returns a readable file holding the given answer. A regular file
// is not a character device, which is how the confirmer detects that there is
// no one to prompt.
func writeStdin(t *testing.T, content string) *os.File {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stdin")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

func TestConfirmerRefusesWhenThereIsNoTerminal(t *testing.T) {
	out := &bytes.Buffer{}
	confirmer := newTerminalConfirmer(writeStdin(t, "yes\n"), out)

	ok, err := confirmer.Confirm("Delete? [y/N] ")

	if ok {
		t.Error("a redirected stdin must not count as approval")
	}
	if !errors.Is(err, errNotInteractive) {
		t.Fatalf("error = %v, want errNotInteractive", err)
	}
	if out.Len() != 0 {
		t.Errorf("nothing should be prompted, got %q", out.String())
	}
}

func TestConfirmerPromptsOnATerminal(t *testing.T) {
	devTTY, err := os.OpenFile(os.DevNull, os.O_RDONLY, 0)
	if err != nil {
		t.Skip("no character device available")
	}
	defer devTTY.Close()
	out := &bytes.Buffer{}
	confirmer := newTerminalConfirmer(devTTY, out)

	ok, err := confirmer.Confirm("Delete? [y/N] ")

	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("an empty answer means no")
	}
	if !strings.Contains(out.String(), "Delete? [y/N]") {
		t.Errorf("prompt = %q", out.String())
	}
}

func TestIsCharDevice(t *testing.T) {
	if isCharDevice(writeStdin(t, "")) {
		t.Error("a regular file is not a character device")
	}
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Skip("no null device")
	}
	defer devNull.Close()
	if !isCharDevice(devNull) {
		t.Error("the null device is a character device")
	}
}
