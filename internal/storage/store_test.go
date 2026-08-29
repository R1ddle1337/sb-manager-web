package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/R1ddle1337/sb-manager-web/internal/types"
)

func TestCancelAndCloneTask(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "web.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	task := types.Task{ID: "task_failed", ServerID: types.ServerLocal, Action: "bbr.enable", Status: types.TaskPending, CreatedAt: time.Now().UTC()}
	if err := store.PutTask(task); err != nil {
		t.Fatal(err)
	}
	canceled, err := store.CancelTask(task.ID)
	if err != nil || canceled.Status != types.TaskCanceled || !canceled.CancelRequested {
		t.Fatalf("cancel result: %#v %v", canceled, err)
	}
	pending := types.Task{ID: "task_pending", ServerID: types.ServerLocal, Action: "bbr.enable", Status: types.TaskPending, CreatedAt: time.Now().UTC()}
	if err := store.PutTask(pending); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CloneTask(pending.ID, "pending-retry", "digest"); err == nil {
		t.Fatal("pending task unexpectedly retryable")
	}
	if _, err := store.CloneTask(canceled.ID, "retry-key", "digest"); err != nil {
		t.Fatal(err)
	}
	duplicate, err := store.CloneTask(canceled.ID, "retry-key", "digest")
	if err != nil || duplicate.IdempotencyKey != "retry-key" {
		t.Fatalf("retry idempotency failed: %#v %v", duplicate, err)
	}
}

func TestRecoverRunningTasks(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "web.db"))
	if err != nil {
		t.Fatal(err)
	}
	task := types.Task{ID: "task_running", ServerID: types.ServerLocal, Action: "health.check", Status: types.TaskRunning, CreatedAt: time.Now().UTC(), Attempt: 2}
	if err := store.PutTask(task); err != nil {
		t.Fatal(err)
	}
	if err := store.RecoverRunningTasks(); err != nil {
		t.Fatal(err)
	}
	recovered, err := store.GetTask(task.ID)
	if err != nil || recovered.Status != types.TaskPending || recovered.Error == "" || recovered.Attempt != 2 {
		t.Fatalf("unexpected recovered task: %#v %v", recovered, err)
	}
	store.Close()
}

func TestSQLiteBackup(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "web.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	backup := filepath.Join(dir, "backups", "copy.db")
	if err := store.Backup(context.Background(), backup); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(backup); err != nil || info.Size() == 0 {
		t.Fatalf("backup was not created: %#v %v", info, err)
	}
}

func TestDeleteUserRevokesSessions(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "web.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.PutUser(types.User{Username: "operator", Hash: "hash", Created: time.Now().UTC(), Role: "operator"}); err != nil {
		t.Fatal(err)
	}
	if err := store.PutSession(types.Session{ID: "session-operator", Username: "operator", CSRF: "csrf", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteUser("operator"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetSession("session-operator"); err == nil {
		t.Fatal("deleted user's session was not revoked")
	}
}
