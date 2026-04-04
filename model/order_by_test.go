package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func renderOrderBySQL(t *testing.T, dialector gorm.Dialector, fields ...OrderField) string {
	t.Helper()

	db := &gorm.DB{Config: &gorm.Config{Dialector: dialector}}
	stmt := &gorm.Statement{DB: db, Table: "audio_scenes"}
	BuildOrderBy(fields...).Build(stmt)
	return stmt.SQL.String()
}

func TestBuildOrderByQuotesReservedKeywordForPostgres(t *testing.T) {
	sql := renderOrderBySQL(t, postgres.New(postgres.Config{}),
		OrderField{Name: "order"},
		OrderField{Name: "created_at"},
	)

	if sql != `"order","created_at"` {
		t.Fatalf("unexpected postgres order by sql: %s", sql)
	}
}

func TestBuildOrderByRemainsCompatibleWithSQLite(t *testing.T) {
	sql := renderOrderBySQL(t, sqlite.Dialector{},
		OrderField{Name: "order"},
		OrderField{Name: "created_at"},
	)

	if sql != "`order`,`created_at`" {
		t.Fatalf("unexpected sqlite order by sql: %s", sql)
	}
}

func TestBuildOrderByQuotesQualifiedColumns(t *testing.T) {
	sql := renderOrderBySQL(t, postgres.New(postgres.Config{}),
		OrderField{Table: "gallery_items", Name: "order", Desc: true},
		OrderField{Table: "gallery_items", Name: "created_at", Desc: true},
		OrderField{Table: "gallery_items", Name: "id", Desc: true},
	)

	if sql != `"gallery_items"."order" DESC,"gallery_items"."created_at" DESC,"gallery_items"."id" DESC` {
		t.Fatalf("unexpected qualified postgres order by sql: %s", sql)
	}
}
