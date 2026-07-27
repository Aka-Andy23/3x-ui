package domainset

import (
	"errors"
	"net"
	"net/url"
	"strings"

	"golang.org/x/net/idna"
)

type Domain struct {
	ASCII   string
	Display string
}

func ParseMany(raw string) ([]Domain, []string) {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	cleaned := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		if index := strings.IndexRune(line, '#'); index >= 0 {
			line = line[:index]
		}
		cleaned = append(cleaned, line)
	}
	parts := strings.FieldsFunc(strings.Join(cleaned, "\n"), func(r rune) bool {
		return r == '\n' || r == '\r' || r == '\t' || r == ' ' || r == ',' || r == ';'
	})
	seen := make(map[string]struct{}, len(parts))
	out := make([]Domain, 0, len(parts))
	var invalid []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		domain, err := Normalize(part)
		if err != nil {
			invalid = append(invalid, part)
			continue
		}
		if _, ok := seen[domain.ASCII]; ok {
			continue
		}
		seen[domain.ASCII] = struct{}{}
		out = append(out, domain)
	}
	return out, invalid
}

func Normalize(raw string) (Domain, error) {
	value := strings.TrimSpace(raw)
	value = strings.TrimPrefix(value, "domain:")
	value = strings.TrimPrefix(value, "full:")
	value = strings.TrimPrefix(value, "*.")
	value = strings.TrimPrefix(value, ".")
	if value == "" {
		return Domain{}, errors.New("domain is empty")
	}
	var parsed *url.URL
	var err error
	if strings.Contains(value, "://") {
		parsed, err = url.Parse(value)
	} else {
		parsed, err = url.Parse("//" + value)
	}
	if err != nil {
		return Domain{}, errors.New("domain is invalid")
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if host == "" {
		return Domain{}, errors.New("domain is invalid")
	}
	if net.ParseIP(strings.Trim(host, "[]")) != nil {
		return Domain{}, errors.New("IP addresses are not domain entries")
	}
	ascii, err := idna.Lookup.ToASCII(host)
	if err != nil {
		return Domain{}, errors.New("domain IDN encoding is invalid")
	}
	ascii = strings.TrimSuffix(strings.ToLower(ascii), ".")
	if len(ascii) == 0 || len(ascii) > 253 {
		return Domain{}, errors.New("domain length is invalid")
	}
	labels := strings.Split(ascii, ".")
	if len(labels) < 2 {
		return Domain{}, errors.New("domain must contain a public suffix")
	}
	for _, label := range labels {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return Domain{}, errors.New("domain label is invalid")
		}
		for _, r := range label {
			if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-') {
				return Domain{}, errors.New("domain label is invalid")
			}
		}
	}
	display, err := idna.Lookup.ToUnicode(ascii)
	if err != nil {
		display = host
	}
	return Domain{ASCII: ascii, Display: strings.ToLower(display)}, nil
}
