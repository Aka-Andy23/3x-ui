package sub

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func TestHappMergeDeduplicatesAndBuildsValidatedLowestDelay(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "x-ui.db")); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })
	db := database.GetDB()
	client := &model.ClientRecord{
		Email:  "alice@example.com",
		SubID:  "alice-sub",
		UUID:   "11111111-2222-4333-8444-555555555555",
		Enable: true,
	}
	if err := db.Create(client).Error; err != nil {
		t.Fatal(err)
	}
	outbound := `{"remarks":"External","outbounds":[{"tag":"unsafe-collision","protocol":"socks","settings":{"servers":[{"address":"1.1.1.1","port":1080}]}}]}`
	for index := range 2 {
		if err := db.Create(&model.ClientExternalLink{
			ClientId:  client.Id,
			Kind:      model.ExternalLinkKindJSON,
			Value:     outbound,
			Remark:    "External",
			Enabled:   true,
			SortIndex: index,
		}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Create(&model.DirectDomain{
		Mode:          model.DirectDomainModeInclude,
		Domain:        "example.org",
		DisplayDomain: "example.org",
		Enabled:       true,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Where(model.Setting{Key: "happProviderId"}).
		Assign(model.Setting{Value: "Ab12Cd34"}).
		FirstOrCreate(&model.Setting{}).Error; err != nil {
		t.Fatal(err)
	}
	profile := &model.ClientSubscriptionProfile{
		ClientId:             client.Id,
		Enabled:              true,
		DisplayName:          "Алиса",
		Language:             "ru",
		Title:                "Алиса VPN",
		UpdateInterval:       60,
		AutoSelectEnabled:    true,
		AutoSelectName:       "Самый быстрый",
		ProbeURL:             "https://www.gstatic.com/generate_204",
		ProbeTimeoutSeconds:  4,
		ProbeIntervalSeconds: 120,
	}
	if err := db.Create(profile).Error; err != nil {
		t.Fatal(err)
	}
	generated, err := NewSubJsonService("", "", "", NewSubService("")).GetHappJSON(client.SubID, "panel.example.com", false)
	if err != nil {
		t.Fatalf("GetHappJSON: %v", err)
	}
	if generated == nil || generated.ProviderID != "Ab12Cd34" || !generated.AutoSelect {
		t.Fatalf("unexpected generation metadata: %+v", generated)
	}
	var documents []map[string]any
	if err := json.Unmarshal([]byte(generated.JSON), &documents); err != nil {
		t.Fatalf("decode generated JSON: %v\n%s", err, generated.JSON)
	}
	if len(documents) != 2 {
		t.Fatalf("documents=%d, want one deduplicated connection plus one automatic group", len(documents))
	}
	firstOutbounds, _ := documents[0]["outbounds"].([]any)
	firstProxy, _ := firstOutbounds[0].(map[string]any)
	tag, _ := firstProxy["tag"].(string)
	if tag == "" || tag == "unsafe-collision" || tag == "direct" {
		t.Fatalf("unstable or unsafe tag: %q", tag)
	}
	routing, _ := documents[1]["routing"].(map[string]any)
	balancers, _ := routing["balancers"].([]any)
	balancer, _ := balancers[0].(map[string]any)
	strategy, _ := balancer["strategy"].(map[string]any)
	if strategy["type"] != "leastPing" || balancer["fallbackTag"] == "direct" {
		t.Fatalf("invalid automatic selection: %#v", balancer)
	}
	burst, _ := documents[1]["burstObservatory"].(map[string]any)
	ping, _ := burst["pingConfig"].(map[string]any)
	if ping["timeout"] != "4s" || ping["interval"] != "120s" {
		t.Fatalf("invalid ping config: %#v", ping)
	}
	var stored model.ClientSubscriptionProfile
	if err := db.First(&stored, profile.Id).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != "valid" || stored.ContentHash == "" || stored.LastValidatedAt == 0 {
		t.Fatalf("profile status not persisted: %+v", stored)
	}
}

func TestHappSubscriptionRejectsDisabledOrExpiredClient(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "x-ui.db")); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })
	client := &model.ClientRecord{Email: "off@example.com", SubID: "off-sub", Enable: true}
	if err := database.GetDB().Create(client).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().Model(client).Update("enable", false).Error; err != nil {
		t.Fatal(err)
	}
	generated, err := NewSubJsonService("", "", "", NewSubService("")).GetHappJSON(client.SubID, "panel.example.com", false)
	if err != nil {
		t.Fatal(err)
	}
	if generated != nil {
		t.Fatal("disabled client received a subscription")
	}
}

func TestHappAutoSelectionAcceptsStableSourceSelectors(t *testing.T) {
	items := []happItem{
		{
			Outbound:     map[string]any{"tag": "old-a", "protocol": "socks", "settings": map[string]any{"servers": []any{map[string]any{"address": "1.1.1.1", "port": 1080}}}},
			Tag:          "proxy-a",
			SourceKey:    "local:7:0",
			AutoEligible: true,
		},
		{
			Outbound:     map[string]any{"tag": "old-b", "protocol": "socks", "settings": map[string]any{"servers": []any{map[string]any{"address": "8.8.8.8", "port": 1080}}}},
			Tag:          "proxy-b",
			SourceKey:    "local:8:0",
			AutoEligible: true,
		},
	}
	profile := model.ClientSubscriptionProfile{
		AutoSelectCandidates: []string{"local:7"},
		ProbeURL:             "https://www.gstatic.com/generate_204",
		ProbeTimeoutSeconds:  5,
		ProbeIntervalSeconds: 60,
		FallbackTag:          "local:7",
	}
	config, err := buildHappAutoConfig(items, profile, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	routing := config["routing"].(map[string]any)
	balancer := routing["balancers"].([]any)[0].(map[string]any)
	selectors := balancer["selector"].([]string)
	if len(selectors) != 1 || selectors[0] != "proxy-a" || balancer["fallbackTag"] != "proxy-a" {
		t.Fatalf("unexpected selector mapping: %#v", balancer)
	}
}
