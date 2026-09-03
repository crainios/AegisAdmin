package websession

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSessionLifecycleAndRotation(t *testing.T) {
	manager, err := New(DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	manager.now = func() time.Time { return now }

	anonymous, err := manager.Create(StateAnonymous, 0, 0)
	if err != nil || !manager.ValidateCSRF(anonymous.ID, anonymous.CSRFToken) {
		t.Fatalf("anonymous session failed: %#v, %v", anonymous, err)
	}
	authenticated, err := manager.Rotate(anonymous.ID, StateAuthenticated, 42, 3)
	if err != nil || authenticated.ID == anonymous.ID {
		t.Fatalf("rotation failed: %#v, %v", authenticated, err)
	}
	if _, found := manager.Get(anonymous.ID); found {
		t.Fatal("old session survived rotation")
	}
	if current, found := manager.Get(authenticated.ID); !found || current.UserID != 42 || current.AuthVersion != 3 {
		t.Fatalf("authenticated session missing: %#v", current)
	}
}

func TestAuthenticatedSessionSurvivesRestartAndLogoutDoesNot(t *testing.T) {
	directory := t.TempDir()
	if err := os.Chmod(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "sessions.json")
	manager, err := NewPersistent(DefaultConfig(), path)
	if err != nil {
		t.Fatal(err)
	}
	pending, err := manager.Create(StateTwoFactor, 7, 2)
	if err != nil {
		t.Fatal(err)
	}
	authenticated, err := manager.Rotate(pending.ID, StateAuthenticated, 7, 2)
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := NewPersistent(DefaultConfig(), path)
	if err != nil {
		t.Fatal(err)
	}
	if session, found := restarted.Get(authenticated.ID); !found || session.UserID != 7 || session.State != StateAuthenticated {
		t.Fatalf("restored session = %#v, found=%v", session, found)
	}
	if err := restarted.Destroy(authenticated.ID); err != nil {
		t.Fatal(err)
	}
	afterLogout, err := NewPersistent(DefaultConfig(), path)
	if err != nil {
		t.Fatal(err)
	}
	if _, found := afterLogout.Get(authenticated.ID); found {
		t.Fatal("logged-out session survived restart")
	}
}

func TestPendingSessionIsNotRestored(t *testing.T) {
	directory := t.TempDir()
	if err := os.Chmod(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "sessions.json")
	manager, err := NewPersistent(DefaultConfig(), path)
	if err != nil {
		t.Fatal(err)
	}
	pending, err := manager.Create(StateTwoFactor, 7, 2)
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := NewPersistent(DefaultConfig(), path)
	if err != nil {
		t.Fatal(err)
	}
	if _, found := restarted.Get(pending.ID); found {
		t.Fatal("pending session survived restart")
	}
}

func TestSessionExpirationAndCapacity(t *testing.T) {
	config := DefaultConfig()
	config.MaximumSessions = 1
	manager, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	manager.now = func() time.Time { return now }
	first, err := manager.Create(StateTwoFactor, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = manager.Create(StateAnonymous, 0, 0); !errors.Is(err, ErrCapacity) {
		t.Fatalf("capacity error = %v", err)
	}
	now = now.Add(config.PendingTimeout + time.Second)
	if _, found := manager.Get(first.ID); found {
		t.Fatal("expired session still exists")
	}
	if _, err = manager.Create(StateAnonymous, 0, 0); err != nil {
		t.Fatalf("expired slot was not reclaimed: %v", err)
	}
}

func TestCookieSecurityAttributes(t *testing.T) {
	cookie := Cookie("token")
	if !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || cookie.Path != "/" || cookie.Domain != "" {
		t.Fatalf("unsafe cookie: %#v", cookie)
	}
}
