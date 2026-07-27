package securefetch

import (
	"context"
	"net"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func TestValidateURL(t *testing.T) {
	valid := []string{
		"https://example.com/config.json",
		"https://[2606:4700:4700::1111]:443/sub",
	}
	for _, raw := range valid {
		if _, err := ValidateURL(raw); err != nil {
			t.Errorf("ValidateURL(%q): %v", raw, err)
		}
	}
	invalid := []string{
		"http://example.com/config.json",
		"file:///etc/passwd",
		"ftp://example.com/file",
		"https://localhost/config.json",
		"https://metadata.google.internal/computeMetadata/v1/",
		"https://user:pass@example.com/config.json",
		"https://127.0.0.1/config.json",
		"https://10.0.0.1/config.json",
		"https://169.254.169.254/latest/meta-data/",
		"https://224.0.0.1/config.json",
		"https://[::1]/config.json",
		"https://example.com:70000/config.json",
	}
	for _, raw := range invalid {
		if _, err := ValidateURL(raw); err == nil {
			t.Errorf("ValidateURL(%q) unexpectedly succeeded", raw)
		}
	}
}

func TestResolvePublicRejectsAnyPrivateAnswer(t *testing.T) {
	original := lookupNetIP
	t.Cleanup(func() { lookupNetIP = original })
	lookupNetIP = func(context.Context, string, string) ([]netip.Addr, error) {
		return []netip.Addr{
			netip.MustParseAddr("1.1.1.1"),
			netip.MustParseAddr("127.0.0.1"),
		}, nil
	}
	if _, err := resolvePublic(context.Background(), "rebinding.example"); err == nil {
		t.Fatal("mixed public/private DNS answers must be rejected")
	}
}

func TestFetchJSONRejectsReboundAddressBeforeDial(t *testing.T) {
	original := lookupNetIP
	t.Cleanup(func() { lookupNetIP = original })
	lookupNetIP = func(context.Context, string, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("169.254.169.254")}, nil
	}
	result, err := FetchJSON(context.Background(), "https://rebind.example/config.json", Options{Timeout: 100 * time.Millisecond})
	if err == nil || result != nil {
		t.Fatalf("FetchJSON result=%v err=%v", result, err)
	}
	if !strings.Contains(err.Error(), "failed") {
		t.Fatalf("unexpected public error: %v", err)
	}
}

func TestValidateAddressBlocksSpecialRanges(t *testing.T) {
	blocked := []string{
		"0.0.0.0", "100.64.0.1", "168.63.129.16", "192.0.2.1", "198.18.0.1",
		"203.0.113.1", "240.0.0.1", "fc00::1", "fe80::1", "2001:db8::1",
	}
	for _, raw := range blocked {
		if err := validateAddress(netip.MustParseAddr(raw)); err == nil {
			t.Errorf("%s unexpectedly allowed", raw)
		}
	}
	if err := validateAddress(netip.MustParseAddr("1.1.1.1")); err != nil {
		t.Errorf("public address rejected: %v", err)
	}
	if ip := net.ParseIP("8.8.8.8"); ip == nil {
		t.Fatal("test setup produced invalid IP")
	}
}
