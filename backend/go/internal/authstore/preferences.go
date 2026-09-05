package authstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var validThemes = map[string]bool{"dark": true, "light": true, "bootstrap": true, "neon": true}
var validLanguages = map[string]bool{"fr": true, "en": true}

func (s *Store) ThemeForUser(ctx context.Context, userID int64) (string, error) {
	var theme string
	err := s.database.QueryRowContext(ctx, `SELECT theme FROM user_preferences WHERE user_id = ?`, userID).Scan(&theme)
	if errors.Is(err, sql.ErrNoRows) {
		return "dark", nil
	}
	if err != nil {
		return "", fmt.Errorf("read user theme: %w", err)
	}
	if !validThemes[theme] {
		return "", errors.New("stored user theme is invalid")
	}
	return theme, nil
}

func (s *Store) SetTheme(ctx context.Context, userID int64, theme string) error {
	if userID <= 0 || !validThemes[theme] {
		return errors.New("invalid user theme")
	}
	result, err := s.database.ExecContext(ctx, `
INSERT INTO user_preferences(user_id, theme, updated_at)
VALUES(?, ?, ?)
ON CONFLICT(user_id) DO UPDATE SET theme=excluded.theme, updated_at=excluded.updated_at
`, userID, theme, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("save user theme: %w", err)
	}
	if rows, rowsErr := result.RowsAffected(); rowsErr != nil || rows != 1 {
		return errors.New("user theme was not saved")
	}
	return nil
}

func (s *Store) LanguageForUser(ctx context.Context, userID int64) (string, error) {
	var language string
	err := s.database.QueryRowContext(ctx, `SELECT language FROM user_preferences WHERE user_id = ?`, userID).Scan(&language)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read user language: %w", err)
	}
	if !validLanguages[language] {
		return "", errors.New("stored user language is invalid")
	}
	return language, nil
}

func (s *Store) SetLanguage(ctx context.Context, userID int64, language string) error {
	if userID <= 0 || !validLanguages[language] {
		return errors.New("invalid user language")
	}
	result, err := s.database.ExecContext(ctx, `
INSERT INTO user_preferences(user_id, theme, language, updated_at)
VALUES(?, 'dark', ?, ?)
ON CONFLICT(user_id) DO UPDATE SET language=excluded.language, updated_at=excluded.updated_at
`, userID, language, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("save user language: %w", err)
	}
	if rows, rowsErr := result.RowsAffected(); rowsErr != nil || rows != 1 {
		return errors.New("user language was not saved")
	}
	return nil
}
