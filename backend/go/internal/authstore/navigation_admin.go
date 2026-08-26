package authstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type AdminCategory struct {
	ID       int64
	Name     string
	Position int
	Modules  []AdminModule
}
type AdminModule struct {
	ID                                   int64
	Key, Name, Route, Icon, AccessPolicy string
	Position                             int
	Enabled, Essential                   bool
}

func (s *Store) EnsureGoNavigation(ctx context.Context) error {
	_, err := s.database.ExecContext(ctx, `
INSERT INTO navigation_modules(module_key,name,route,icon,category_id,position,is_enabled,is_essential,access_policy,created_at,updated_at)
SELECT 'processes','Processus','/processes','≋',c.id,
       COALESCE((SELECT MAX(position)+1 FROM navigation_modules WHERE category_id=c.id),0),
       1,0,'view',strftime('%Y-%m-%dT%H:%M:%SZ','now'),strftime('%Y-%m-%dT%H:%M:%SZ','now')
FROM navigation_categories c
WHERE c.name='Supervision' AND NOT EXISTS(SELECT 1 FROM navigation_modules WHERE module_key='processes')
LIMIT 1`)
	if err != nil {
		return fmt.Errorf("ensure Go navigation: %w", err)
	}
	return nil
}

func (s *Store) AdminNavigation(ctx context.Context) ([]AdminCategory, error) {
	rows, err := s.database.QueryContext(ctx, `SELECT c.id,c.name,c.position,m.id,m.module_key,m.name,m.route,m.icon,m.position,m.is_enabled,m.is_essential,m.access_policy FROM navigation_categories c LEFT JOIN navigation_modules m ON m.category_id=c.id ORDER BY c.position,c.id,m.position,m.id`)
	if err != nil {
		return nil, fmt.Errorf("read navigation administration: %w", err)
	}
	defer rows.Close()
	items := []AdminCategory{}
	index := map[int64]int{}
	for rows.Next() {
		var cID int64
		var cName string
		var cPos int
		var id *int64
		var key, name, route, icon, policy *string
		var pos, enabled, essential *int
		if err = rows.Scan(&cID, &cName, &cPos, &id, &key, &name, &route, &icon, &pos, &enabled, &essential, &policy); err != nil {
			return nil, err
		}
		at, ok := index[cID]
		if !ok {
			at = len(items)
			index[cID] = at
			items = append(items, AdminCategory{ID: cID, Name: cName, Position: cPos, Modules: []AdminModule{}})
		}
		if id != nil {
			items[at].Modules = append(items[at].Modules, AdminModule{ID: *id, Key: *key, Name: *name, Route: *route, Icon: *icon, Position: *pos, Enabled: *enabled == 1, Essential: *essential == 1, AccessPolicy: *policy})
		}
	}
	return items, rows.Err()
}
func (s *Store) CreateCategory(ctx context.Context, name string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.database.ExecContext(ctx, `INSERT INTO navigation_categories(name,position,created_at,updated_at) VALUES(?,COALESCE((SELECT MAX(position)+1 FROM navigation_categories),0),?,?)`, name, now, now)
	return err
}
func (s *Store) RenameCategory(ctx context.Context, id int64, name string) error {
	result, err := s.database.ExecContext(ctx, `UPDATE navigation_categories SET name=?,updated_at=? WHERE id=?`, name, time.Now().UTC().Format(time.RFC3339), id)
	return oneRow(result, err)
}
func (s *Store) DeleteCategory(ctx context.Context, id int64) error {
	result, err := s.database.ExecContext(ctx, `DELETE FROM navigation_categories WHERE id=? AND NOT EXISTS(SELECT 1 FROM navigation_modules WHERE category_id=?)`, id, id)
	return oneRow(result, err)
}
func (s *Store) ToggleModule(ctx context.Context, id int64, enabled bool) error {
	result, err := s.database.ExecContext(ctx, `UPDATE navigation_modules SET is_enabled=?,updated_at=? WHERE id=? AND is_essential=0`, boolInt(enabled), time.Now().UTC().Format(time.RFC3339), id)
	return oneRow(result, err)
}
func (s *Store) MoveCategory(ctx context.Context, id int64, direction int) error {
	return s.move(ctx, "navigation_categories", id, direction, 0)
}
func (s *Store) MoveModule(ctx context.Context, id int64, direction int) error {
	var category int64
	if err := s.database.QueryRowContext(ctx, `SELECT category_id FROM navigation_modules WHERE id=?`, id).Scan(&category); err != nil {
		return err
	}
	return s.move(ctx, "navigation_modules", id, direction, category)
}
func (s *Store) ReorderNavigation(ctx context.Context, categoryIDs []int64, moduleIDsByCategory map[int64][]int64) error {
	if len(categoryIDs) == 0 || len(categoryIDs) > 100 {
		return errors.New("invalid category order")
	}
	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	knownCategories, knownModules := map[int64]bool{}, map[int64]bool{}
	rows, err := tx.QueryContext(ctx, `SELECT id FROM navigation_categories`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		knownCategories[id] = true
	}
	if err = rows.Close(); err != nil {
		return err
	}
	rows, err = tx.QueryContext(ctx, `SELECT id FROM navigation_modules`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		knownModules[id] = true
	}
	if err = rows.Close(); err != nil {
		return err
	}
	seenCategories, seenModules := map[int64]bool{}, map[int64]bool{}
	now := time.Now().UTC().Format(time.RFC3339)
	for categoryPosition, categoryID := range categoryIDs {
		if !knownCategories[categoryID] || seenCategories[categoryID] {
			return errors.New("invalid category identifier")
		}
		seenCategories[categoryID] = true
		if _, exists := moduleIDsByCategory[categoryID]; !exists {
			return errors.New("missing category modules")
		}
		if _, err = tx.ExecContext(ctx, `UPDATE navigation_categories SET position=?,updated_at=? WHERE id=?`, categoryPosition, now, categoryID); err != nil {
			return err
		}
		for modulePosition, moduleID := range moduleIDsByCategory[categoryID] {
			if !knownModules[moduleID] || seenModules[moduleID] {
				return errors.New("invalid module identifier")
			}
			seenModules[moduleID] = true
			if _, err = tx.ExecContext(ctx, `UPDATE navigation_modules SET category_id=?,position=?,updated_at=? WHERE id=?`, categoryID, modulePosition, now, moduleID); err != nil {
				return err
			}
		}
	}
	if len(seenCategories) != len(knownCategories) || len(seenModules) != len(knownModules) {
		return errors.New("incomplete navigation order")
	}
	return tx.Commit()
}
func (s *Store) move(ctx context.Context, table string, id int64, direction int, category int64) error {
	if direction != -1 && direction != 1 {
		return errors.New("invalid direction")
	}
	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	filter := ""
	args := []any{id}
	if table == "navigation_modules" {
		filter = " AND category_id=?"
		args = append(args, category)
	}
	var position int
	if err = tx.QueryRowContext(ctx, `SELECT position FROM `+table+` WHERE id=?`+filter, args...).Scan(&position); err != nil {
		return err
	}
	operator, order := "<", "DESC"
	if direction > 0 {
		operator, order = ">", "ASC"
	}
	query := `SELECT id,position FROM ` + table + ` WHERE position ` + operator + ` ?` + filter + ` ORDER BY position ` + order + `,id ` + order + ` LIMIT 1`
	otherArgs := []any{position}
	if table == "navigation_modules" {
		otherArgs = append(otherArgs, category)
	}
	var otherID int64
	var otherPosition int
	if err = tx.QueryRowContext(ctx, query, otherArgs...).Scan(&otherID, &otherPosition); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err = tx.ExecContext(ctx, `UPDATE `+table+` SET position=?,updated_at=? WHERE id=?`, otherPosition, now, id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE `+table+` SET position=?,updated_at=? WHERE id=?`, position, now, otherID); err != nil {
		return err
	}
	return tx.Commit()
}
func oneRow(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		return errors.New("item not found or protected")
	}
	return nil
}
