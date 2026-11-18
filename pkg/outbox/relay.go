// Package outbox: relay yang membaca tabel outbox dan menerbitkan event ke Kafka.
//
// Tabel yang diharapkan (tiap service yang pakai outbox punya tabel sendiri
// di database-nya sendiri):
//
//	outbox(id uuid, topic text, aggregate_id text, event_type text,
//	       tenant_id uuid null, payload jsonb, created_at, published_at null)
//
// Jaminan: at-least-once. Event bisa terkirim lebih dari sekali (misalnya relay
// mati setelah kirim ke Kafka tapi sebelum commit). Consumer wajib idempotent,
// pakai header event_id.
package outbox

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/twmb/franz-go/pkg/kgo"
)

// Producer: port ke Kafka (dipenuhi *kgo.Client).
type Producer interface {
	ProduceSync(ctx context.Context, rs ...*kgo.Record) kgo.ProduceResults
}

// Config: nilai nol dipakai sebagai default.
type Config struct {
	BatchSize      int           // default 100
	Interval       time.Duration // jeda antar polling, default 500ms
	MaxBackoff     time.Duration // jeda maksimal saat error, default 30s
	PublishTimeout time.Duration // batas waktu kirim 1 batch, default 10s
}

type Relay struct {
	pool *pgxpool.Pool
	prod Producer
	log  *slog.Logger
	cfg  Config
}

func NewRelay(pool *pgxpool.Pool, prod Producer, log *slog.Logger, cfg Config) *Relay {
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 100
	}
	if cfg.Interval <= 0 {
		cfg.Interval = 500 * time.Millisecond
	}
	if cfg.MaxBackoff <= 0 {
		cfg.MaxBackoff = 30 * time.Second
	}
	if cfg.PublishTimeout <= 0 {
		cfg.PublishTimeout = 10 * time.Second
	}
	return &Relay{pool: pool, prod: prod, log: log, cfg: cfg}
}

// row: satu baris outbox yang belum terkirim.
type row struct {
	ID          string
	Topic       string
	AggregateID string
	EventType   string
	TenantID    *string // kolom nullable
	Payload     string  // JSON mentah
}

const selectPending = `
	select id, topic, aggregate_id, event_type, tenant_id, payload::text
	from outbox
	where published_at is null
	order by created_at, id
	limit $1
	for update skip locked`

func scanRow(r pgx.CollectableRow) (row, error) {
	var x row
	err := r.Scan(&x.ID, &x.Topic, &x.AggregateID, &x.EventType, &x.TenantID, &x.Payload)
	return x, err
}

// Run berjalan sampai ctx dibatalkan.
func (r *Relay) Run(ctx context.Context) {
	backoff := r.cfg.Interval
	for {
		n, err := r.publishBatch(ctx)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			r.log.ErrorContext(ctx, "outbox relay gagal", "err", err, "retry_in", backoff.String())
			if !sleep(ctx, backoff) {
				return
			}
			backoff = min(backoff*2, r.cfg.MaxBackoff)
			continue
		}

		backoff = r.cfg.Interval
		if n > 0 {
			r.log.DebugContext(ctx, "outbox terkirim", "jumlah", n)
		}
		if n == r.cfg.BatchSize {
			continue // kemungkinan masih ada antrean, langsung ambil batch berikutnya
		}
		if !sleep(ctx, r.cfg.Interval) {
			return
		}
	}
}

// publishBatch: ambil baris pending -> kirim ke Kafka -> tandai terkirim, semuanya
// dalam 1 transaksi. Kalau kirim gagal, transaksi di-rollback dan baris tetap pending.
func (r *Relay) publishBatch(ctx context.Context) (int, error) {
	sent := 0
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, selectPending, r.cfg.BatchSize)
		if err != nil {
			return err
		}
		pending, err := pgx.CollectRows(rows, scanRow)
		if err != nil {
			return err
		}
		if len(pending) == 0 {
			return nil
		}

		recs := make([]*kgo.Record, len(pending))
		ids := make([]string, len(pending))
		for i, p := range pending {
			recs[i] = toRecord(p)
			ids[i] = p.ID
		}

		pctx, cancel := context.WithTimeout(ctx, r.cfg.PublishTimeout)
		defer cancel()
		if err := r.prod.ProduceSync(pctx, recs...).FirstErr(); err != nil {
			return fmt.Errorf("kirim ke kafka: %w", err)
		}

		if _, err := tx.Exec(ctx,
			`update outbox set published_at = now() where id = any($1::text[]::uuid[])`, ids,
		); err != nil {
			return fmt.Errorf("tandai terkirim: %w", err)
		}
		sent = len(pending)
		return nil
	})
	return sent, err
}

// toRecord: baris outbox -> pesan Kafka.
//   - key    = tenant_id (kalau ada), supaya event 1 toko selalu di partisi yang sama
//   - value  = payload JSON
//   - header = event_id (untuk dedup di consumer), event_type, tenant_id
func toRecord(p row) *kgo.Record {
	key := p.AggregateID
	headers := []kgo.RecordHeader{
		{Key: "event_id", Value: []byte(p.ID)},
		{Key: "event_type", Value: []byte(p.EventType)},
	}
	if p.TenantID != nil {
		key = *p.TenantID
		headers = append(headers, kgo.RecordHeader{Key: "tenant_id", Value: []byte(*p.TenantID)})
	}
	return &kgo.Record{
		Topic:   p.Topic,
		Key:     []byte(key),
		Value:   []byte(p.Payload),
		Headers: headers,
	}
}

// sleep menunggu d, atau berhenti lebih awal kalau ctx dibatalkan (return false).
func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
