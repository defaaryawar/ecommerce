package gateway

import (
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

// Route: semua path berawalan Prefix diteruskan ke Target.
// Path RPC berbentuk /<package>.<Service>/<Method>, jadi prefix "/identity.v1."
// mencakup semua service di package identity.v1.
type Route struct {
	Prefix string
	Target *url.URL
}

type routeProxy struct {
	prefix string
	proxy  *httputil.ReverseProxy
}

// NewProxy membuat handler yang meneruskan request sesuai tabel rute.
func NewProxy(routes []Route, transport http.RoundTripper, log *slog.Logger) http.Handler {
	rps := make([]routeProxy, 0, len(routes))
	for _, rt := range routes {
		target := rt.Target
		rps = append(rps, routeProxy{
			prefix: rt.Prefix,
			proxy: &httputil.ReverseProxy{
				Rewrite: func(pr *httputil.ProxyRequest) {
					pr.SetURL(target)
					pr.SetXForwarded()
				},
				Transport: transport,
				ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
					log.ErrorContext(r.Context(), "upstream gagal", "path", r.URL.Path, "err", err)
					writeError(w, http.StatusServiceUnavailable, "unavailable", "service tidak tersedia")
				},
			},
		})
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, rp := range rps {
			if strings.HasPrefix(r.URL.Path, rp.prefix) {
				rp.proxy.ServeHTTP(w, r)
				return
			}
		}
		writeError(w, http.StatusNotFound, "not_found", "rute tidak ditemukan")
	})
}
