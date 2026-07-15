package handler

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// TestChecksumParityWithRust verifies the native Go checksum is byte-identical
// to the canonical Rust binary. If the Rust binary isn't built, the test is
// skipped (native fallback is what runs in that case anyway).
func TestChecksumParityWithRust(t *testing.T) {
	bin := rustHashBinary(t)
	if bin == "" {
		t.Skip("rust kelma-hash binary not built; skipping parity check")
	}

	cases := [][]any{
		{[]string{"a", "b"}, []string{"x"}},
		{[]string{"<img src=\"x.png\"> front", "back"}, []string{}},
		{[]string{"/supɔʁte/", "café", "naïve"}, []string{"tag1", "tag2"}},
		{"Basic", map[string]any{"fields": []string{"Front", "Back"}, "id": 1}},
		{map[string]any{"new_per_day": 20, "name": "Default"}},
		{"note-guid", "Default", 0},
	}

	for i, parts := range cases {
		native := checksumNative(parts...)
		payload, err := json.Marshal(map[string]any{"parts": parts})
		if err != nil {
			t.Fatalf("case %d: marshal: %v", i, err)
		}
		cmd := exec.Command(bin)
		cmd.Stdin = bytes.NewReader(payload)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("case %d: binary: %v", i, err)
		}
		viaBin := string(bytes.TrimSpace(out))
		if native != viaBin {
			t.Errorf("case %d: native=%s rust=%s", i, native, viaBin)
		}
	}
}

func rustHashBinary(t *testing.T) string {
	t.Helper()
	name := "kelma-hash-" + runtime.GOOS + "-"
	switch runtime.GOARCH {
	case "arm64":
		name += "arm64"
	case "amd64":
		name += "amd64"
	default:
		return ""
	}
	p := filepath.Join("..", "..", "clients", "rust", "kelma-hash", "bin", name)
	if _, err := os.Stat(p); err == nil {
		abs, _ := filepath.Abs(p)
		return abs
	}
	return ""
}
