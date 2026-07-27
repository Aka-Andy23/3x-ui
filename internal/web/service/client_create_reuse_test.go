package service

import (
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func settingsHoldUUID(t *testing.T, inboundSvc *InboundService, inboundId int, uuid string) bool {
	t.Helper()
	ib, err := inboundSvc.GetInbound(inboundId)
	if err != nil {
		t.Fatalf("GetInbound %d: %v", inboundId, err)
	}
	return strings.Contains(ib.Settings, uuid)
}

func TestCreateRepeatKeepsExistingUUID(t *testing.T) {
	setupBulkDB(t)
	svc := &ClientService{}
	inboundSvc := &InboundService{}

	ibA := mkInbound(t, 21001, model.VLESS, `{"clients":[]}`)
	ibB := mkInbound(t, 21002, model.VLESS, `{"clients":[]}`)

	const originalUUID = "aaaaaaaa-1111-2222-3333-444444444444"
	if _, err := svc.Create(inboundSvc, &ClientCreatePayload{
		Client:     model.Client{Email: "repeat@x", ID: originalUUID, SubID: "sub-repeat", Enable: true},
		InboundIds: []int{ibA.Id},
	}); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	if rec := lookupClientRecord(t, "repeat@x"); rec.UUID != originalUUID {
		t.Fatalf("record UUID after first Create = %q, want %q", rec.UUID, originalUUID)
	}

	if _, err := svc.Create(inboundSvc, &ClientCreatePayload{
		Client:     model.Client{Email: "repeat@x", SubID: "sub-repeat", Enable: true},
		InboundIds: []int{ibB.Id},
	}); err != nil {
		t.Fatalf("repeat Create: %v", err)
	}

	if rec := lookupClientRecord(t, "repeat@x"); rec.UUID != originalUUID {
		t.Fatalf("record UUID after repeat Create = %q, want %q", rec.UUID, originalUUID)
	}
	if !settingsHoldUUID(t, inboundSvc, ibA.Id, originalUUID) {
		t.Fatalf("inbound A settings lost the original UUID")
	}
	if !settingsHoldUUID(t, inboundSvc, ibB.Id, originalUUID) {
		t.Fatalf("inbound B settings did not reuse the original UUID")
	}
}

func TestCreatePersistsSubscriptionBundleAndHonorsDisabledStatus(t *testing.T) {
	setupBulkDB(t)
	svc := &ClientService{}
	inboundSvc := &InboundService{}
	inbound := mkInbound(t, 21003, model.VLESS, `{"clients":[]}`)
	enabled := true
	disabled := false
	_, err := svc.Create(inboundSvc, &ClientCreatePayload{
		Client:       model.Client{Email: "bundle@x", ID: "aaaaaaaa-1111-2222-3333-555555555555", SubID: "sub-bundle"},
		ClientEnable: &disabled,
		InboundIds:   []int{inbound.Id},
		ExternalLinks: []ExternalLinkInput{{
			Kind:    model.ExternalLinkKindJSON,
			Value:   `{"outbounds":[{"protocol":"socks","settings":{"servers":[{"address":"1.1.1.1","port":1080}]}}]}`,
			Enabled: &enabled,
		}},
		SubscriptionProfile: &SubscriptionProfileInput{
			Enabled:              true,
			DisplayName:          "Bundle",
			Language:             "en",
			Title:                "Bundle",
			UpdateInterval:       60,
			ProbeURL:             "https://www.gstatic.com/generate_204",
			ProbeTimeoutSeconds:  5,
			ProbeIntervalSeconds: 300,
		},
		DirectDomains: []DirectDomainInput{{
			Value:   "example.org",
			Mode:    model.DirectDomainModeInclude,
			Enabled: &enabled,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	rec := lookupClientRecord(t, "bundle@x")
	if rec.Enable {
		t.Fatal("disabled create status was not honored")
	}
	var sourceCount, profileCount, domainCount int64
	db := database.GetDB()
	db.Model(&model.ClientExternalLink{}).Where("client_id = ?", rec.Id).Count(&sourceCount)
	db.Model(&model.ClientSubscriptionProfile{}).Where("client_id = ?", rec.Id).Count(&profileCount)
	db.Model(&model.DirectDomain{}).Where("client_id = ?", rec.Id).Count(&domainCount)
	if sourceCount != 1 || profileCount != 1 || domainCount != 1 {
		t.Fatalf("bundle counts: sources=%d profiles=%d domains=%d", sourceCount, profileCount, domainCount)
	}
}

func TestCreateRejectsInvalidBundleBeforeAttaching(t *testing.T) {
	setupBulkDB(t)
	svc := &ClientService{}
	inboundSvc := &InboundService{}
	inbound := mkInbound(t, 21004, model.VLESS, `{"clients":[]}`)
	enabled := true
	_, err := svc.Create(inboundSvc, &ClientCreatePayload{
		Client:     model.Client{Email: "invalid-bundle@x", ID: "aaaaaaaa-1111-2222-3333-666666666666", SubID: "sub-invalid"},
		InboundIds: []int{inbound.Id},
		ExternalLinks: []ExternalLinkInput{{
			Kind:    model.ExternalLinkKindJSON,
			Value:   `{"api":{}}`,
			Enabled: &enabled,
		}},
	})
	if err == nil {
		t.Fatal("invalid bundle accepted")
	}
	var clients int64
	database.GetDB().Model(&model.ClientRecord{}).Where("email = ?", "invalid-bundle@x").Count(&clients)
	if clients != 0 || settingsHoldUUID(t, inboundSvc, inbound.Id, "aaaaaaaa-1111-2222-3333-666666666666") {
		t.Fatal("invalid bundle left partial client state")
	}
}
