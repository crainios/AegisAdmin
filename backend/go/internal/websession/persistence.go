package websession

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const (
	persistenceVersion = 1
	maximumFileSize    = 16 * 1024 * 1024
)

type persistedSessions struct {
	Version  int                `json:"version"`
	Sessions []persistedSession `json:"sessions"`
}

type persistedSession struct {
	ID          string    `json:"id"`
	CSRFToken   string    `json:"csrf_token"`
	UserID      int64     `json:"user_id"`
	AuthVersion int64     `json:"auth_version"`
	CreatedAt   time.Time `json:"created_at"`
	LastSeenAt  time.Time `json:"last_seen_at"`
}

// NewPersistent crée un gestionnaire dont les sessions authentifiées
// survivent aux redémarrages du serveur web. Les étapes anonymes, 2FA et de
// changement de mot de passe restent délibérément en mémoire uniquement.
func NewPersistent(config Config, path string) (*Manager, error) {
	manager, err := New(config)
	if err != nil {
		return nil, err
	}
	if !filepath.IsAbs(path) {
		return nil, errors.New("session persistence path must be absolute")
	}
	directory := filepath.Dir(path)
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("session persistence directory is unsafe")
	}
	manager.persistencePath = path
	if err := manager.loadPersistent(); err != nil {
		return nil, err
	}
	return manager, nil
}

func (m *Manager) loadPersistent() error {
	info, err := os.Lstat(m.persistencePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 || info.Size() > maximumFileSize {
		return errors.New("persistent session file is unsafe")
	}
	file, err := os.Open(m.persistencePath)
	if err != nil {
		return fmt.Errorf("open persistent sessions: %w", err)
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, maximumFileSize+1))
	decoder.DisallowUnknownFields()
	var stored persistedSessions
	if err := decoder.Decode(&stored); err != nil || stored.Version != persistenceVersion || len(stored.Sessions) > m.config.MaximumSessions {
		return errors.New("persistent session file is invalid")
	}
	if err := ensureJSONEnd(decoder); err != nil {
		return errors.New("persistent session file contains trailing data")
	}
	now := m.now().UTC()
	for _, item := range stored.Sessions {
		session := Session{ID: item.ID, CSRFToken: item.CSRFToken, State: StateAuthenticated, UserID: item.UserID, AuthVersion: item.AuthVersion, CreatedAt: item.CreatedAt.UTC(), LastSeenAt: item.LastSeenAt.UTC()}
		if !validPersistentSession(session, now) {
			return errors.New("persistent session record is invalid")
		}
		if _, duplicate := m.sessions[session.ID]; duplicate {
			return errors.New("persistent session identifier is duplicated")
		}
		if !m.expired(session) {
			m.sessions[session.ID] = session
		}
	}
	m.lastPersistence = now
	return nil
}

func ensureJSONEnd(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("unexpected JSON data")
	}
	return nil
}

func validPersistentSession(session Session, now time.Time) bool {
	return validToken(session.ID) && validToken(session.CSRFToken) && session.UserID > 0 && session.AuthVersion > 0 &&
		!session.CreatedAt.IsZero() && !session.LastSeenAt.IsZero() && !session.LastSeenAt.Before(session.CreatedAt) &&
		!session.CreatedAt.After(now.Add(time.Minute)) && !session.LastSeenAt.After(now.Add(time.Minute))
}

func validToken(value string) bool {
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(value)
	return err == nil && len(decoded) == 32
}

func (m *Manager) persistLocked() error {
	if m.persistencePath == "" {
		return nil
	}
	items := make([]persistedSession, 0, len(m.sessions))
	for _, session := range m.sessions {
		if session.State != StateAuthenticated || m.expired(session) {
			continue
		}
		items = append(items, persistedSession{session.ID, session.CSRFToken, session.UserID, session.AuthVersion, session.CreatedAt.UTC(), session.LastSeenAt.UTC()})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	directory := filepath.Dir(m.persistencePath)
	temporary, err := os.CreateTemp(directory, ".sessions-*.tmp")
	if err != nil {
		return fmt.Errorf("create persistent sessions: %w", err)
	}
	temporaryPath := temporary.Name()
	removeTemporary := true
	defer func() {
		_ = temporary.Close()
		if removeTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return err
	}
	encoder := json.NewEncoder(temporary)
	if err := encoder.Encode(persistedSessions{Version: persistenceVersion, Sessions: items}); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, m.persistencePath); err != nil {
		return fmt.Errorf("replace persistent sessions: %w", err)
	}
	removeTemporary = false
	directoryHandle, err := os.Open(directory)
	if err != nil {
		return fmt.Errorf("open persistent session directory: %w", err)
	}
	if err := directoryHandle.Sync(); err != nil {
		_ = directoryHandle.Close()
		return fmt.Errorf("sync persistent session directory: %w", err)
	}
	if err := directoryHandle.Close(); err != nil {
		return fmt.Errorf("close persistent session directory: %w", err)
	}
	m.lastPersistence = m.now().UTC()
	return nil
}
