package authstore

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

var migrationName = regexp.MustCompile(`^[0-9]{3}_[a-z0-9_]+\.sql$`)

func RootInitialized(ctx context.Context, databasePath string) (bool, error) {
	if !filepath.IsAbs(databasePath) {
		return false, errors.New("database path must be absolute")
	}
	info, err := os.Lstat(databasePath)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return false, errors.New("database must be a regular non-symlink file")
	}
	dsn := (&url.URL{Scheme: "file", Path: databasePath, RawQuery: "mode=ro&_pragma=query_only(1)"}).String()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return false, err
	}
	defer db.Close()
	var tableCount int
	if err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='users'`).Scan(&tableCount); err != nil || tableCount == 0 {
		return false, err
	}
	var rootCount int
	if err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE type='root'`).Scan(&rootCount); err != nil {
		return false, err
	}
	return rootCount > 0, nil
}

func Migrate(ctx context.Context, databasePath, migrationsDirectory string) ([]string, error) {
	if !filepath.IsAbs(databasePath) || !filepath.IsAbs(migrationsDirectory) {
		return nil, errors.New("database and migrations paths must be absolute")
	}
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o770); err != nil {
		return nil, err
	}
	dsn := (&url.URL{Scheme: "file", Path: databasePath}).String() + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if _, err = db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY, checksum TEXT NOT NULL, applied_at TEXT NOT NULL)`); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(migrationsDirectory)
	if err != nil {
		return nil, err
	}
	names := []string{}
	for _, entry := range entries {
		if !entry.IsDir() && migrationName.MatchString(entry.Name()) {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	applied := []string{}
	for _, name := range names {
		content, readErr := os.ReadFile(filepath.Join(migrationsDirectory, name))
		if readErr != nil || len(strings.TrimSpace(string(content))) == 0 {
			return nil, fmt.Errorf("read migration %s", name)
		}
		digest := sha256.Sum256(content)
		checksum := hex.EncodeToString(digest[:])
		var existing string
		err = db.QueryRowContext(ctx, `SELECT checksum FROM schema_migrations WHERE name=?`, name).Scan(&existing)
		if err == nil {
			if existing != checksum {
				return nil, fmt.Errorf("migration already applied was modified: %s", name)
			}
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		tx, beginErr := db.BeginTx(ctx, nil)
		if beginErr != nil {
			return nil, beginErr
		}
		if _, err = tx.ExecContext(ctx, string(content)); err == nil {
			_, err = tx.ExecContext(ctx, `INSERT INTO schema_migrations(name,checksum,applied_at) VALUES(?,?,?)`, name, checksum, time.Now().UTC().Format(time.RFC3339))
		}
		if err != nil {
			_ = tx.Rollback()
			return nil, fmt.Errorf("apply migration %s: %w", name, err)
		}
		if err = tx.Commit(); err != nil {
			return nil, err
		}
		applied = append(applied, name)
	}
	return applied, nil
}

func (s *Store) InitializeRoot(ctx context.Context, firstName, lastName, email, password string) error {
	if err := validateRootIdentity(firstName, lastName, email); err != nil {
		return err
	}
	hash, err := hashAdministrativePassword(password)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.database.ExecContext(ctx, `INSERT INTO users(login,password_hash,type,status,can_view,can_act,can_modify,must_change_password,auth_version,created_at,updated_at,password_changed_at,first_name,last_name,email,two_factor_required) VALUES('root',?,'root','active',1,1,1,0,1,?,?,?,?,?,?,0)`, string(hash), now, now, now, strings.TrimSpace(firstName), strings.TrimSpace(lastName), strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		return fmt.Errorf("initialize root: %w", err)
	}
	return nil
}

func (s *Store) ResetRootPassword(ctx context.Context, password string) error {
	hash, err := hashAdministrativePassword(password)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.database.ExecContext(ctx, `UPDATE users SET password_hash=?,must_change_password=0,auth_version=auth_version+1,updated_at=?,password_changed_at=? WHERE type='root'`, string(hash), now, now)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		return errors.New("root account is not initialized")
	}
	return nil
}

// DisableRootTwoFactor clears the root TOTP enrollment and invalidates every
// session authenticated with the previous account version.
func (s *Store) DisableRootTwoFactor(ctx context.Context) error {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.database.ExecContext(ctx, `UPDATE users SET two_factor_required=0,totp_secret=NULL,totp_enabled_at=NULL,auth_version=auth_version+1,updated_at=? WHERE type='root'`, now)
	if err != nil {
		return fmt.Errorf("disable root two-factor authentication: %w", err)
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		return errors.New("root account is not initialized")
	}
	return nil
}

func hashAdministrativePassword(password string) ([]byte, error) {
	length := utf8.RuneCountInString(password)
	if length < 12 || length > 72 {
		return nil, errors.New("password must contain between 12 and 72 characters")
	}
	return bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
}

func validateRootIdentity(firstName, lastName, email string) error {
	for _, value := range []string{firstName, lastName} {
		length := utf8.RuneCountInString(strings.TrimSpace(value))
		if length < 1 || length > 100 {
			return errors.New("first name and last name must contain between 1 and 100 characters")
		}
	}
	address := strings.ToLower(strings.TrimSpace(email))
	parsed, err := mail.ParseAddress(address)
	if err != nil || parsed.Address != address || len(address) > 254 {
		return errors.New("invalid email address")
	}
	return nil
}
