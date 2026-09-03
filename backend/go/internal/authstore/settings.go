package authstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"
)

type ApplicationSettings struct{ DefaultLog, CertbotEmail string }

type SMTPSettings struct {
	Host, Username, Security string
	Port                     int
	PasswordConfigured       bool
}

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

func (s *Store) SMTPSettings(ctx context.Context) (SMTPSettings, error) {
	rows, err := s.database.QueryContext(ctx, `SELECT setting_key,setting_value FROM application_settings WHERE setting_key IN ('smtp.host','smtp.username','smtp.password','smtp.port','smtp.security')`)
	if err != nil {
		return SMTPSettings{}, fmt.Errorf("read SMTP settings: %w", err)
	}
	defer rows.Close()
	result := SMTPSettings{Port: 587, Security: "starttls"}
	for rows.Next() {
		var key, value string
		if err = rows.Scan(&key, &value); err != nil {
			return result, err
		}
		switch key {
		case "smtp.host":
			result.Host = value
		case "smtp.username":
			result.Username = value
		case "smtp.password":
			result.PasswordConfigured = value != ""
		case "smtp.port":
			if port, parseErr := strconv.Atoi(value); parseErr == nil {
				result.Port = port
			}
		case "smtp.security":
			result.Security = value
		}
	}
	return result, rows.Err()
}

func (s *Store) UpdateSMTPSettings(ctx context.Context, value SMTPSettings, password *string) error {
	items := map[string]string{"smtp.host": value.Host, "smtp.username": value.Username, "smtp.port": strconv.Itoa(value.Port), "smtp.security": value.Security}
	if password != nil {
		encrypted := ""
		var err error
		if *password != "" {
			encrypted, err = s.encryptSecret(*password)
			if err != nil {
				return fmt.Errorf("encrypt SMTP password: %w", err)
			}
		}
		items["smtp.password"] = encrypted
	}
	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339)
	for key, item := range items {
		if _, err = tx.ExecContext(ctx, `INSERT INTO application_settings(setting_key,setting_value,created_at,updated_at) VALUES(?,?,?,?) ON CONFLICT(setting_key) DO UPDATE SET setting_value=excluded.setting_value,updated_at=excluded.updated_at`, key, item, now, now); err != nil {
			return fmt.Errorf("update SMTP setting: %w", err)
		}
	}
	return tx.Commit()
}

func (s *Store) SMTPPassword(ctx context.Context) (string, error) {
	var value string
	err := s.database.QueryRowContext(ctx, `SELECT setting_value FROM application_settings WHERE setting_key='smtp.password'`).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) || value == "" {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read SMTP password: %w", err)
	}
	return s.decryptSecret(value)
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
