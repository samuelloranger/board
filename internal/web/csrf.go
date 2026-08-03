package web

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
)

const csrfHeader = "X-CSRF-Token"

// checkHost rejects requests whose Host header is not loopback (or explicitly
// allowed). Without this, DNS rebinding defeats the Origin check entirely: an
// attacker domain re-resolved to 127.0.0.1 on the board's port arrives with
// Origin == Host, so the mutation check passes AND the browser treats the page
// as same-origin, letting it read the CSRF token out of the served HTML.
// Applies to every request, not just mutations — reading "/" is the token leak.
func checkHost(r *http.Request, allowed map[string]struct{}) error {
	host := r.Host
	if host == "" {
		return fmt.Errorf("missing Host header")
	}
	name := hostname(host)
	if isLoopbackHost(name) {
		return nil
	}
	if _, any := allowed["*"]; any {
		return nil
	}
	if _, ok := allowed[strings.ToLower(name)]; ok {
		return nil
	}
	return fmt.Errorf("Host %q not allowed (board serves loopback; use --addr to allow another host)", host)
}

// hostname strips the port and any IPv6 brackets from a Host header value.
func hostname(host string) string {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return strings.Trim(host, "[]")
}

func isLoopbackHost(name string) bool {
	lower := strings.ToLower(name)
	if lower == "localhost" || strings.HasSuffix(lower, ".localhost") {
		return true
	}
	if ip := net.ParseIP(name); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

// allowedHostSet normalizes configured hosts (bare or host:port) for lookup.
func allowedHostSet(hosts []string) map[string]struct{} {
	set := make(map[string]struct{}, len(hosts))
	for _, h := range hosts {
		if h = strings.TrimSpace(h); h == "" {
			continue
		}
		set[strings.ToLower(hostname(h))] = struct{}{}
	}
	return set
}

func newCSRFToken() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("web: csrf token: " + err.Error())
	}
	return hex.EncodeToString(b[:])
}

// checkMutation rejects cross-site and tokenless writes.
// Requires X-CSRF-Token matching the process token. When Origin is set it must
// match the request host; when Sec-Fetch-Site is set it must be same-origin or none.
func checkMutation(r *http.Request, token string) error {
	got := r.Header.Get(csrfHeader)
	if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
		return fmt.Errorf("missing or invalid CSRF token")
	}
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" {
		switch site {
		case "same-origin", "none":
			// ok
		default:
			return fmt.Errorf("cross-site request blocked")
		}
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil || u.Host == "" {
			return fmt.Errorf("invalid Origin")
		}
		if !strings.EqualFold(u.Host, r.Host) {
			return fmt.Errorf("Origin mismatch")
		}
	}
	return nil
}

func isMutating(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}
