.PHONY: install dev generate check check-generated build e2e integration helm-check e2e-production screenshots

install:
	go mod download
	pnpm --dir web install --frozen-lockfile

dev:
	./scripts/dev.sh

generate:
	go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0 --config api/codegen.yaml api/openapi.yaml
	go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate
	pnpm --dir web generate
	pnpm --dir web exec prettier --write src/api/generated.ts

check-generated:
	./scripts/check-generated.sh

check:
	test -z "$$(gofmt -l cmd internal)"
	go vet ./...
	go test -race ./...
	pnpm --dir web typecheck
	pnpm --dir web lint
	pnpm --dir web format:check
	pnpm --dir web test
	pnpm --dir web build

build:
	pnpm --dir web build
	cp -R web/dist/. internal/httpapi/assets/
	mkdir -p bin
	CGO_ENABLED=0 go build -trimpath -o bin/console ./cmd/console

e2e:
	pnpm --dir web test:e2e

e2e-production: build
	CONSOLE_E2E_PRODUCTION=1 pnpm --dir web test:e2e

integration:
	test -n "$$CONSOLE_TEST_DATABASE_URL"
	go test -race -count=1 ./internal/store ./internal/collector ./internal/httpapi ./internal/renewal

helm-check:
	helm lint charts/openbao-operator-console --set existingSecret=console-runtime
	helm template console charts/openbao-operator-console --set existingSecret=console-runtime > /dev/null

screenshots: build
	pnpm --dir web exec playwright test --config playwright.screenshots.config.ts
