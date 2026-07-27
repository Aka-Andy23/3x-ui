package domainset

import "testing"

func TestParseManyNormalizesURLsIDNCommentsAndDuplicates(t *testing.T) {
	raw := `
		# global list
		HTTPS://Example.COM:8443/path?q=1#fragment
		пример.рф
		xn--e1afmkfd.xn--p1ai # duplicate in ASCII
		// disabled.example
		api.example.com, CDN.EXAMPLE.COM; invalid
	`
	domains, invalid := ParseMany(raw)
	if len(domains) != 4 {
		t.Fatalf("domains=%+v invalid=%+v", domains, invalid)
	}
	if len(invalid) != 1 || invalid[0] != "invalid" {
		t.Fatalf("invalid=%v", invalid)
	}
	want := map[string]bool{
		"example.com":           true,
		"xn--e1afmkfd.xn--p1ai": true,
		"api.example.com":       true,
		"cdn.example.com":       true,
	}
	for _, domain := range domains {
		if !want[domain.ASCII] {
			t.Errorf("unexpected domain %+v", domain)
		}
	}
}

func TestNormalizeRejectsIPsAndMalformedDomains(t *testing.T) {
	for _, value := range []string{"127.0.0.1", "[::1]", "localhost", "-bad.example", "bad_.example"} {
		if _, err := Normalize(value); err == nil {
			t.Errorf("%q accepted", value)
		}
	}
}

func FuzzNormalize(f *testing.F) {
	f.Add("https://пример.рф:443/path?q=1")
	f.Add("example.com")
	f.Add("127.0.0.1")
	f.Fuzz(func(t *testing.T, value string) {
		_, _ = Normalize(value)
	})
}
