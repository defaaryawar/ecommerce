package auth

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func newTestIssuer(t *testing.T, ttl time.Duration) *Issuer {
	t.Helper()
	iss, err := NewIssuer("test", ttl, map[Audience][]byte{
		AudienceMerchant:   []byte(strings.Repeat("m", 32)),
		AudiencePlatform:   []byte(strings.Repeat("p", 32)),
		AudienceStorefront: []byte(strings.Repeat("s", 32)),
	})
	if err != nil {
		t.Fatal(err)
	}
	return iss
}

// forgeToken menandatangani token LANGSUNG (melewati Issue), untuk mensimulasikan
// token yang tanda tangannya valid tapi isinya melanggar aturan.
func forgeToken(t *testing.T, key []byte, aud Audience, c Claims) string {
	t.Helper()
	now := time.Now()
	c.RegisteredClaims = jwt.RegisteredClaims{
		Issuer:    "test",
		Subject:   "u-1",
		Audience:  jwt.ClaimStrings{string(aud)},
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute)),
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func TestStorefrontTanpaTenant_Diterima(t *testing.T) {
	iss := newTestIssuer(t, time.Minute)
	tok, _, err := iss.Issue(AudienceStorefront, "buyer-1", Claims{})
	if err != nil {
		t.Fatalf("token pembeli tanpa tenant harusnya bisa dibuat: %v", err)
	}
	c, err := iss.Verify(AudienceStorefront, tok)
	if err != nil {
		t.Fatalf("token pembeli harusnya valid: %v", err)
	}
	if c.Subject != "buyer-1" || c.TenantID != "" {
		t.Fatalf("isi token salah: %+v", c)
	}
}

func TestStorefrontDenganTenant_Ditolak(t *testing.T) {
	iss := newTestIssuer(t, time.Minute)
	if _, _, err := iss.Issue(AudienceStorefront, "buyer-1", Claims{TenantID: "tenant-1"}); err == nil {
		t.Fatal("token pembeli tidak boleh membawa tenant_id")
	}
}

func TestVerify_MerchantTanpaTenant_DitolakWalauTandaTanganValid(t *testing.T) {
	iss := newTestIssuer(t, time.Minute)
	tok := forgeToken(t, []byte(strings.Repeat("m", 32)), AudienceMerchant, Claims{Role: "owner"})

	if _, err := iss.Verify(AudienceMerchant, tok); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("token merchant tanpa tenant_id harusnya ditolak, dapat: %v", err)
	}
}

func TestVerify_StorefrontDenganTenant_DitolakWalauTandaTanganValid(t *testing.T) {
	iss := newTestIssuer(t, time.Minute)
	tok := forgeToken(t, []byte(strings.Repeat("s", 32)), AudienceStorefront, Claims{TenantID: "tenant-1"})

	if _, err := iss.Verify(AudienceStorefront, tok); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("token pembeli dengan tenant_id harusnya ditolak, dapat: %v", err)
	}
}

func TestTokenStorefrontDanMerchant_TidakBisaTertukar(t *testing.T) {
	iss := newTestIssuer(t, time.Minute)
	buyer, _, _ := iss.Issue(AudienceStorefront, "buyer-1", Claims{})
	seller, _, _ := iss.Issue(AudienceMerchant, "user-1", Claims{TenantID: "tenant-1", Role: "owner"})

	if _, err := iss.Verify(AudienceMerchant, buyer); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("token pembeli harus ditolak di realm merchant, dapat: %v", err)
	}
	if _, err := iss.Verify(AudienceStorefront, seller); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("token penjual harus ditolak di realm storefront, dapat: %v", err)
	}
}

func TestIssueDanVerify(t *testing.T) {
	iss := newTestIssuer(t, time.Minute)
	tok, _, err := iss.Issue(AudienceMerchant, "user-1", Claims{TenantID: "tenant-1", Role: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	c, err := iss.Verify(AudienceMerchant, tok)
	if err != nil {
		t.Fatalf("token valid malah ditolak: %v", err)
	}
	if c.Subject != "user-1" || c.TenantID != "tenant-1" || c.Role != "owner" {
		t.Fatalf("isi token salah: %+v", c)
	}
}

func TestTokenMerchantDitolakDiPlatform(t *testing.T) {
	iss := newTestIssuer(t, time.Minute)
	tok, _, _ := iss.Issue(AudienceMerchant, "user-1", Claims{TenantID: "tenant-1"})
	if _, err := iss.Verify(AudiencePlatform, tok); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("token merchant harusnya ditolak di platform, dapat: %v", err)
	}
}

func TestTokenDiubahDitolak(t *testing.T) {
	iss := newTestIssuer(t, time.Minute)
	tok, _, _ := iss.Issue(AudienceMerchant, "user-1", Claims{TenantID: "tenant-1"})
	palsu := tok[:len(tok)-2] + "xx" // rusak bagian stempel
	if _, err := iss.Verify(AudienceMerchant, palsu); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("token yang diubah harusnya ditolak, dapat: %v", err)
	}
}

func TestTokenKedaluwarsa(t *testing.T) {
	iss := newTestIssuer(t, -time.Minute) // TTL negatif = langsung kedaluwarsa
	tok, _, _ := iss.Issue(AudienceMerchant, "user-1", Claims{TenantID: "tenant-1"})
	if _, err := iss.Verify(AudienceMerchant, tok); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("token kedaluwarsa harusnya ditolak, dapat: %v", err)
	}
}

func TestMerchantWajibTenant(t *testing.T) {
	iss := newTestIssuer(t, time.Minute)
	if _, _, err := iss.Issue(AudienceMerchant, "user-1", Claims{}); err == nil {
		t.Fatal("token merchant tanpa tenant_id harusnya gagal dibuat")
	}
}

func TestStorefrontBolehTanpaTenant(t *testing.T) {
	iss := newTestIssuer(t, time.Minute)
	tok, _, err := iss.Issue(AudienceStorefront, "buyer-1", Claims{})
	if err != nil {
		t.Fatalf("storefront tanpa tenant_id harusnya boleh, dapat error: %v", err)
	}
	c, err := iss.Verify(AudienceStorefront, tok)
	if err != nil {
		t.Fatalf("token storefront valid ditolak: %v", err)
	}
	if c.Subject != "buyer-1" {
		t.Fatalf("subject salah: %s", c.Subject)
	}
	if c.TenantID != "" {
		t.Fatalf("storefront global seharusnya tanpa tenant_id, dapat: %s", c.TenantID)
	}
}

func TestStorefrontBolehDenganTenant(t *testing.T) {
	iss := newTestIssuer(t, time.Minute)
	tok, _, err := iss.Issue(AudienceStorefront, "buyer-1", Claims{TenantID: "tenant-1"})
	if err != nil {
		t.Fatalf("storefront dengan tenant_id harusnya boleh juga, dapat error: %v", err)
	}
	c, err := iss.Verify(AudienceStorefront, tok)
	if err != nil {
		t.Fatalf("token storefront valid ditolak: %v", err)
	}
	if c.TenantID != "tenant-1" {
		t.Fatalf("tenant_id salah: %s", c.TenantID)
	}
}

func TestKeyTerlaluPendek(t *testing.T) {
	_, err := NewIssuer("test", time.Minute, map[Audience][]byte{AudienceMerchant: []byte("pendek")})
	if err == nil {
		t.Fatal("key pendek harusnya ditolak")
	}
}
