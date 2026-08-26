package authstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"modernc.org/sqlite"
)

const maximumRestoreBytes = 20 * 1024 * 1024

type sqliteBackuper interface {
	NewBackup(string) (*sqlite.Backup, error)
	NewRestore(string) (*sqlite.Backup, error)
}

func (s *Store) BackupDatabase(ctx context.Context) ([]byte, error) {
	file, err := os.CreateTemp(filepath.Dir(s.path), ".aegisadmin-backup-*.sqlite")
	if err != nil {
		return nil, err
	}
	path := file.Name()
	file.Close()
	os.Remove(path)
	defer os.Remove(path)
	if err = s.withBackup(ctx, func(b sqliteBackuper) (*sqlite.Backup, error) { return b.NewBackup(path) }); err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}
func (s *Store) RestoreDatabase(ctx context.Context, content []byte) error {
	if len(content) < 100 || len(content) > maximumRestoreBytes || string(content[:16]) != "SQLite format 3\x00" {
		return errors.New("invalid SQLite backup")
	}
	file, err := os.CreateTemp(filepath.Dir(s.path), ".aegisadmin-restore-*.sqlite")
	if err != nil {
		return err
	}
	path := file.Name()
	defer os.Remove(path)
	if _, err = file.Write(content); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	if err = validateBackup(path); err != nil {
		return err
	}
	safety, err := s.BackupDatabase(ctx)
	if err != nil {
		return fmt.Errorf("safety backup: %w", err)
	}
	directory := filepath.Join(filepath.Dir(s.path), "backups")
	if err = os.MkdirAll(directory, 0770); err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(directory, "before-restore-latest.sqlite"), safety, 0660); err != nil {
		return err
	}
	if err = s.withBackup(ctx, func(b sqliteBackuper) (*sqlite.Backup, error) { return b.NewRestore(path) }); err != nil {
		return fmt.Errorf("restore database: %w", err)
	}
	if _, err = s.database.ExecContext(ctx, `UPDATE users SET auth_version=auth_version+1`); err != nil {
		return fmt.Errorf("invalidate restored sessions: %w", err)
	}
	return s.Check(ctx)
}
func (s *Store) withBackup(ctx context.Context, create func(sqliteBackuper) (*sqlite.Backup, error)) error {
	connection, err := s.database.Conn(ctx)
	if err != nil {
		return err
	}
	defer connection.Close()
	return connection.Raw(func(driver any) error {
		backup, err := create(driver.(sqliteBackuper))
		if err != nil {
			return err
		}
		for more := true; more; {
			more, err = backup.Step(-1)
			if err != nil {
				_ = backup.Finish()
				return err
			}
		}
		return backup.Finish()
	})
}
func validateBackup(path string) error {
	db, err := sql.Open("sqlite", path+"?mode=ro")
	if err != nil {
		return err
	}
	defer db.Close()
	var integrity string
	if err = db.QueryRow(`PRAGMA integrity_check`).Scan(&integrity); err != nil || integrity != "ok" {
		return errors.New("backup integrity check failed")
	}
	required := []string{"schema_migrations", "users", "navigation_categories", "navigation_modules", "user_module_permissions", "application_settings", "user_access_log"}
	for _, table := range required {
		var count int
		if err = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&count); err != nil || count != 1 {
			return fmt.Errorf("missing table %s", table)
		}
	}
	var roots int
	if err = db.QueryRow(`SELECT COUNT(*) FROM users WHERE type='root' AND status='active'`).Scan(&roots); err != nil || roots != 1 {
		return errors.New("backup root account is invalid")
	}
	return nil
}
