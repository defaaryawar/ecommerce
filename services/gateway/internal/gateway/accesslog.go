package gateway

import (
	"log/slog"
	"net/http"
	"time"
)

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// Unwrap: supaya fitur seperti Flush tetap jalan lewat wrapper ini.
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// AccessLog mencatat satu baris log per request.
func AccessLog(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		attrs := []any{
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration_ms", time.Since(start).Milliseconds(),
		}
		// Header ini diisi Authenticate di request yang sama, jadi bisa dibaca di sini.
		if tid := r.Header.Get(HeaderTenantID); tid != "" {
			attrs = append(attrs, "tenant_id", tid)
		}
		log.InfoContext(r.Context(), "request", attrs...)
	})
}
