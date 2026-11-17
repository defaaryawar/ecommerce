$env:DATABASE_URL = "postgres://app:app@localhost:5432/identity?sslmode=disable"
$env:JWT_MERCHANT_SECRET = "dev-merchant-secret-ganti-minimal-32-byte"
$env:JWT_PLATFORM_SECRET = "dev-platform-secret-ganti-minimal-32-byte"
$env:LOG_LEVEL = "debug"
go -C services/identity run ./cmd/server