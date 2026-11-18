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

#### 5. Tenant = toko/penjual
`tenant_id` hanya melekat pada data milik toko. Data pembeli dan platform tidak punya tenant_id.

#### 6. Jalur baca pembeli
- Halaman satu toko: slug -> tenant_id, lalu query via tenantctx.InTx (RLS tetap berlaku).
- Lintas toko (search, beranda): dari search index yang dibangun lewat event Kafka.
- Dilarang membuat role BYPASSRLS untuk kebutuhan storefront.

#### 7. Sub-order dan akses pembeli
Sub-order menyimpan tenant_id dan buyer_id. Policy RLS: satu untuk penjual (tenant_id),
satu untuk pembeli (buyer_id).

#### 8. Data pembeli
customers/alamat dilindungi berdasarkan pemilik (customer_id), bukan tenant.

## 11. Outbox relay
- Relay = goroutine polling (500ms) pakai FOR UPDATE SKIP LOCKED, library franz-go.
- Jaminan at-least-once. Consumer wajib idempotent pakai header event_id.
- Key pesan = tenant_id. Nama topic: <service>.<aggregate>.events.v1 (kolom topic di tabel outbox).
- Auto-create topic hanya untuk dev.

### Perubahan yang ikut terjadi
- pkg/auth: tenant_id wajib hanya untuk realm merchant.
- Gateway: X-Tenant-Id hanya diisi di rute merchant.
- Identity: menambah realm storefront (kredensial pembeli).
- Event baru mengikuti konvensi <service>.<aggregate>.<kejadian>.v<versi>.

### Pertanyaan terbuka
Fitur wajib versi pertama, model payment (escrow/split), kurir, skala tahun pertama.
Keputusan arah dikonfirmasi oleh: <nama lead>, 2026-11-17.