package authstore

import (
	"context"
	"fmt"
	"time"
)

type ApplicationSettings struct{ DefaultLog, CertbotEmail string }

func (s *Store) Settings(ctx context.Context) (ApplicationSettings, error) {
	rows, err := s.database.QueryContext(ctx, `SELECT setting_key,setting_value FROM application_settings WHERE setting_key IN ('logs.default_source','certbot.default_email')`)
	if err != nil {
		return ApplicationSettings{}, fmt.Errorf("read settings: %w", err)
	}
	defer rows.Close()
	var result ApplicationSettings
	for rows.Next() {
		var key, value string
		if err = rows.Scan(&key, &value); err != nil {
			return result, err
		}
		switch key {
		case "logs.default_source":
			result.DefaultLog = value
		case "certbot.default_email":
			result.CertbotEmail = value
		}
	}
	return result, rows.Err()
}
func (s *Store) UpdateSettings(ctx context.Context, value ApplicationSettings) error {
	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339)
	for key, item := range map[string]string{"logs.default_source": value.DefaultLog, "certbot.default_email": value.CertbotEmail} {
		if _, err = tx.ExecContext(ctx, `INSERT INTO application_settings(setting_key,setting_value,created_at,updated_at) VALUES(?,?,?,?) ON CONFLICT(setting_key) DO UPDATE SET setting_value=excluded.setting_value,updated_at=excluded.updated_at`, key, item, now, now); err != nil {
			return fmt.Errorf("update setting: %w", err)
		}
	}
	return tx.Commit()
}
