module github.com/ilhasoft/wwcs

go 1.16

require (
	github.com/adjust/rmq/v4 v4.0.1
	github.com/aws/aws-sdk-go v1.38.8
	github.com/go-playground/universal-translator v0.17.0 // indirect
	github.com/go-playground/validator v9.31.0+incompatible
	github.com/go-redis/redis/v8 v8.11.4
	github.com/gorilla/websocket v1.4.2
	github.com/jinzhu/configor v1.2.1
	github.com/leodido/go-urn v1.2.0 // indirect
	github.com/prometheus/client_golang v1.11.0
	github.com/sirupsen/logrus v1.7.1
	github.com/stretchr/testify v1.6.1
	gopkg.in/go-playground/assert.v1 v1.2.1 // indirect
)

replace github.com/adjust/rmq/v4 v4.0.1 => github.com/rasoro/rmq/v4 v4.0.2
