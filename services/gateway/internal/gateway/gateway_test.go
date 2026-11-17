package gateway

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/defaaryawar/ecommerce/pkg/auth"
)

const loginPath = "/identity.v1.MerchantAuthService/Login"
const protectedPath = "/identity.v1.MerchantAuthService/Whatever"

func newTestIssuer(t *testing.T) *auth.Issuer {
	t.Helper()
	iss, err := auth.NewIssuer("ecommerce-identity", time.Minute, map[auth.Audience][]byte{
		auth.AudienceMerchant: []byte(strings.Repeat("m", 32)),
		auth.AudiencePlatform: []byte(strings.Repeat("p", 32)),
	})
	if err != nil {
		t.Fatal(err)
	}
	return iss
}

func merchantToken(t *testing.T, iss *auth.Issuer) string {
	t.Helper()
	tok, _, err := iss.Issue(auth.AudienceMerchant, "user-1", auth.Claims{TenantID: "tenant-1", Role: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

// capture menyimpan header yang diterima handler di dalam gateway.
func capture(got *http.Header) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*got = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	})
}

func do(h http.Handler, path, token string, extra map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range extra {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAuthenticate_Status(t *testing.T) {
	iss := newTestIssuer(t)
	platformTok, _, _ := iss.Issue(auth.AudiencePlatform, "admin-1", auth.Claims{Roles: []string{"support"}})

	cases := []struct {
		nama  string
		path  string
		token string
		want  int
	}{
		{"rute publik tanpa token", loginPath, "", http.StatusOK},
		{"rute terlindungi tanpa token", protectedPath, "", http.StatusUnauthorized},
		{"token sampah", protectedPath, "abc.def.ghi", http.StatusUnauthorized},
		{"token platform di rute merchant", protectedPath, platformTok, http.StatusUnauthorized},
		{"token merchant valid", protectedPath, merchantToken(t, iss), http.StatusOK},
	}

	for _, c := range cases {
		t.Run(c.nama, func(t *testing.T) {
			var got http.Header
			rec := do(Authenticate(iss, capture(&got)), c.path, c.token, nil)
			if rec.Code != c.want {
				t.Fatalf("status %d, harusnya %d", rec.Code, c.want)
			}
			if c.want == http.StatusUnauthorized && !strings.Contains(rec.Body.String(), "unauthenticated") {
				t.Fatalf("body error salah: %s", rec.Body.String())
			}
		})
	}
}

func TestAuthenticate_HeaderDiisiDariToken_BukanDariClient(t *testing.T) {
	iss := newTestIssuer(t)
	var got http.Header

	// Client mencoba menyamar sebagai tenant dan role lain.
	do(Authenticate(iss, capture(&got)), protectedPath, merchantToken(t, iss), map[string]string{
		HeaderTenantID: "tenant-orang-lain",
		HeaderRole:     "super-admin",
	})

	if got.Get(HeaderTenantID) != "tenant-1" {
		t.Fatalf("tenant harus dari token: %q", got.Get(HeaderTenantID))
	}
	if got.Get(HeaderRole) != "owner" || got.Get(HeaderUserID) != "user-1" {
		t.Fatalf("role/user harus dari token: %v", got)
	}
}

func TestAuthenticate_HeaderPalsuDibuangDiRutePublik(t *testing.T) {
	iss := newTestIssuer(t)
	var got http.Header

	do(Authenticate(iss, capture(&got)), loginPath, "", map[string]string{
		HeaderTenantID: "tenant-palsu",
		HeaderUserID:   "user-palsu",
	})

	if got.Get(HeaderTenantID) != "" || got.Get(HeaderUserID) != "" {
		t.Fatalf("header internal dari client harus dibuang: %v", got)
	}
}

func TestProxy_MeneruskanHeaderKeService(t *testing.T) {
	var got http.Header
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		_, _ = w.Write([]byte("dari-upstream"))
	}))
	defer upstream.Close()

	target, _ := url.Parse(upstream.URL)
	iss := newTestIssuer(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := Authenticate(iss, NewProxy([]Route{{Prefix: "/identity.v1.", Target: target}}, http.DefaultTransport, log))

	rec := do(h, protectedPath, merchantToken(t, iss), map[string]string{HeaderTenantID: "palsu"})

	if rec.Code != http.StatusOK || rec.Body.String() != "dari-upstream" {
		t.Fatalf("respons upstream tidak diteruskan: %d %q", rec.Code, rec.Body.String())
	}
	if got.Get(HeaderTenantID) != "tenant-1" || got.Get(HeaderRole) != "owner" {
		t.Fatalf("service harus menerima identitas dari token: %v", got)
	}
}

func TestProxy_RuteTidakDikenal(t *testing.T) {
	iss := newTestIssuer(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := Authenticate(iss, NewProxy(nil, http.DefaultTransport, log))

	rec := do(h, "/foo.v1.Bar/Baz", merchantToken(t, iss), nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("harusnya 404, dapat %d", rec.Code)
	}
}

func TestProxy_UpstreamMati(t *testing.T) {
	upstream := httptest.NewServer(http.NotFoundHandler())
	target, _ := url.Parse(upstream.URL)
	upstream.Close() // sengaja dimatikan sebelum request

	iss := newTestIssuer(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := Authenticate(iss, NewProxy([]Route{{Prefix: "/identity.v1.", Target: target}}, http.DefaultTransport, log))

	rec := do(h, protectedPath, merchantToken(t, iss), nil)
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "unavailable") {
		t.Fatalf("harusnya 503 unavailable, dapat %d %s", rec.Code, rec.Body.String())
	}
}
