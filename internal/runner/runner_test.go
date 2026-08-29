package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestActionCommandWhitelist(t *testing.T) {
	if _, err := ActionCommand("not-allowed", nil); err == nil {
		t.Fatal("unsupported action was accepted")
	}
	if _, err := ActionCommand("core.update", map[string]any{"version": "1.14.0;id"}); err == nil {
		t.Fatal("unsafe version was accepted")
	}
	args, err := ActionCommand("hy2-buffer.enable", nil)
	if err != nil || len(args) != 3 || args[0] != "hy2" {
		t.Fatalf("unexpected action command: %#v, %v", args, err)
	}
}

func TestRunnerTimeout(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fake-sb")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nsleep 2\n"), 0755); err != nil {
		t.Fatal(err)
	}
	r := Runner{Path: path, Timeout: 30 * time.Millisecond}
	result, err := r.Run(context.Background(), "status")
	if err == nil || !result.TimedOut {
		t.Fatalf("expected timeout, result=%#v err=%v", result, err)
	}
}
