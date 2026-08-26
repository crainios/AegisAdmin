package authstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type AdminUser struct {
	ID                                                      int64
	Login, FirstName, LastName, Email, Type, Status         string
	MustChangePassword, TwoFactorRequired, TwoFactorEnabled bool
	AuthVersion                                             int64
}
type AssignableModule struct {
	ID                  int64
	Key, Name, Category string
}

func (s *Store) AdminUsers(ctx context.Context) ([]AdminUser, error) {
	rows, err := s.database.QueryContext(ctx, `SELECT id,login,first_name,last_name,email,type,status,must_change_password,two_factor_required,totp_enabled_at IS NOT NULL,auth_version FROM users ORDER BY type='root' DESC,login COLLATE NOCASE`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()
	items := []AdminUser{}
	for rows.Next() {
		var u AdminUser
		var must, required, enabled int
		if err = rows.Scan(&u.ID, &u.Login, &u.FirstName, &u.LastName, &u.Email, &u.Type, &u.Status, &must, &required, &enabled, &u.AuthVersion); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		u.MustChangePassword = must == 1
		u.TwoFactorRequired = required == 1
		u.TwoFactorEnabled = enabled == 1
		items = append(items, u)
	}
	return items, rows.Err()
}
func (s *Store) AssignableModules(ctx context.Context) ([]AssignableModule, error) {
	rows, err := s.database.QueryContext(ctx, `SELECT m.id,m.module_key,m.name,c.name FROM navigation_modules m JOIN navigation_categories c ON c.id=m.category_id WHERE m.is_enabled=1 AND m.access_policy='view' AND m.module_key!='about' ORDER BY c.position,c.id,m.position,m.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []AssignableModule{}
	for rows.Next() {
		var m AssignableModule
		if err = rows.Scan(&m.ID, &m.Key, &m.Name, &m.Category); err != nil {
			return nil, err
		}
		items = append(items, m)
	}
	return items, rows.Err()
}
func (s *Store) ModulePermissions(ctx context.Context, userID int64) (map[int64]string, error) {
	rows, err := s.database.QueryContext(ctx, `SELECT module_id,permission_level FROM user_module_permissions WHERE user_id=?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[int64]string{}
	for rows.Next() {
		var id int64
		var level string
		if err = rows.Scan(&id, &level); err != nil {
			return nil, err
		}
		result[id] = level
	}
	return result, rows.Err()
}
func (s *Store) CreateAdminUser(ctx context.Context, u AdminUser, passwordHash string, permissions map[int64]string) (int64, error) {
	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := tx.ExecContext(ctx, `INSERT INTO users(login,first_name,last_name,email,password_hash,type,status,can_view,can_act,can_modify,must_change_password,auth_version,two_factor_required,created_at,updated_at,password_changed_at) VALUES(?,?,?,?,?,'user','active',1,0,0,1,1,?,?,?,?)`, u.Login, u.FirstName, u.LastName, u.Email, passwordHash, boolInt(u.TwoFactorRequired), now, now, now)
	if err != nil {
		return 0, fmt.Errorf("create user: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	if err = replacePermissions(ctx, tx, id, permissions, now); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}
func (s *Store) UpdateAdminUser(ctx context.Context, u AdminUser, permissions map[int64]string, resetTOTP bool) error {
	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339)
	var kind string
	if err = tx.QueryRowContext(ctx, `SELECT type FROM users WHERE id=?`, u.ID).Scan(&kind); err != nil {
		return err
	}
	if kind == "root" {
		_, err = tx.ExecContext(ctx, `UPDATE users SET first_name=?,last_name=?,email=?,updated_at=? WHERE id=? AND type='root'`, u.FirstName, u.LastName, u.Email, now, u.ID)
		if err != nil {
			return err
		}
		return tx.Commit()
	}
	reset := 0
	if resetTOTP {
		reset = 1
	}
	_, err = tx.ExecContext(ctx, `UPDATE users SET login=?,first_name=?,last_name=?,email=?,status=?,two_factor_required=?,totp_secret=CASE WHEN ?=1 THEN NULL ELSE totp_secret END,totp_enabled_at=CASE WHEN ?=1 THEN NULL ELSE totp_enabled_at END,auth_version=auth_version+CASE WHEN ?=1 THEN 1 ELSE 0 END,updated_at=? WHERE id=? AND type='user'`, u.Login, u.FirstName, u.LastName, u.Email, u.Status, boolInt(u.TwoFactorRequired), reset, reset, reset, now, u.ID)
	if err != nil {
		return fmt.Errorf("update user: %w", err)
	}
	if err = replacePermissions(ctx, tx, u.ID, permissions, now); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) ResetAdminPassword(ctx context.Context, id int64, hash string) error {
	result, err := s.database.ExecContext(ctx, `UPDATE users SET password_hash=?,must_change_password=1,auth_version=auth_version+1,updated_at=?,password_changed_at=? WHERE id=? AND type='user'`, hash, time.Now().UTC().Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339), id)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		return errors.New("ordinary user not found")
	}
	return nil
}
func (s *Store) DeleteAdminUser(ctx context.Context, id int64) error {
	result, err := s.database.ExecContext(ctx, `DELETE FROM users WHERE id=? AND type='user'`, id)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		return errors.New("ordinary user not found")
	}
	return nil
}
func replacePermissions(ctx context.Context, tx *sql.Tx, userID int64, items map[int64]string, now string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM user_module_permissions WHERE user_id=?`, userID); err != nil {
		return err
	}
	for id, level := range items {
		if level == "none" {
			continue
		}
		if level != "view" && level != "action" && level != "modify" {
			return errors.New("invalid permission")
		}
		result, err := tx.ExecContext(ctx, `INSERT INTO user_module_permissions(user_id,module_id,permission_level,created_at,updated_at) SELECT ?,id,?,?,? FROM navigation_modules WHERE id=? AND is_enabled=1 AND access_policy='view'`, userID, level, now, now, id)
		if err != nil {
			return err
		}
		count, _ := result.RowsAffected()
		if count != 1 {
			return errors.New("invalid module")
		}
	}
	return nil
}
func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
