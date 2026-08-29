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
