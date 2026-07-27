package clientconfig

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseAllowlistAndPreserveModernTransport(t *testing.T) {
	raw := []byte(`{
		"remarks": "東京",
		"api": {"tag": "admin"},
		"inbounds": [{"port": 22}],
		"log": {"access": "/tmp/leak"},
		"outbounds": [
			{
				"tag": "edge",
				"protocol": "vless",
				"settings": {"vnext": [{"address": "example.com", "port": 443, "users": [{"id": "11111111-2222-4333-8444-555555555555", "encryption": "none"}]}]},
				"streamSettings": {"network": "xhttp", "security": "tls", "tlsSettings": {"serverName": "example.com", "echConfigList": "abc"}},
				"sendThrough": "127.0.0.1"
			},
			{"tag": "danger", "protocol": "freedom", "settings": {}}
		]
	}`)
	configs, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(configs) != 1 || len(configs[0].Outbounds) != 1 {
		t.Fatalf("unexpected result: %+v", configs)
	}
	if configs[0].Remark != "東京" {
		t.Fatalf("remark=%q", configs[0].Remark)
	}
	outbound := configs[0].Outbounds[0]
	if _, ok := outbound["sendThrough"]; ok {
		t.Fatal("non-allowlisted outbound field survived")
	}
	stream, ok := outbound["streamSettings"].(map[string]any)
	if !ok || stream["network"] != "xhttp" {
		t.Fatalf("modern stream settings lost: %#v", outbound["streamSettings"])
	}
	joined := strings.Join(configs[0].Warnings, " ")
	for _, section := range []string{"api", "inbounds", "log"} {
		if !strings.Contains(joined, section) {
			t.Errorf("missing stripped-section warning for %s", section)
		}
	}
}

func TestParseRejectsUnknownSectionAndDeepJSON(t *testing.T) {
	if _, err := Parse([]byte(`{"command":"shutdown","outbounds":[]}`)); err == nil {
		t.Fatal("unknown top-level section accepted")
	}
	value := any(map[string]any{"outbounds": []any{}})
	for range MaxDepth + 2 {
		value = []any{value}
	}
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(raw); err == nil {
		t.Fatal("deep JSON accepted")
	}
}

func TestParseReportsLineAndColumn(t *testing.T) {
	_, err := Parse([]byte("{\n  \"outbounds\": [\n    }\n  ]\n}"))
	var parseErr *ParseError
	if err == nil || !strings.Contains(err.Error(), "line") || !strings.Contains(err.Error(), "column") {
		t.Fatalf("missing location: %v", err)
	}
	if !strings.Contains(err.Error(), "line 3") {
		t.Fatalf("unexpected error location: %v", err)
	}
	_ = parseErr
}

func FuzzParse(f *testing.F) {
	f.Add([]byte(`{"outbounds":[{"protocol":"socks","settings":{"servers":[{"address":"1.1.1.1","port":1080}]}}]}`))
	f.Add([]byte(`[]`))
	f.Add([]byte(`{"api":{}}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = Parse(data)
	})
}
