package auth

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func newTestIssuer(t *testing.T, ttl time.Duration) *Issuer {
	t.Helper()
	iss, err := NewIssuer("test", ttl, map[Audience][]byte{
		AudienceMerchant: []byte(strings.Repeat("m", 32)),
		AudiencePlatform: []byte(strings.Repeat("p", 32)),
	})
	if err != nil {
		t.Fatal(err)
	}
	return iss
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

func TestKeyTerlaluPendek(t *testing.T) {
	_, err := NewIssuer("test", time.Minute, map[Audience][]byte{AudienceMerchant: []byte("pendek")})
	if err == nil {
		t.Fatal("key pendek harusnya ditolak")
	}
}
