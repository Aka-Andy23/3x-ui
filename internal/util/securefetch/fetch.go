package securefetch

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Options struct {
	Timeout      time.Duration
	MaxBytes     int64
	MaxRedirects int
	UserAgent    string
	ETag         string
	LastModified string
}

type Result struct {
	Body         []byte
	StatusCode   int
	ContentType  string
	ETag         string
	LastModified string
	NotModified  bool
}

var deniedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("168.63.129.16/32"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("224.0.0.0/4"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("::/128"),
	netip.MustParsePrefix("::1/128"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("fc00::/7"),
	netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("ff00::/8"),
}

var lookupNetIP = net.DefaultResolver.LookupNetIP

func ValidateURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, errors.New("invalid remote URL")
	}
	if !strings.EqualFold(u.Scheme, "https") {
		return nil, errors.New("remote URL must use HTTPS")
	}
	if u.Host == "" || u.Hostname() == "" {
		return nil, errors.New("remote URL must contain a host")
	}
	if u.User != nil {
		return nil, errors.New("remote URL user information is not allowed")
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") || host == "metadata.google.internal" {
		return nil, errors.New("remote host is not allowed")
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		if err := validateAddress(ip); err != nil {
			return nil, err
		}
	}
	if u.Port() != "" {
		port, err := strconv.Atoi(u.Port())
		if err != nil || port < 1 || port > 65535 {
			return nil, errors.New("remote URL port is invalid")
		}
	}
	u.Fragment = ""
	return u, nil
}

func FetchJSON(ctx context.Context, raw string, opts Options) (*Result, error) {
	u, err := ValidateURL(raw)
	if err != nil {
		return nil, err
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 8 * time.Second
	}
	if opts.MaxBytes <= 0 {
		opts.MaxBytes = 2 << 20
	}
	if opts.MaxBytes > 16<<20 {
		opts.MaxBytes = 16 << 20
	}
	if opts.MaxRedirects < 0 {
		opts.MaxRedirects = 0
	}
	if opts.MaxRedirects > 5 {
		opts.MaxRedirects = 5
	}
	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	transport := &http.Transport{
		Proxy:               nil,
		ForceAttemptHTTP2:   true,
		TLSHandshakeTimeout: opts.Timeout,
		TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12},
		DialContext: func(dialCtx context.Context, network, address string) (net.Conn, error) {
			host, port, splitErr := net.SplitHostPort(address)
			if splitErr != nil {
				return nil, errors.New("remote address is invalid")
			}
			addresses, resolveErr := resolvePublic(dialCtx, host)
			if resolveErr != nil {
				return nil, resolveErr
			}
			dialer := &net.Dialer{Timeout: opts.Timeout}
			var dialErr error
			for _, ip := range addresses {
				conn, connErr := dialer.DialContext(dialCtx, network, net.JoinHostPort(ip.String(), port))
				if connErr == nil {
					return conn, nil
				}
				dialErr = connErr
			}
			if dialErr == nil {
				dialErr = errors.New("remote host has no public address")
			}
			return nil, dialErr
		},
	}
	defer transport.CloseIdleConnections()

	redirects := 0
	client := &http.Client{
		Transport: transport,
		Timeout:   opts.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			redirects++
			if redirects > opts.MaxRedirects {
				return errors.New("remote response exceeded redirect limit")
			}
			if _, redirectErr := ValidateURL(req.URL.String()); redirectErr != nil {
				return redirectErr
			}
			return nil
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, errors.New("remote request is invalid")
	}
	if opts.UserAgent == "" {
		opts.UserAgent = "3x-ui-happ/1"
	}
	req.Header.Set("User-Agent", opts.UserAgent)
	req.Header.Set("Accept", "application/json, application/*+json")
	if opts.ETag != "" {
		req.Header.Set("If-None-Match", opts.ETag)
	}
	if opts.LastModified != "" {
		req.Header.Set("If-Modified-Since", opts.LastModified)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.New("remote request failed")
	}
	defer resp.Body.Close()
	result := &Result{
		StatusCode:   resp.StatusCode,
		ContentType:  resp.Header.Get("Content-Type"),
		ETag:         resp.Header.Get("ETag"),
		LastModified: resp.Header.Get("Last-Modified"),
	}
	if resp.StatusCode == http.StatusNotModified {
		result.NotModified = true
		return result, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return result, fmt.Errorf("remote response status %d", resp.StatusCode)
	}
	mediaType, _, err := mime.ParseMediaType(result.ContentType)
	if err != nil || !(mediaType == "application/json" || mediaType == "text/json" || strings.HasSuffix(mediaType, "+json")) {
		return result, errors.New("remote response content type is not JSON")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, opts.MaxBytes+1))
	if err != nil {
		return result, errors.New("remote response could not be read")
	}
	if int64(len(body)) > opts.MaxBytes {
		return result, errors.New("remote response exceeds size limit")
	}
	result.Body = body
	return result, nil
}

func resolvePublic(ctx context.Context, host string) ([]netip.Addr, error) {
	if ip, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
		if err := validateAddress(ip); err != nil {
			return nil, err
		}
		return []netip.Addr{ip.Unmap()}, nil
	}
	addresses, err := lookupNetIP(ctx, "ip", host)
	if err != nil || len(addresses) == 0 {
		return nil, errors.New("remote host could not be resolved")
	}
	out := make([]netip.Addr, 0, len(addresses))
	for _, ip := range addresses {
		ip = ip.Unmap()
		if err := validateAddress(ip); err != nil {
			return nil, err
		}
		out = append(out, ip)
	}
	return out, nil
}

func validateAddress(ip netip.Addr) error {
	ip = ip.Unmap()
	if !ip.IsValid() || !ip.IsGlobalUnicast() || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return errors.New("remote host resolves to a disallowed address")
	}
	for _, prefix := range deniedPrefixes {
		if prefix.Contains(ip) {
			return errors.New("remote host resolves to a disallowed address")
		}
	}
	return nil
}
