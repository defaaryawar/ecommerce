# Progres

## Selesai
- [x] Tahap 1: monorepo, go.work, go.mod (gen/go, pkg, services/identity), replace, .gitignore, .gitattributes
- [x] Tahap 2A: pg-core + pg-core-replica (replikasi teruji: streaming, replica read-only)

## Sedang / berikutnya
- [ ] Tahap 2B: pg-txn + replica, kafka, kafka-ui, redis, jaeger (lihat catatan di bawah)
- [ ] Tahap 3: proto identity v1 + buf generate
- [ ] Tahap 4: pkg/ (config, logger, otelx, auth JWT 3 realm, tenantctx RLS)
- [ ] Tahap 5: identity service (domain, app, adapter postgres, handler connect) + unit test
- [ ] Tahap 6: jalanin identity ke Postgres, cek trace di Jaeger
- [ ] Tahap 7: gateway

## Catatan
- Shell: PowerShell 5.1 (pakai `;` atau baris terpisah, bukan `&&`)
- Shortcut: function dc { docker compose -f deploy/docker-compose.yml @args }
- File .sh wajib LF
- Port: pg-core 5432, replica 5433, pg-txn 5442, replica 5443, kafka 9092, kafka-ui 8080, redis 6379, jaeger 16686/4317