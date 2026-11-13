// Package tenantctx: menyimpan tenant_id di context dan menjalankan query
// dengan Postgres Row Level Security (RLS) aktif.
//
// Pola di service yang datanya per-tenant (product, order, customer, ...):
//
//	ALTER TABLE products ENABLE ROW LEVEL SECURITY;
//	ALTER TABLE products FORCE ROW LEVEL SECURITY; -- supaya owner tabel juga kena
//	CREATE POLICY tenant_isolation ON products
//	  USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
//	  WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);
//
// Lalu semua query dijalankan lewat InTx. Kalau lupa nulis WHERE tenant_id,
// Postgres tetap cuma mengembalikan data tenant yang aktif.
package tenantctx

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ctxKey struct{}

var ErrNoTenant = errors.New("tenantctx: tenant_id tidak ada di context")

// With menaruh tenant_id ke context (dipanggil oleh interceptor/gateway).
func With(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, ctxKey{}, tenantID)
}

// TenantID mengambil tenant_id dari context.
func TenantID(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(ctxKey{}).(string)
	return v, ok && v != ""
}

// InTx menjalankan fn di dalam transaksi dengan app.tenant_id di-set (SET LOCAL),
// jadi otomatis hilang saat transaksi selesai dan aman dipakai dengan connection pool.
func InTx(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) error {
	tid, ok := TenantID(ctx)
	if !ok {
		return ErrNoTenant
	}
	return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "select set_config('app.tenant_id', $1, true)", tid); err != nil {
			return err
		}
		return fn(tx)
	})
}
