-include .env
export

.PHONY: generate
generate:
	go tool oapi-codegen \
  -generate types,chi-server \
  -package api \
  -o internal/generated/api.gen.go \
  contracts/openapi/trip-service.openapi.yaml

.PHONY: migrate
migrate:
	goose -dir migrations postgres "$(DATABASE_URL)" up

.PHONY: run
run:
	go run cmd/trip-service/main.go


.PHONY: lint
lint:
	golangci-lint fmt --config=.golangci.yml
	golangci-lint run --config=.golangci.yml

.PHONY: test test-integration

test:
	CGO_ENABLED=1 go test -race -coverprofile coverage.out -coverpkg "./internal/domain/...,./internal/usecase/..." ./...
	go tool cover -func coverage.out | awk '/^total:/ { found=1; print; gsub("%", "", $$3); if ($$3 + 0 < 50) exit 1 } END { if (!found) exit 1 }'

test-integration:
	CGO_ENABLED=1 go test -v -tags=integration ./...
