package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/defaaryawar/ecommerce/pkg/auth"
	"github.com/defaaryawar/ecommerce/pkg/config"
	"github.com/defaaryawar/ecommerce/pkg/logger"
	"github.com/defaaryawar/ecommerce/pkg/otelx"
	"github.com/defaaryawar/ecommerce/services/gateway/internal/gateway"
)

const (
	serviceName = "gateway"
	// Harus sama dengan issuer yang dipakai identity saat menerbitkan token.
	jwtIssuer = "ecommerce-identity"
)

func main() {
	if err := run(); err != nil {
		slog.Error("gateway berhenti karena error", "err", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log := logger.New(serviceName, config.String("LOG_LEVEL", "info"))
	slog.SetDefault(log)

	shutdownTracer, err := otelx.Init(ctx, serviceName, config.String("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317"))
	if err != nil {
		return fmt.Errorf("init tracing: %w", err)
	}
	defer func() { _ = shutdownTracer(context.Background()) }()

	// Gateway cuma MEMVERIFIKASI token, jadi cukup pegang key merchant.
	issuer, err := auth.NewIssuer(jwtIssuer, time.Minute, map[auth.Audience][]byte{
		auth.AudienceMerchant: []byte(config.MustString("JWT_MERCHANT_SECRET")),
	})
	if err != nil {
		return err
	}

	identityURL, err := url.Parse(config.String("IDENTITY_URL", "http://localhost:8081"))
	if err != nil {
		return fmt.Errorf("IDENTITY_URL: %w", err)
	}

	// Tabel rute. Service baru = tambah 1 baris di sini.
	routes := []gateway.Route{
		{Prefix: "/identity.v1.", Target: identityURL},
	}

	// otelhttp.NewTransport menempelkan trace_id ke request keluar, supaya
	// identity bisa membuat span anak dari trace gateway.
	proxy := gateway.NewProxy(routes, otelhttp.NewTransport(http.DefaultTransport), log)

	// Urutan bungkus (dari luar ke dalam): trace -> log -> cek token -> proxy
	var h http.Handler = gateway.Authenticate(issuer, proxy)
	h = gateway.AccessLog(log, h)
	h = otelhttp.NewHandler(h, serviceName,
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
			return r.Method + " " + r.URL.Path
		}),
	)

	root := http.NewServeMux()
	root.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	root.Handle("/", h)

	addr := config.String("HTTP_ADDR", ":8000")
	srv := &http.Server{
		Addr:              addr,
		Handler:           root,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("gateway jalan", "addr", addr, "identity", identityURL.String())
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	log.Info("shutdown...")
	shCtx, shCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shCancel()
	return srv.Shutdown(shCtx)
}
