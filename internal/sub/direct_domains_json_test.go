package sub

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func TestJSONSubscriptionAppliesGlobalAndClientDirectDomains(t *testing.T) {
	seedSubDB(t)
	seedSubInbound(t, "s1", "direct-json", 4520, 1, wsTLSStream)
	db := database.GetDB()
	var client model.ClientRecord
	if err := db.Where("sub_id = ?", "s1").First(&client).Error; err != nil {
		t.Fatal(err)
	}
	rows := []model.DirectDomain{
		{Mode: model.DirectDomainModeInclude, Domain: "global.example", DisplayDomain: "global.example", Enabled: true},
		{Mode: model.DirectDomainModeInclude, Domain: "excluded.example", DisplayDomain: "excluded.example", Enabled: true},
		{ClientId: client.Id, Mode: model.DirectDomainModeInclude, Domain: "client.example", DisplayDomain: "client.example", Enabled: true},
		{ClientId: client.Id, Mode: model.DirectDomainModeExclude, Domain: "excluded.example", DisplayDomain: "excluded.example", Enabled: true},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}

	body, _, err := NewSubJsonService("", "", "", NewSubService("")).GetJson("s1", "panel.example")
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal([]byte(body), &config); err != nil {
		t.Fatal(err)
	}
	routing, _ := config["routing"].(map[string]any)
	rules, _ := routing["rules"].([]any)
	encoded, _ := json.Marshal(rules)
	text := string(encoded)
	if !containsAll(text, "domain:global.example", "domain:client.example", "domain:panel.example") {
		t.Fatalf("missing direct rules: %s", text)
	}
	if containsAll(text, "domain:excluded.example") {
		t.Fatalf("client exclusion was ignored: %s", text)
	}
}

func containsAll(value string, expected ...string) bool {
	for _, item := range expected {
		if !strings.Contains(value, item) {
			return false
		}
	}
	return true
}
