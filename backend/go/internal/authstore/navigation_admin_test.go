package authstore

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestReorderNavigationMovesModulesAcrossCategories(t *testing.T) {
	path := filepath.Join(t.TempDir(), "aegisadmin.sqlite")
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = database.Exec(`
CREATE TABLE navigation_categories (id INTEGER PRIMARY KEY, name TEXT, position INTEGER, updated_at TEXT);
CREATE TABLE navigation_modules (id INTEGER PRIMARY KEY, category_id INTEGER, position INTEGER, updated_at TEXT);
INSERT INTO navigation_categories VALUES (1,'A',0,''),(2,'B',1,'');
INSERT INTO navigation_modules VALUES (10,1,0,''),(11,1,1,''),(12,2,0,'');`)
	if err != nil {
		t.Fatal(err)
	}
	database.Close()
	store, err := OpenReadWrite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err = store.ReorderNavigation(context.Background(), []int64{2, 1}, map[int64][]int64{2: {12, 10}, 1: {11}}); err != nil {
		t.Fatal(err)
	}
	var category, position int
	if err = store.database.QueryRow(`SELECT category_id,position FROM navigation_modules WHERE id=10`).Scan(&category, &position); err != nil {
		t.Fatal(err)
	}
	if category != 2 || position != 1 {
		t.Fatalf("module 10 = category %d position %d", category, position)
	}
	if err = store.database.QueryRow(`SELECT position FROM navigation_categories WHERE id=2`).Scan(&position); err != nil {
		t.Fatal(err)
	}
	if position != 0 {
		t.Fatalf("category 2 position = %d", position)
	}
	if err = store.ReorderNavigation(context.Background(), []int64{1, 2}, map[int64][]int64{1: {11}, 2: {12}}); err == nil {
		t.Fatal("incomplete order accepted")
	}
}
