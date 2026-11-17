# Keputusan Arsitektur

## ADR-001: Pivot dari Multi-Tenant SaaS ke Marketplace

**Tanggal:** 2026-11-17
**Status:** Diterima

### Konteks

Project awalnya didesain sebagai multi-tenant SaaS (tiap toko = website sendiri). Arah
berubah ke marketplace: 1 platform, banyak toko di dalamnya (Shopee versi ringkas).

### Keputusan

#### 1. Customer = akun global

Tabel `customers` nanti **tanpa `tenant_id`**, tanpa RLS. Pembeli mendaftar sekali dan
bisa belanja di toko mana saja.

**Trade-off yang dipilih:** kalau ternyata arahnya berubah balik ke SaaS, tinggal tambah
tabel relasi `tenant_customers(tenant_id, customer_id)` — murah. Arah sebaliknya (dari
per-tenant ke global) jauh lebih mahal karena butuh deduplikasi email dan merge ID lintas
tenant.

#### 2. Token storefront tanpa `tenant_id`

Token JWT realm `storefront` tidak lagi mewajibkan `tenant_id`. Yang menentukan "beli di
toko mana" adalah isi cart, bukan identity. Validasi di `pkg/auth/jwt.go` diubah:
hanya realm `merchant` yang wajib punya `tenant_id`.

#### 3. RLS tetap aktif untuk data merchant-facing

Row Level Security **tidak dibuang** — tetap aktif untuk semua data sisi penjual: produk,
inventori, sub-order, dashboard merchant. Yang menjadi global hanya data sisi pembeli
(customers, cart, wishlist, dll).

#### 4. Konsekuensi yang dipertimbangkan nanti

Hal-hal yang belum diimplementasi tapi sudah dipertimbangkan:

- **Order parent + sub-order per toko.** Satu checkout bisa menghasilkan sub-order ke
  beberapa toko berbeda. Masing-masing sub-order masuk RLS tenant penjualnya.
- **Escrow payment + wallet penjual.** Dana pembeli masuk escrow dulu, baru dicairkan ke
  wallet penjual setelah barang diterima.
- **Pencarian lintas toko via event Kafka ke search engine.** Pencarian produk lintas toko
  tidak bisa query langsung ke DB tenant karena bentrok RLS. Solusinya: event product_updated
  di-publish ke Kafka, consumer indexing ke search engine (Meilisearch/Elasticsearch), dan
  storefront query ke search engine.

### Yang tidak berubah

- `tenantctx` + RLS — tetap untuk data merchant-facing
- Identity service (domain, service, repo, handler, proto, migration) — ini merchant auth,
  tidak terpengaruh
- Outbox + Kafka pattern
- Struktur DB / migration yang sudah ada
