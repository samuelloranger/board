package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCheckMutation(t *testing.T) {
	token := "test-csrf-token-aaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	cases := []struct {
		name    string
		headers map[string]string
		host    string
		wantErr bool
	}{
		{
			name:    "missing token",
			headers: map[string]string{},
			host:    "127.0.0.1:7420",
			wantErr: true,
		},
		{
			name:    "bad token",
			headers: map[string]string{csrfHeader: "nope"},
			host:    "127.0.0.1:7420",
			wantErr: true,
		},
		{
			name:    "token only",
			headers: map[string]string{csrfHeader: token},
			host:    "127.0.0.1:7420",
		},
		{
			name: "same-origin",
			headers: map[string]string{
				csrfHeader:       token,
				"Sec-Fetch-Site": "same-origin",
				"Origin":         "http://127.0.0.1:7420",
			},
			host: "127.0.0.1:7420",
		},
		{
			name: "sec-fetch none",
			headers: map[string]string{
				csrfHeader:       token,
				"Sec-Fetch-Site": "none",
			},
			host: "127.0.0.1:7420",
		},
		{
			name: "cross-site blocked",
			headers: map[string]string{
				csrfHeader:       token,
				"Sec-Fetch-Site": "cross-site",
			},
			host:    "127.0.0.1:7420",
			wantErr: true,
		},
		{
			name: "origin mismatch",
			headers: map[string]string{
				csrfHeader: token,
				"Origin":   "http://evil.example",
			},
			host:    "127.0.0.1:7420",
			wantErr: true,
		},
		{
			// DNS rebinding: attacker.example re-resolved to 127.0.0.1 on the
			// board's port arrives with Origin == Host, so checkMutation alone
			// passes it. checkHost is what actually stops this — see
			// TestCheckHost / TestHandlerRejectsReboundHost.
			name: "rebound origin matches host slips past checkMutation",
			headers: map[string]string{
				csrfHeader:       token,
				"Sec-Fetch-Site": "same-origin",
				"Origin":         "http://attacker.example:7420",
			},
			host: "attacker.example:7420",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "http://"+tc.host+"/api/tasks/1/run", nil)
			r.Host = tc.host
			for k, v := range tc.headers {
				r.Header.Set(k, v)
			}
			err := checkMutation(r, token)
			if tc.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tc.wantErr && err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCheckHost(t *testing.T) {
	cases := []struct {
		name    string
		host    string
		allowed []string
		wantErr bool
	}{
		{name: "loopback ipv4", host: "127.0.0.1:7420"},
		{name: "loopback ipv4 no port", host: "127.0.0.1"},
		{name: "loopback ipv6", host: "[::1]:7420"},
		{name: "localhost", host: "localhost:7420"},
		{name: "localhost subdomain", host: "board.localhost:7420"},
		{name: "uppercase localhost", host: "LOCALHOST:7420"},
		// The rebinding case: resolves to 127.0.0.1, but the Host header is
		// the attacker's name, so it must be refused.
		{name: "rebound attacker name", host: "attacker.example:7420", wantErr: true},
		{name: "lan ip not allowed by default", host: "192.168.1.5:9000", wantErr: true},
		{name: "empty host", host: "", wantErr: true},
		{name: "explicitly allowed", host: "192.168.1.5:9000", allowed: []string{"192.168.1.5:9000"}},
		{name: "allowed bare host", host: "board.lan:9000", allowed: []string{"board.lan"}},
		{name: "allowed is host-only, not port", host: "board.lan:1234", allowed: []string{"board.lan:9000"}},
		{name: "wildcard allows any", host: "anything.example", allowed: []string{"*"}, wantErr: false},
		{name: "other allowed does not admit attacker", host: "attacker.example", allowed: []string{"board.lan"}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.Host = tc.host
			err := checkHost(r, allowedHostSet(tc.allowed))
			if tc.wantErr && err == nil {
				t.Fatalf("expected error for host %q", tc.host)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("host %q: %v", tc.host, err)
			}
		})
	}
}
