package websession

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"
)

const CookieName = "__Host-aegisadmin_session"

type State string

const (
	StateAnonymous      State = "anonymous"
	StatePassword       State = "password_change"
	StateTwoFactor      State = "two_factor_challenge"
	StateTwoFactorSetup State = "two_factor_enrollment"
	StateAuthenticated  State = "authenticated"
)

var ErrCapacity = errors.New("session capacity reached")

type Config struct {
	IdleTimeout     time.Duration
	AbsoluteTimeout time.Duration
	PendingTimeout  time.Duration
	MaximumSessions int
}

func DefaultConfig() Config {
	return Config{
		IdleTimeout: 30 * time.Minute, AbsoluteTimeout: 8 * time.Hour,
		PendingTimeout: 5 * time.Minute, MaximumSessions: 10_000,
	}
}

type Session struct {
	ID          string
	CSRFToken   string
	State       State
	UserID      int64
	AuthVersion int64
	CreatedAt   time.Time
	LastSeenAt  time.Time
}

type Manager struct {
	mu       sync.Mutex
	sessions map[string]Session
	config   Config
	now      func() time.Time
	random   io.Reader
}

func New(config Config) (*Manager, error) {
	if config.IdleTimeout <= 0 || config.AbsoluteTimeout <= 0 ||
		config.PendingTimeout <= 0 || config.MaximumSessions < 1 {
		return nil, errors.New("invalid session configuration")
	}
	return &Manager{
		sessions: make(map[string]Session), config: config,
		now: time.Now, random: rand.Reader,
	}, nil
}

func (m *Manager) Create(state State, userID, authVersion int64) (Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removeExpiredLocked()
	if len(m.sessions) >= m.config.MaximumSessions {
		return Session{}, ErrCapacity
	}
	return m.createLocked(state, userID, authVersion)
}

func (m *Manager) Rotate(id string, state State, userID, authVersion int64) (Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, id)
	m.removeExpiredLocked()
	if len(m.sessions) >= m.config.MaximumSessions {
		return Session{}, ErrCapacity
	}
	return m.createLocked(state, userID, authVersion)
}

func (m *Manager) Get(id string) (Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	session, found := m.sessions[id]
	if !found || m.expired(session) {
		delete(m.sessions, id)
		return Session{}, false
	}
	session.LastSeenAt = m.now().UTC()
	m.sessions[id] = session
	return session, true
}

func (m *Manager) ValidateCSRF(id, token string) bool {
	session, found := m.Get(id)
	if !found {
		return false
	}
	expected, expectedErr := base64.RawURLEncoding.Strict().DecodeString(session.CSRFToken)
	actual, actualErr := base64.RawURLEncoding.Strict().DecodeString(token)
	return expectedErr == nil && actualErr == nil && len(expected) == 32 && len(actual) == 32 &&
		subtle.ConstantTimeCompare(expected, actual) == 1
}

func (m *Manager) Destroy(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, id)
}

func Cookie(id string) *http.Cookie {
	return &http.Cookie{
		Name: CookieName, Value: id, Path: "/", Secure: true, HttpOnly: true,
		SameSite: http.SameSiteStrictMode, MaxAge: 0,
	}
}

func ExpiredCookie() *http.Cookie {
	cookie := Cookie("")
	cookie.MaxAge = -1
	cookie.Expires = time.Unix(1, 0).UTC()
	return cookie
}

func (m *Manager) createLocked(state State, userID, authVersion int64) (Session, error) {
	if state != StateAnonymous && state != StatePassword && state != StateTwoFactor &&
		state != StateTwoFactorSetup && state != StateAuthenticated {
		return Session{}, errors.New("invalid session state")
	}
	if (state == StateAnonymous && (userID != 0 || authVersion != 0)) ||
		(state != StateAnonymous && (userID < 1 || authVersion < 1)) {
		return Session{}, errors.New("invalid session identity")
	}
	for attempts := 0; attempts < 4; attempts++ {
		id, err := randomToken(m.random)
		if err != nil {
			return Session{}, err
		}
		if _, exists := m.sessions[id]; exists {
			continue
		}
		csrf, err := randomToken(m.random)
		if err != nil {
			return Session{}, err
		}
		now := m.now().UTC()
		session := Session{
			ID: id, CSRFToken: csrf, State: state, UserID: userID,
			AuthVersion: authVersion, CreatedAt: now, LastSeenAt: now,
		}
		m.sessions[id] = session
		return session, nil
	}
	return Session{}, errors.New("unable to allocate unique session")
}

func randomToken(source io.Reader) (string, error) {
	value := make([]byte, 32)
	if _, err := io.ReadFull(source, value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func (m *Manager) expired(session Session) bool {
	now := m.now().UTC()
	absolute := m.config.AbsoluteTimeout
	if session.State != StateAuthenticated {
		absolute = m.config.PendingTimeout
	}
	return now.Sub(session.CreatedAt) > absolute || now.Sub(session.LastSeenAt) > m.config.IdleTimeout
}

func (m *Manager) removeExpiredLocked() {
	for id, session := range m.sessions {
		if m.expired(session) {
			delete(m.sessions, id)
		}
	}
}
