package database

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type legacyClientExternalLink struct {
	Id        int    `gorm:"primaryKey;autoIncrement"`
	ClientId  int    `gorm:"column:client_id"`
	Kind      string `gorm:"column:kind"`
	Value     string `gorm:"column:value;type:text"`
	Remark    string `gorm:"column:remark"`
	SortIndex int    `gorm:"column:sort_index"`
	CreatedAt int64  `gorm:"autoCreateTime:milli"`
}

func (legacyClientExternalLink) TableName() string {
	return "client_external_links"
}

func TestHappModelsMigrateLegacySQLiteIdempotently(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy.db")
	legacy, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	if err := legacy.AutoMigrate(&model.ClientRecord{}, &legacyClientExternalLink{}); err != nil {
		t.Fatal(err)
	}
	client := &model.ClientRecord{Email: "legacy@example.com", SubID: "legacy-sub", Enable: true}
	if err := legacy.Create(client).Error; err != nil {
		t.Fatal(err)
	}
	if err := legacy.Create(&legacyClientExternalLink{ClientId: client.Id, Kind: "link", Value: "trojan://pw@example.com:443", Remark: "legacy"}).Error; err != nil {
		t.Fatal(err)
	}
	sqlDB, err := legacy.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	if err := InitDB(dbPath); err != nil {
		t.Fatalf("first migration: %v", err)
	}
	for _, column := range []string{"enabled", "comment", "last_good_json", "updated_at"} {
		if !db.Migrator().HasColumn(&model.ClientExternalLink{}, column) {
			t.Errorf("missing migrated column %s", column)
		}
	}
	for _, table := range []any{&model.ClientSubscriptionProfile{}, &model.DirectDomain{}} {
		if !db.Migrator().HasTable(table) {
			t.Errorf("missing migrated table %T", table)
		}
	}
	var count int64
	if err := db.Model(&model.ClientExternalLink{}).Where("client_id = ?", client.Id).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("legacy row count=%d", count)
	}
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err := InitDB(dbPath); err != nil {
		t.Fatalf("second migration: %v", err)
	}
	t.Cleanup(func() { _ = CloseDB() })
	if err := db.Model(&model.ClientExternalLink{}).Where("client_id = ?", client.Id).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("second-run row count=%d", count)
	}
}

func TestHappModelsMigratePostgres(t *testing.T) {
	if os.Getenv("XUI_DB_TYPE") != "postgres" || os.Getenv("XUI_DB_DSN") == "" {
		t.Skip("set XUI_DB_TYPE=postgres and XUI_DB_DSN to run")
	}
	if err := InitDB(""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = CloseDB() })
	for _, table := range []any{&model.ClientSubscriptionProfile{}, &model.DirectDomain{}} {
		if !db.Migrator().HasTable(table) || !postgresModelSettled(table) {
			t.Errorf("PostgreSQL model not settled: %T", table)
		}
	}
}
