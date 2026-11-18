package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connecthttp"
	"connectrpc.com/grpcreflect/v2"
	"connectrpc.com/otelconnect"
	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/defaaryawar/ecommerce/gen/go/identity/v1/identityv1connect"
	"github.com/defaaryawar/ecommerce/pkg/auth"
	"github.com/defaaryawar/ecommerce/pkg/config"
	"github.com/defaaryawar/ecommerce/pkg/logger"
	"github.com/defaaryawar/ecommerce/pkg/otelx"
	"github.com/defaaryawar/ecommerce/pkg/outbox"
	"github.com/defaaryawar/ecommerce/services/identity/internal/adapter/grpcadapter"
	"github.com/defaaryawar/ecommerce/services/identity/internal/adapter/postgres"
	"github.com/defaaryawar/ecommerce/services/identity/internal/app"
	"github.com/defaaryawar/ecommerce/services/identity/migrations"
)

const serviceName = "identity"

func main() {
	if err := run(); err != nil {
		slog.Error("service berhenti karena error", "err", err)
		os.Exit(1)
	}
}

func run() error {
	// ctx ini batal otomatis saat Ctrl+C atau SIGTERM (docker stop).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log := logger.New(serviceName, config.String("LOG_LEVEL", "info"))
	slog.SetDefault(log)

	// --- Tracing ---
	shutdownTracer, err := otelx.Init(ctx, serviceName, config.String("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317"))
	if err != nil {
		return fmt.Errorf("init tracing: %w", err)
	}
	defer func() { _ = shutdownTracer(context.Background()) }()

	// --- Database ---
	pgCfg, err := pgxpool.ParseConfig(config.MustString("DATABASE_URL"))
	if err != nil {
		return fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	pgCfg.ConnConfig.Tracer = otelpgx.NewTracer() // tiap query muncul sebagai span di Jaeger
	pool, err := pgxpool.NewWithConfig(ctx, pgCfg)
	if err != nil {
		return fmt.Errorf("connect db: %w", err)
	}
	defer pool.Close()

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		return fmt.Errorf("ping db: %w", err)
	}

	// Dev: migrasi jalan otomatis saat start. Production: jalankan sebagai job terpisah.
	if config.Bool("MIGRATE_ON_START", true) {
		if err := migrate(ctx, pool); err != nil {
			return fmt.Errorf("migrasi: %w", err)
		}
		log.Info("migrasi selesai")
	}

	// --- Outbox relay -> Kafka (aktif kalau KAFKA_BROKERS diisi) ---
	if brokers := config.String("KAFKA_BROKERS", ""); brokers != "" {
		kc, err := kgo.NewClient(
			kgo.SeedBrokers(strings.Split(brokers, ",")...),
			kgo.AllowAutoTopicCreation(), // dev saja. Production: topic dibuat eksplisit
			kgo.RecordDeliveryTimeout(10*time.Second),
		)
		if err != nil {
			return fmt.Errorf("kafka client: %w", err)
		}
		defer kc.Close()

		relayCtx, stopRelay := context.WithCancel(ctx)
		relayDone := make(chan struct{})
		go func() {
			defer close(relayDone)
			outbox.NewRelay(pool, kc, log, outbox.Config{}).Run(relayCtx)
		}()
		// Urutan defer (LIFO): relay berhenti dulu, baru Kafka client dan pool ditutup.
		defer func() { stopRelay(); <-relayDone }()
		log.Info("outbox relay aktif", "brokers", brokers)
	}

	// --- Wiring: susun dari lapis paling dalam ke luar ---
	issuer, err := auth.NewIssuer("ecommerce-identity", config.Duration("JWT_TTL", 15*time.Minute),
		map[auth.Audience][]byte{
			auth.AudienceMerchant: []byte(config.MustString("JWT_MERCHANT_SECRET")),
			auth.AudiencePlatform: []byte(config.MustString("JWT_PLATFORM_SECRET")),
		})
	if err != nil {
		return err
	}
	svc := app.NewService(postgres.New(pool), issuer, log)

	// Tracing RPC. WithTrustRemote: service ini internal, jadi percaya trace dari
	// pemanggil (gateway) dan bikin span anak, bukan trace baru.
	otelServer, err := otelconnect.NewServerInterceptor(otelconnect.WithTrustRemote())
	if err != nil {
		return fmt.Errorf("otel interceptor: %w", err)
	}

	// --- Server Connect v2 ---
	server := connect.NewServer(otelServer)
	identityv1connect.RegisterMerchantAuthServiceHandler(server, grpcadapter.NewHandler(svc, log))
	// Reflection: daftarkan SETELAH semua service ter-register.
	grpcreflect.Register(server)

	rpcMux := http.NewServeMux()
	connecthttp.Mount(rpcMux, server)

	// Router luar: /healthz dipisah supaya tidak bentrok dengan route RPC.
	root := http.NewServeMux()
	root.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		hctx, hcancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer hcancel()
		if err := pool.Ping(hctx); err != nil {
			http.Error(w, "db tidak bisa dihubungi", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ok"))
	})
	root.Handle("/", rpcMux)

	// Satu port melayani gRPC (HTTP/2 tanpa TLS), gRPC-Web, dan Connect (HTTP/1.1 JSON).
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)

	addr := config.String("HTTP_ADDR", ":8081")
	srv := &http.Server{
		Addr:              addr,
		Handler:           root,
		Protocols:         protocols,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("identity service jalan", "addr", addr)
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

func migrate(ctx context.Context, pool *pgxpool.Pool) error {
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	return goose.UpContext(ctx, db, ".")
}
