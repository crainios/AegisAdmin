package authstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	database  *sql.DB
	path      string
	secretKey []byte
}

func (s *Store) SetSecretKey(key []byte) error {
	if len(key) != 32 {
		return errors.New("secret key must contain 32 bytes")
	}
	s.secretKey = append([]byte(nil), key...)
	return nil
}

type User struct {
	ID                 int64
	Login              string
	PasswordHash       string
	Type               string
	Status             string
	MustChangePassword bool
	AuthVersion        int64
	TwoFactorRequired  bool
	TOTPSecret         sql.NullString
	TOTPEnabledAt      sql.NullString
}

type NavigationCategory struct {
	ID      int64
	Name    string
	Modules []NavigationModule
}

type NavigationModule struct {
	ID              int64
	Key             string
	Name            string
	Route           string
	Icon            string
	PermissionLevel string
}

func OpenReadOnly(path string) (*Store, error) {
	return open(path, true)
}

func OpenReadWrite(path string) (*Store, error) {
	return open(path, false)
}

func open(path string, readOnly bool) (*Store, error) {
	if !filepath.IsAbs(path) {
		return nil, errors.New("database path must be absolute")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect database: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, errors.New("database must be a regular non-symlink file")
	}

	dsn := (&url.URL{Scheme: "file", Path: path}).String() + "?_pragma=busy_timeout(5000)"
	if readOnly {
		dsn += "&mode=ro&_pragma=query_only(1)"
	} else {
		dsn += "&mode=rw&_pragma=foreign_keys(1)"
	}
	database, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)
	database.SetConnMaxLifetime(5 * time.Minute)
	return &Store{database: database, path: path}, nil
}

func (s *Store) PrepareTOTP(ctx context.Context, id, authVersion int64, secret string) (User, error) {
	const query = `
UPDATE users
SET totp_secret = COALESCE(totp_secret, ?),
    totp_enabled_at = NULL,
    updated_at = ?
WHERE id = ? AND status = 'active' AND auth_version = ? AND totp_enabled_at IS NULL
RETURNING id, login, password_hash, type, status, must_change_password,
          auth_version, two_factor_required, totp_secret, totp_enabled_at`
	return scanRequiredUser(s.database.QueryRowContext(
		ctx, query, secret, time.Now().UTC().Format(time.RFC3339), id, authVersion,
	), "prepare TOTP")
}

func (s *Store) EnableTOTP(ctx context.Context, id, authVersion int64, secret string) (User, error) {
	const query = `
UPDATE users
SET totp_enabled_at = ?,
    auth_version = auth_version + 1,
    updated_at = ?
WHERE id = ? AND status = 'active' AND auth_version = ?
  AND totp_secret = ? AND totp_enabled_at IS NULL
RETURNING id, login, password_hash, type, status, must_change_password,
          auth_version, two_factor_required, totp_secret, totp_enabled_at`
	now := time.Now().UTC().Format(time.RFC3339)
	return scanRequiredUser(s.database.QueryRowContext(
		ctx, query, now, now, id, authVersion, secret,
	), "enable TOTP")
}

func (s *Store) DisableTOTP(ctx context.Context, id, authVersion int64) (User, error) {
	const query = `
UPDATE users
SET totp_secret = NULL, totp_enabled_at = NULL,
    auth_version = auth_version + 1, updated_at = ?
WHERE id = ? AND status = 'active' AND auth_version = ?
  AND two_factor_required = 0 AND totp_enabled_at IS NOT NULL
RETURNING id, login, password_hash, type, status, must_change_password,
          auth_version, two_factor_required, totp_secret, totp_enabled_at`
	return scanRequiredUser(s.database.QueryRowContext(ctx, query, time.Now().UTC().Format(time.RFC3339), id, authVersion), "disable TOTP")
}

func (s *Store) ChangePassword(ctx context.Context, id, authVersion int64, currentHash, newHash string) (User, error) {
	const query = `
UPDATE users
SET password_hash = ?,
    must_change_password = 0,
    auth_version = auth_version + 1,
    updated_at = ?,
    password_changed_at = ?
WHERE id = ? AND status = 'active' AND auth_version = ? AND password_hash = ?
RETURNING id, login, password_hash, type, status, must_change_password,
          auth_version, two_factor_required, totp_secret, totp_enabled_at`
	now := time.Now().UTC().Format(time.RFC3339)
	return scanRequiredUser(s.database.QueryRowContext(
		ctx, query, newHash, now, now, id, authVersion, currentHash,
	), "change password")
}

func (s *Store) Close() error {
	return s.database.Close()
}

func (s *Store) Check(ctx context.Context) error {
	const query = `
SELECT COUNT(*)
FROM pragma_table_info('users')
WHERE name IN (
    'id', 'login', 'password_hash', 'type', 'status',
    'must_change_password', 'auth_version', 'two_factor_required',
    'totp_secret', 'totp_enabled_at'
)`
	var columns int
	if err := s.database.QueryRowContext(ctx, query).Scan(&columns); err != nil {
		return fmt.Errorf("read users schema: %w", err)
	}
	if columns != 10 {
		return fmt.Errorf("users schema is incomplete: %d/10 columns", columns)
	}
	return nil
}

func (s *Store) FindByLogin(ctx context.Context, login string) (User, bool, error) {
	return s.findOne(ctx, "login = ? COLLATE NOCASE", login)
}

func (s *Store) FindRoot(ctx context.Context) (User, bool, error) {
	return s.findOne(ctx, "type = 'root'", nil)
}

func (s *Store) FindByID(ctx context.Context, id int64) (User, bool, error) {
	return s.findOne(ctx, "id = ?", id)
}

func (s *Store) Menu(ctx context.Context, user User) ([]NavigationCategory, error) {
	isRoot := 0
	if user.Type == "root" {
		isRoot = 1
	}
	const query = `
SELECT c.id, c.name, m.id, m.module_key, m.name, m.route, m.icon,
       CASE
           WHEN ? = 1 THEN 'modify'
           WHEN m.module_key = 'about' THEN 'view'
           ELSE ump.permission_level
       END AS permission_level
FROM navigation_categories c
INNER JOIN navigation_modules m ON m.category_id = c.id
LEFT JOIN user_module_permissions ump
       ON ump.user_id = ? AND ump.module_id = m.id
WHERE m.is_enabled = 1
  AND (
      ? = 1
      OR (
          m.access_policy = 'view'
          AND (m.module_key = 'about' OR ump.permission_level IS NOT NULL)
      )
  )
ORDER BY c.position, c.id, m.position, m.id`
	rows, err := s.database.QueryContext(ctx, query, isRoot, user.ID, isRoot)
	if err != nil {
		return nil, fmt.Errorf("read navigation menu: %w", err)
	}
	defer rows.Close()
	categories := make([]NavigationCategory, 0)
	index := make(map[int64]int)
	for rows.Next() {
		var categoryID int64
		var categoryName string
		var module NavigationModule
		if err = rows.Scan(
			&categoryID, &categoryName, &module.ID, &module.Key, &module.Name,
			&module.Route, &module.Icon, &module.PermissionLevel,
		); err != nil {
			return nil, fmt.Errorf("scan navigation menu: %w", err)
		}
		position, exists := index[categoryID]
		if !exists {
			position = len(categories)
			index[categoryID] = position
			categories = append(categories, NavigationCategory{ID: categoryID, Name: categoryName})
		}
		categories[position].Modules = append(categories[position].Modules, module)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate navigation menu: %w", err)
	}
	return categories, nil
}

func (s *Store) PermissionForModule(ctx context.Context, user User, moduleKey string) (string, bool, error) {
	if user.Type == "root" {
		var enabled int
		err := s.database.QueryRowContext(ctx, `
SELECT is_enabled FROM navigation_modules WHERE module_key = ? LIMIT 1`, moduleKey).Scan(&enabled)
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, nil
		}
		if err != nil {
			return "", false, fmt.Errorf("read root module permission: %w", err)
		}
		if enabled != 1 {
			return "", false, nil
		}
		return "modify", true, nil
	}
	var level string
	err := s.database.QueryRowContext(ctx, `
SELECT CASE WHEN m.module_key = 'about' THEN 'view' ELSE ump.permission_level END
FROM navigation_modules m
LEFT JOIN user_module_permissions ump ON ump.user_id = ? AND ump.module_id = m.id
WHERE m.module_key = ? AND m.is_enabled = 1 AND m.access_policy = 'view'
  AND (m.module_key = 'about' OR ump.permission_level IS NOT NULL)
LIMIT 1`, user.ID, moduleKey).Scan(&level)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read module permission: %w", err)
	}
	return level, true, nil
}

func (s *Store) findOne(ctx context.Context, predicate string, argument any) (User, bool, error) {
	query := `
SELECT id, login, password_hash, type, status, must_change_password,
       auth_version, two_factor_required, totp_secret, totp_enabled_at
FROM users
WHERE ` + predicate + `
LIMIT 1`
	var row *sql.Row
	if argument == nil {
		row = s.database.QueryRowContext(ctx, query)
	} else {
		row = s.database.QueryRowContext(ctx, query, argument)
	}
	user, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, false, nil
	}
	if err != nil {
		return User{}, false, fmt.Errorf("find user: %w", err)
	}
	return user, true, nil
}

type rowScanner interface {
	Scan(...any) error
}

func scanUser(row rowScanner) (User, error) {
	var user User
	var mustChangePassword, twoFactorRequired int
	err := row.Scan(
		&user.ID, &user.Login, &user.PasswordHash, &user.Type, &user.Status,
		&mustChangePassword, &user.AuthVersion, &twoFactorRequired,
		&user.TOTPSecret, &user.TOTPEnabledAt,
	)
	if err != nil {
		return User{}, err
	}
	user.MustChangePassword = mustChangePassword == 1
	user.TwoFactorRequired = twoFactorRequired == 1
	return user, nil
}

func scanRequiredUser(row rowScanner, action string) (User, error) {
	user, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, fmt.Errorf("%s: account state changed", action)
	}
	if err != nil {
		return User{}, fmt.Errorf("%s: %w", action, err)
	}
	return user, nil
}
