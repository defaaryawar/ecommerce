// Package auth: penerbitan & verifikasi JWT untuk 3 realm (platform, merchant, storefront).
// Tiap realm punya signing key sendiri, jadi token dari satu realm tidak bisa dipakai di realm lain.
//
// Catatan: untuk dev pakai HS256. Di production, ganti ke EdDSA/RS256 supaya service lain
// cukup pegang public key.
package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Audience = realm token.
type Audience string

const (
	AudiencePlatform   Audience = "platform"   // tim internal aplikasi
	AudienceMerchant   Audience = "merchant"   // pemilik & staff toko
	AudienceStorefront Audience = "storefront" // customer / pembeli
)

// Claims isi token. TenantID wajib untuk merchant & storefront, kosong untuk platform.
type Claims struct {
	TenantID string   `json:"tenant_id,omitempty"`
	Role     string   `json:"role,omitempty"`  // role merchant: owner/admin/staff
	Roles    []string `json:"roles,omitempty"` // role platform: super_admin/support/...
	jwt.RegisteredClaims
}

var ErrInvalidToken = errors.New("auth: token tidak valid")

// Issuer menerbitkan dan memverifikasi token.
type Issuer struct {
	issuer string
	ttl    time.Duration
	keys   map[Audience][]byte
}

// NewIssuer: keys berisi signing key per audience (minimal 32 byte).
func NewIssuer(issuer string, ttl time.Duration, keys map[Audience][]byte) (*Issuer, error) {
	for aud, k := range keys {
		if len(k) < 32 {
			return nil, fmt.Errorf("auth: key untuk audience %q minimal 32 byte", aud)
		}
	}
	return &Issuer{issuer: issuer, ttl: ttl, keys: keys}, nil
}

// Issue membuat access token untuk subject (user id) di realm aud.
func (i *Issuer) Issue(aud Audience, subject string, c Claims) (string, time.Time, error) {
	key, ok := i.keys[aud]
	if !ok {
		return "", time.Time{}, fmt.Errorf("auth: audience %q tidak dikonfigurasi", aud)
	}
	if aud != AudiencePlatform && c.TenantID == "" {
		return "", time.Time{}, fmt.Errorf("auth: token %q wajib punya tenant_id", aud)
	}
	now := time.Now()
	exp := now.Add(i.ttl)
	c.RegisteredClaims = jwt.RegisteredClaims{
		Issuer:    i.issuer,
		Subject:   subject,
		Audience:  jwt.ClaimStrings{string(aud)},
		IssuedAt:  jwt.NewNumericDate(now),
		NotBefore: jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(exp),
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(key)
	if err != nil {
		return "", time.Time{}, err
	}
	return tok, exp, nil
}

// Verify memvalidasi token untuk realm aud (signature, issuer, audience, expiry).
func (i *Issuer) Verify(aud Audience, token string) (*Claims, error) {
	key, ok := i.keys[aud]
	if !ok {
		return nil, fmt.Errorf("auth: audience %q tidak dikonfigurasi", aud)
	}
	c := &Claims{}
	_, err := jwt.ParseWithClaims(token, c,
		func(*jwt.Token) (any, error) { return key, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithAudience(string(aud)),
		jwt.WithIssuer(i.issuer),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	return c, nil
}
