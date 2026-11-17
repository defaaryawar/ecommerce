module github.com/defaaryawar/ecommerce/services/identity

go 1.26.0

require github.com/defaaryawar/ecommerce/pkg v0.0.0-20261117043332-e5114d4c1261

require (
	github.com/golang-jwt/jwt/v5 v5.3.1 // indirect
	golang.org/x/crypto v0.59.0
)

replace (
	github.com/namalu/ecommerce/gen/go => ../../gen/go
	github.com/namalu/ecommerce/pkg => ../../pkg
)
