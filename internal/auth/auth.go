package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/R1ddle1337/sb-manager-web/internal/storage"
	"github.com/R1ddle1337/sb-manager-web/internal/types"
	"golang.org/x/crypto/argon2"
)

type Manager struct{ store *storage.Store }

func New(store *storage.Store) *Manager { return &Manager{store: store} }

func randomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	return b, err
}

func RandomToken(n int) (string, error) {
	b, err := randomBytes(n)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func HashPassword(password string) (string, error) {
	if len(password) < 12 {
		return "", errors.New("password must contain at least 12 characters")
	}
	salt, err := randomBytes(16)
	if err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password), salt, 1, 64*1024, 2, 32)
	return fmt.Sprintf("argon2id$v=19$m=65536,t=1,p=2$%s$%s",
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(hash)), nil
}

func VerifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 5 || parts[0] != "argon2id" || parts[1] != "v=19" {
		return false
	}
	salt, err1 := base64.RawStdEncoding.DecodeString(parts[3])
	expected, err2 := base64.RawStdEncoding.DecodeString(parts[4])
	if err1 != nil || err2 != nil {
		return false
	}
	actual := argon2.IDKey([]byte(password), salt, 1, 64*1024, 2, uint32(len(expected)))
	return subtle.ConstantTimeCompare(actual, expected) == 1
}

func (m *Manager) EnsureAdmin() (string, bool, error) {
	has, err := m.store.HasUsers()
	if err != nil || has {
		return "", false, err
	}
	password, err := RandomToken(18)
	if err != nil {
		return "", false, err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return "", false, err
	}
	err = m.store.PutUser(types.User{Username: "admin", Hash: hash, Created: time.Now().UTC()})
	return password, true, err
}

func (m *Manager) Authenticate(username, password string) bool {
	user, err := m.store.GetUser(username)
	return err == nil && VerifyPassword(user.Hash, password)
}

func (m *Manager) SetPassword(username, password string) error {
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	return m.store.PutUser(types.User{Username: username, Hash: hash, Created: time.Now().UTC()})
}

func (m *Manager) NewSession(username string) (types.Session, error) {
	id, err := RandomToken(32)
	if err != nil {
		return types.Session{}, err
	}
	csrf, err := RandomToken(24)
	if err != nil {
		return types.Session{}, err
	}
	session := types.Session{ID: id, CSRF: csrf, Username: username, ExpiresAt: time.Now().UTC().Add(12 * time.Hour)}
	return session, m.store.PutSession(session)
}

func (m *Manager) ValidateSession(id string) (types.Session, bool) {
	session, err := m.store.GetSession(id)
	if err != nil || session.ExpiresAt.Before(time.Now().UTC()) {
		return types.Session{}, false
	}
	return session, true
}

func (m *Manager) DeleteSession(id string) error { return m.store.DeleteSession(id) }

func HashEnrollmentToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(h[:])
}
