package sub

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"

	"github.com/gin-gonic/gin"
)

func TestHappEndpointHeadersETagAndHead(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "x-ui.db")); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })
	db := database.GetDB()
	client := &model.ClientRecord{
		Email:  "alice@example.com",
		SubID:  "alice-subscription-id",
		UUID:   "11111111-2222-4333-8444-555555555555",
		Enable: true,
	}
	if err := db.Create(client).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.ClientExternalLink{
		ClientId: client.Id,
		Kind:     model.ExternalLinkKindJSON,
		Value:    `{"remarks":"Москва","outbounds":[{"tag":"edge","protocol":"socks","settings":{"servers":[{"address":"1.1.1.1","port":1080}]}}]}`,
		Enabled:  true,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.ClientSubscriptionProfile{
		ClientId:             client.Id,
		Enabled:              true,
		DisplayName:          "Алиса",
		Title:                "Алиса VPN",
		Language:             "ru",
		UpdateInterval:       60,
		ProbeURL:             "https://www.gstatic.com/generate_204",
		ProbeTimeoutSeconds:  5,
		ProbeIntervalSeconds: 300,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Where(model.Setting{Key: "happProviderId"}).
		Assign(model.Setting{Value: "Ab12Cd34"}).
		FirstOrCreate(&model.Setting{}).Error; err != nil {
		t.Fatal(err)
	}

	subService := NewSubService("")
	controller := &SUBController{
		subService:       subService,
		subJsonService:   NewSubJsonService("", "", "", subService),
		happLimiter:      newRequestLimiter(120, time.Minute),
		updateInterval:   "60",
		subEnableRouting: false,
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/happ/:subid", controller.subHappJsons)
	router.HEAD("/happ/:subid", controller.subHappJsons)

	first := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "https://panel.example.com/happ/"+client.SubID, nil)
	router.ServeHTTP(first, request)
	if first.Code != http.StatusOK {
		t.Fatalf("GET status=%d body=%s", first.Code, first.Body.String())
	}
	etag := first.Header().Get("ETag")
	if etag == "" || first.Header().Get("Cache-Control") != "private, max-age=0, must-revalidate" {
		t.Fatalf("unsafe cache headers: %#v", first.Header())
	}
	if first.Header().Get("providerid") != "Ab12Cd34" || first.Header().Get("X-Subscription-Status") != "valid" {
		t.Fatalf("missing Happ headers: %#v", first.Header())
	}
	disposition := first.Header().Get("Content-Disposition")
	if !strings.Contains(disposition, "filename*=UTF-8''") || !strings.Contains(disposition, "%D0%90") {
		t.Fatalf("Unicode filename is not encoded safely: %q", disposition)
	}
	if strings.Contains(first.Body.String(), "alice@example.com") {
		t.Fatal("administrative client identity leaked into the generated proxy document")
	}

	notModified := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "https://panel.example.com/happ/"+client.SubID, nil)
	request.Header.Set("If-None-Match", etag)
	router.ServeHTTP(notModified, request)
	if notModified.Code != http.StatusNotModified || notModified.Body.Len() != 0 {
		t.Fatalf("conditional GET status=%d body=%q", notModified.Code, notModified.Body.String())
	}

	head := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodHead, "https://panel.example.com/happ/"+client.SubID, nil)
	router.ServeHTTP(head, request)
	if head.Code != http.StatusOK || head.Body.Len() != 0 || head.Header().Get("ETag") != etag {
		t.Fatalf("HEAD status=%d body=%q headers=%#v", head.Code, head.Body.String(), head.Header())
	}
}
