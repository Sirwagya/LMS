package httpadapter

import (
	"net/http/httptest"
	"net/netip"
	"testing"
)

func TestNativeBearerAuthRequiresExplicitNonBrowserRequest(t *testing.T) {
	req := httptest.NewRequest("POST", "/api/v1/login", nil)
	if nativeAuth(req) {
		t.Fatal("ordinary requests must use cookie authentication")
	}
	req.Header.Set("X-LMS-Auth", "bearer")
	if !nativeAuth(req) {
		t.Fatal("native v1 client should be able to request bearer authentication")
	}
	for _, header := range []string{"Origin", "Cookie", "Sec-Fetch-Site", "Sec-Fetch-Mode"} {
		req.Header.Set(header, "example")
		if nativeAuth(req) {
			t.Fatalf("browser header %s must prevent bearer disclosure", header)
		}
		req.Header.Del(header)
	}
}

func TestClientIPTrustsOnlyConfiguredProxyChain(t *testing.T) {
	a := &api{Dependencies: Dependencies{TrustedProxies: []netip.Prefix{netip.MustParsePrefix("192.0.2.0/24")}}}
	req := httptest.NewRequest("POST", "/api/login", nil)
	req.Header.Set("X-Forwarded-For", "198.51.100.5, 203.0.113.9, 192.0.2.8")
	req.RemoteAddr = "203.0.113.3:12345"
	if got := a.clientIP(req); got != "203.0.113.3" {
		t.Fatalf("untrusted client supplied a proxy header: %s", got)
	}
	req.RemoteAddr = "192.0.2.9:12345"
	if got := a.clientIP(req); got != "203.0.113.9" {
		t.Fatalf("expected nearest untrusted peer, got %s", got)
	}
}

func TestEventStreamCapacityIsReleased(t *testing.T) {
	var slots streamSlots
	var releases []func()
	for range 4 {
		release, ok := slots.acquire("student")
		if !ok {
			t.Fatal("stream was rejected below per-account limit")
		}
		releases = append(releases, release)
	}
	if _, ok := slots.acquire("student"); ok {
		t.Fatal("per-account stream cap exceeded")
	}
	releases[0]()
	release, ok := slots.acquire("student")
	if !ok {
		t.Fatal("closing a stream must release its slot")
	}
	release()
	for _, closeStream := range releases[1:] {
		closeStream()
	}
	if slots.total != 0 || len(slots.users) != 0 {
		t.Fatal("closed streams left capacity allocated")
	}
}
