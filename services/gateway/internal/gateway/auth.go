// Package gateway: pintu depan. Cek token, buang header palsu, teruskan ke service.
package gateway

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/defaaryawar/ecommerce/pkg/auth"
)

// Header yang DIISI gateway untuk service di belakangnya. Header dengan nama
// ini dari client selalu dibuang, jadi tidak bisa dipalsukan.
const (
	HeaderTenantID = "X-Tenant-Id"
	HeaderUserID   = "X-User-Id"
	HeaderRole     = "X-User-Role"
)

// Verifier: port ke pengecek token (dipenuhi *auth.Issuer).
type Verifier interface {
	Verify(aud auth.Audience, token string) (*auth.Claims, error)
}

// publicProcedures: RPC yang boleh dipanggil tanpa token.
// Default-nya semua RPC WAJIB token. Sengaja daftar putih, bukan daftar hitam:
// kalau lupa mendaftarkan RPC baru, hasilnya terkunci, bukan terbuka.
var publicProcedures = map[string]bool{
	"/identity.v1.MerchantAuthService/RegisterOwner": true,
	"/identity.v1.MerchantAuthService/Login":         true,
}

// Authenticate membungkus next dengan pengecekan token.
func Authenticate(v Verifier, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. Buang header "internal" dari client. Wajib, sebelum apa pun.
		r.Header.Del(HeaderTenantID)
		r.Header.Del(HeaderUserID)
		r.Header.Del(HeaderRole)

		if publicProcedures[r.URL.Path] {
			next.ServeHTTP(w, r)
			return
		}

		token, ok := bearerToken(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthenticated", "token tidak ada")
			return
		}
		// Sementara semua rute non-publik adalah rute merchant.
		// Nanti rute "Platform*" cek audience platform.
		claims, err := v.Verify(auth.AudienceMerchant, token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthenticated", "token tidak valid")
			return
		}

		// 2. Isi header dari token yang sudah terverifikasi.
		r.Header.Set(HeaderTenantID, claims.TenantID)
		r.Header.Set(HeaderUserID, claims.Subject)
		r.Header.Set(HeaderRole, claims.Role)
		next.ServeHTTP(w, r)
	})
}

func bearerToken(r *http.Request) (string, bool) {
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return "", false
	}
	return strings.TrimSpace(h[len(prefix):]), true
}

// writeError menulis error dalam format JSON Connect, jadi client frontend
// memperlakukannya sama dengan error dari service.
func writeError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"code": code, "message": msg})
}
