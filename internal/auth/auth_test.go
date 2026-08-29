package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/R1ddle1337/sb-manager-web/internal/storage"
)

func TestPasswordAndSession(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil || !VerifyPassword(hash, "correct horse battery staple") || VerifyPassword(hash, "wrong password") {
		t.Fatalf("password verification failed: hash=%q err=%v", hash, err)
	}
	if _, err := HashPassword("short"); err == nil {
		t.Fatal("short password was accepted")
	}
	store, err := storage.Open(t.TempDir() + "/web.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	m := New(store)
	session, err := m.NewSession("admin")
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := m.ValidateSession(session.ID); !ok || got.CSRF != session.CSRF {
		t.Fatal("new session did not validate")
	}
	if _, ok := m.ValidateSession("missing"); ok {
		t.Fatal("missing session validated")
	}
	if err := m.CreateUser("operator", "another correct password", "operator"); err != nil {
		t.Fatal(err)
	}
	if got := m.Role("operator"); got != "operator" {
		t.Fatalf("unexpected role: %s", got)
	}
	if err := m.SetPassword("operator", "updated correct password"); err != nil {
		t.Fatal(err)
	}
	if !m.Authenticate("operator", "updated correct password") {
		t.Fatal("operator password update failed")
	}
	_ = time.Now()
}

func TestEnsureOwnerUsesRandomUsername(t *testing.T) {
	store, err := storage.Open(t.TempDir() + "/web.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	manager := New(store)
	credential, created, err := manager.EnsureOwner()
	if err != nil || !created {
		t.Fatalf("owner creation failed: %#v %v", credential, err)
	}
	if !strings.HasPrefix(credential.Username, "owner-") || credential.Username == "admin" || len(credential.Password) < 12 {
		t.Fatalf("credentials were not randomized: %#v", credential)
	}
	second, created, err := manager.EnsureOwner()
	if err != nil || created || second.Username != "" {
		t.Fatalf("owner was created twice: %#v created=%v err=%v", second, created, err)
	}
}
