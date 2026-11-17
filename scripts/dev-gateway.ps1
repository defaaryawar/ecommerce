$env:JWT_MERCHANT_SECRET = "dev-merchant-secret-ganti-minimal-32-byte"
$env:LOG_LEVEL = "debug"
go -C services/gateway run ./cmd/server