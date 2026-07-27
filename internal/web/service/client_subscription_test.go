package service

import (
	"path/filepath"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func setupClientSubscriptionTestDB(t *testing.T) *model.ClientRecord {
	t.Helper()
	if err := database.InitDB(filepath.Join(t.TempDir(), "x-ui.db")); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })
	client := &model.ClientRecord{
		Email:  "alice@example.com",
		SubID:  "alice-sub",
		UUID:   "11111111-2222-4333-8444-555555555555",
		Enable: true,
	}
	if err := database.GetDB().Create(client).Error; err != nil {
		t.Fatalf("create client: %v", err)
	}
	return client
}

func TestExternalJSONSourcesAreValidatedAndScoped(t *testing.T) {
	client := setupClientSubscriptionTestDB(t)
	svc := &ClientService{}
	enabled := true
	inputs := []ExternalLinkInput{
		{
			Kind:     model.ExternalLinkKindJSON,
			Value:    `{"remarks":"manual","outbounds":[{"tag":"edge","protocol":"socks","settings":{"servers":[{"address":"1.1.1.1","port":1080}]}}]}`,
			Remark:   "Manual",
			Comment:  "Only Alice",
			Enabled:  &enabled,
			Priority: 10,
		},
		{
			Kind:                  model.ExternalLinkKindJSONSubscription,
			Value:                 "https://provider.example/config.json",
			Remark:                "Remote",
			Enabled:               &enabled,
			UpdateIntervalMinutes: 15,
			TimeoutSeconds:        4,
			MaxResponseBytes:      1024,
			MaxRedirects:          1,
		},
	}
	if err := svc.SetExternalLinksForRecord(client.Id, inputs); err != nil {
		t.Fatalf("SetExternalLinksForRecord: %v", err)
	}
	rows, err := svc.GetExternalLinksForRecord(client.Id)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].ClientId != client.Id || rows[1].ClientId != client.Id {
		t.Fatalf("unexpected rows: %+v", rows)
	}
	if rows[0].Kind != model.ExternalLinkKindJSON || rows[1].Kind != model.ExternalLinkKindJSONSubscription {
		t.Fatalf("unexpected kinds: %+v", rows)
	}
	if err := svc.SetExternalLinksForRecord(client.Id, []ExternalLinkInput{{
		Kind:    model.ExternalLinkKindJSON,
		Value:   `{"api":{},"outbounds":[]}`,
		Enabled: &enabled,
	}}); err == nil {
		t.Fatal("unsafe JSON without supported proxy outbounds accepted")
	}
	if err := svc.SetExternalLinksForRecord(client.Id, []ExternalLinkInput{{
		Kind:    model.ExternalLinkKindJSONSubscription,
		Value:   "https://127.0.0.1/config.json",
		Enabled: &enabled,
	}}); err == nil {
		t.Fatal("private remote JSON URL accepted")
	}
}

func TestSubscriptionProfileAndDirectDomainValidation(t *testing.T) {
	client := setupClientSubscriptionTestDB(t)
	svc := &ClientService{}
	input := SubscriptionProfileInput{
		Enabled:              true,
		DisplayName:          "Alice",
		Language:             "ru",
		Title:                "Alice VPN",
		UpdateInterval:       60,
		AutoSelectEnabled:    true,
		AutoSelectName:       "Lowest latency",
		ProbeURL:             "https://www.gstatic.com/generate_204",
		ProbeTimeoutSeconds:  5,
		ProbeIntervalSeconds: 300,
	}
	if _, err := svc.SaveSubscriptionProfileForRecord(client.Id, input); err == nil {
		t.Fatal("automatic selection accepted without Happ Provider ID")
	}
	if err := (&SettingService{}).saveSetting("happProviderId", "Ab12Cd34"); err != nil {
		t.Fatal(err)
	}
	profile, err := svc.SaveSubscriptionProfileForRecord(client.Id, input)
	if err != nil {
		t.Fatalf("SaveSubscriptionProfileForRecord: %v", err)
	}
	if profile.ClientId != client.Id || !profile.AutoSelectEnabled || profile.Language != "ru" {
		t.Fatalf("unexpected profile: %+v", profile)
	}
	input.Enabled = false
	input.AutoSelectEnabled = false
	profile, err = svc.SaveSubscriptionProfileForRecord(client.Id, input)
	if err != nil {
		t.Fatalf("disable profile: %v", err)
	}
	if profile.Enabled || profile.AutoSelectEnabled {
		t.Fatalf("profile booleans were not persisted: %+v", profile)
	}
	row, err := svc.UpsertDirectDomain(0, DirectDomainInput{Value: "HTTPS://Пример.РФ:443/path?q=1", Mode: model.DirectDomainModeInclude})
	if err != nil {
		t.Fatalf("UpsertDirectDomain: %v", err)
	}
	if row.Domain != "xn--e1afmkfd.xn--p1ai" {
		t.Fatalf("domain=%q", row.Domain)
	}
	exclusion, err := svc.UpsertDirectDomain(client.Id, DirectDomainInput{Value: "пример.рф", Mode: model.DirectDomainModeExclude})
	if err != nil {
		t.Fatalf("client exclusion: %v", err)
	}
	if err := svc.DeleteDirectDomain(0, exclusion.Id); err == nil {
		t.Fatal("cross-scope delete succeeded")
	}
}
