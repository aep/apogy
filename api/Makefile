all: go/gen.go ts/client/index.ts

go/gen.go: openapi.yaml
	mkdir -p go
	oapi-codegen -config gen-go.yaml openapi.yaml


ts/client/index.ts: openapi.yaml
	mkdir -p ts/client
	npx openapi-typescript-codegen -i openapi.yaml -o ts/client

deps:
	go install github.com/deepmap/oapi-codegen/cmd/oapi-codegen@latest
	npm install @openapitools/openapi-generator-cli -g

.PHONY:
