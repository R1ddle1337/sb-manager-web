package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStateSnapshotDigestAndSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte(`{"schema_version":2,"nodes":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	digest, schema, err := stateSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(digest) != 64 || schema != 2 {
		t.Fatalf("unexpected snapshot: digest=%q schema=%d", digest, schema)
	}
	if _, _, err := stateSnapshot(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing state file was accepted")
	}
}

func TestControllerURLRequiresTLSRemotely(t *testing.T) {
	for _, test := range []struct {
		url string
		ok  bool
	}{
		{"https://panel.example.com/", true},
		{"http://127.0.0.1:9091", true},
		{"http://panel.example.com", false},
		{"https://panel.example.com/path", false},
	} {
		if got := validateControllerURL(test.url) == nil; got != test.ok {
			t.Fatalf("validateControllerURL(%q)=%v, want %v", test.url, got, test.ok)
		}
	}
}
