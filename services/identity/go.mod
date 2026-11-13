module github.com/defaaryawar/ecommerce/services/identity

go 1.24.4

require (
	github.com/namalu/ecommerce/gen/go v0.0.0
	github.com/namalu/ecommerce/pkg v0.0.0
)

replace (
	github.com/namalu/ecommerce/gen/go => ../../gen/go
	github.com/namalu/ecommerce/pkg => ../../pkg
)