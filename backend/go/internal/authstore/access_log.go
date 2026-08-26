package authstore

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

var AccessLogEvents = []string{"login_success", "login_failure", "two_factor_success", "two_factor_failure", "logout", "password_changed", "two_factor_enabled", "two_factor_disabled"}
var accessLogEventSet = func() map[string]bool {
	result := map[string]bool{}
	for _, event := range AccessLogEvents {
		result[event] = true
	}
	return result
}()

type AccessLogEntry struct {
	ID                                             int64
	UserID                                         sql.NullInt64
	Login, Event, IPAddress, UserAgent, OccurredAt string
	Success                                        bool
}
type AccessLogFilter struct {
	Login, IPAddress, Event string
	Page, PerPage           int
}
type AccessLogResult struct {
	Items                []AccessLogEntry
	Total, Page, PerPage int
}

func (s *Store) RecordAccess(ctx context.Context, userID *int64, login, event string, success bool, ipAddress, userAgent string) error {
	if !accessLogEventSet[event] {
		return errors.New("invalid access log event")
	}
	login = strings.TrimSpace(login)
	if login == "" {
		login = "(inconnu)"
	}
	if len(login) > 190 {
		login = login[:190]
	}
	if len(ipAddress) > 45 {
		ipAddress = ipAddress[:45]
	}
	if len(userAgent) > 512 {
		userAgent = userAgent[:512]
	}
	_, err := s.database.ExecContext(ctx, `INSERT INTO user_access_log(user_id,login,event,success,ip_address,user_agent,occurred_at) VALUES(?,?,?,?,?,?,?)`, userID, login, event, success, ipAddress, userAgent, time.Now().UTC().Format(time.RFC3339))
	return err
}

func (s *Store) AccessLog(ctx context.Context, filter AccessLogFilter) (AccessLogResult, error) {
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PerPage < 1 || filter.PerPage > 100 {
		filter.PerPage = 50
	}
	filter.Login = strings.TrimSpace(filter.Login)
	filter.IPAddress = strings.TrimSpace(filter.IPAddress)
	if !accessLogEventSet[filter.Event] {
		filter.Event = ""
	}
	where, args := []string{}, []any{}
	if filter.Login != "" {
		where = append(where, `instr(lower(login),lower(?))>0`)
		args = append(args, filter.Login)
	}
	if filter.IPAddress != "" {
		where = append(where, `ip_address=?`)
		args = append(args, filter.IPAddress)
	}
	if filter.Event != "" {
		where = append(where, `event=?`)
		args = append(args, filter.Event)
	}
	clause := ""
	if len(where) > 0 {
		clause = " WHERE " + strings.Join(where, " AND ")
	}
	result := AccessLogResult{Items: []AccessLogEntry{}, Page: filter.Page, PerPage: filter.PerPage}
	if err := s.database.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_access_log`+clause, args...).Scan(&result.Total); err != nil {
		return result, err
	}
	queryArgs := append(append([]any{}, args...), filter.PerPage, (filter.Page-1)*filter.PerPage)
	rows, err := s.database.QueryContext(ctx, `SELECT id,user_id,login,event,success,ip_address,user_agent,occurred_at FROM user_access_log`+clause+` ORDER BY occurred_at DESC,id DESC LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var item AccessLogEntry
		if err := rows.Scan(&item.ID, &item.UserID, &item.Login, &item.Event, &item.Success, &item.IPAddress, &item.UserAgent, &item.OccurredAt); err != nil {
			return result, err
		}
		result.Items = append(result.Items, item)
	}
	return result, rows.Err()
}

func (s *Store) AccessLogUsers(ctx context.Context) ([]string, error) {
	rows, err := s.database.QueryContext(ctx, `SELECT login FROM users ORDER BY type='root' DESC,login COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []string{}
	for rows.Next() {
		var login string
		if err := rows.Scan(&login); err != nil {
			return nil, err
		}
		items = append(items, login)
	}
	return items, rows.Err()
}
