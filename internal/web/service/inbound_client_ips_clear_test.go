package service

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func TestClearClientIpsRemovesFlatAndAttributedState(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "x-ui.db")); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })
	db := database.GetDB()
	entries, _ := json.Marshal([]model.ClientIpEntry{{IP: "203.0.113.8", Timestamp: 1}})
	if err := db.Create(&model.InboundClientIps{ClientEmail: "clear@example.com", Ips: string(entries)}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.NodeClientIp{NodeGuid: "node-a", Email: "clear@example.com", Ips: string(entries)}).Error; err != nil {
		t.Fatal(err)
	}

	if err := (&InboundService{}).ClearClientIps("clear@example.com"); err != nil {
		t.Fatal(err)
	}
	var flat model.InboundClientIps
	if err := db.Where("client_email = ?", "clear@example.com").First(&flat).Error; err != nil {
		t.Fatal(err)
	}
	if flat.Ips != "" {
		t.Fatalf("flat IP state was not cleared: %s", flat.Ips)
	}
	var attributed int64
	if err := db.Model(&model.NodeClientIp{}).Where("email = ?", "clear@example.com").Count(&attributed).Error; err != nil {
		t.Fatal(err)
	}
	if attributed != 0 {
		t.Fatalf("attributed IP rows remain: %d", attributed)
	}
}
