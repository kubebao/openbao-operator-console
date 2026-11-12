.PHONY: evaluation evaluation-open evaluation-down install dev generate check check-generated build dist e2e integration integration-cluster integration-cluster-down helm-check e2e-production screenshots lint lint-fix \
	image vulncheck audit licenses check-licenses check-release-tools e2e-storage

GOLANGCI_LINT = go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse HEAD 2>/dev/null)
IMAGE ?= openbao-operator-console:local

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

lint:
	$(GOLANGCI_LINT) run ./...

lint-fix:
	$(GOLANGCI_LINT) fmt ./...
	$(GOLANGCI_LINT) run --fix ./...

check: lint
	go test -race ./...
	pnpm --dir web typecheck
	pnpm --dir web lint
	pnpm --dir web format:check
	pnpm --dir web test
	pnpm --dir web build

build:
	./scripts/build.sh "$(VERSION)" "$(COMMIT)"

# Release archives of the static binary for the CLI commands and running without Kubernetes.
DIST_PLATFORMS ?= linux/amd64 linux/arm64 darwin/amd64 darwin/arm64

dist:
	./scripts/dist.sh $(VERSION) $(COMMIT) $(DIST_PLATFORMS)

# Disposable kind cluster with the OpenBao Operator, an OpenBao cluster, and this console.
evaluation:
	./scripts/evaluation.sh up

evaluation-open:
	./scripts/evaluation.sh open

evaluation-down:
	./scripts/evaluation.sh down

image:
	docker build --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) -t $(IMAGE) .

e2e:
	pnpm --dir web test:e2e

e2e-production: build
	CONSOLE_E2E_PRODUCTION=1 pnpm --dir web test:e2e

integration-cluster:
	./scripts/integration-cluster.sh up

integration-cluster-down:
	./scripts/integration-cluster.sh down

integration:
	test -n "$$CONSOLE_TEST_DATABASE_URL"
	go test -race -count=1 ./internal/store ./internal/collector ./internal/httpapi ./internal/renewal

CHART = charts/openbao-operator-console

# Lints and renders the chart with every values file in its ci directory, and checks that
# values.schema.json rejects invalid values.
helm-check:
	@for values in $(CHART)/ci/*-values.yaml; do \
		echo "helm lint --strict $$values"; \
		helm lint --strict $(CHART) --values $$values || exit 1; \
		helm template openbao-operator-console $(CHART) --namespace console-system --values $$values > /dev/null || exit 1; \
	done
	@if helm template openbao-operator-console $(CHART) --values $(CHART)/ci/default-values.yaml \
		--set database.sslMode=insecure > /dev/null 2>&1; then \
		echo "values.schema.json accepted an invalid database.sslMode" >&2; exit 1; \
	fi
	@if helm template openbao-operator-console $(CHART) --values $(CHART)/ci/default-values.yaml \
		--set unknownValue=true > /dev/null 2>&1; then \
		echo "values.schema.json accepted an unknown value" >&2; exit 1; \
	fi
	@if helm template openbao-operator-console $(CHART) > /dev/null 2>&1; then \
		echo "the chart rendered without existingSecret" >&2; exit 1; \
	fi
	@if helm template openbao-operator-console $(CHART) --set existingSecret=console-runtime > /dev/null 2>&1; then \
		echo "the chart rendered without publicURL or an Ingress host" >&2; exit 1; \
	fi
	@if helm template openbao-operator-console $(CHART) --values $(CHART)/ci/default-values.yaml \
		--set 'trustedProxies={0.0.0.0/0}' > /dev/null 2>&1; then \
		echo "values.schema.json accepted a catch-all trusted proxy" >&2; exit 1; \
	fi
	@if helm template openbao-operator-console $(CHART) --values $(CHART)/ci/default-values.yaml \
		--set publicURL=http://console.example.com > /dev/null 2>&1; then \
		echo "values.schema.json accepted a publicURL without HTTPS" >&2; exit 1; \
	fi
	@if helm template openbao-operator-console $(CHART) --values $(CHART)/ci/default-values.yaml \
		--set publicURL=https://console.example.com/console > /dev/null 2>&1; then \
		echo "values.schema.json accepted a publicURL with a path" >&2; exit 1; \
	fi
	@if helm template openbao-operator-console $(CHART) --values $(CHART)/ci/default-values.yaml \
		--set database.engine=unknown > /dev/null 2>&1; then \
		echo "values.schema.json accepted an unknown database engine" >&2; exit 1; \
	fi
	@if helm template openbao-operator-console $(CHART) --values $(CHART)/ci/default-values.yaml \
		--set database.urlSecret=postgres-secret > /dev/null 2>&1; then \
		echo "values.schema.json accepted PostgreSQL connection settings for SQLite" >&2; exit 1; \
	fi
	CONSOLE_TEST_HELM=1 go test -count=1 ./internal/config -run '^TestHelmReader'

screenshots: build
	pnpm --dir web exec playwright test --config playwright.screenshots.config.ts

vulncheck:
	go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...

audit:
	pnpm --dir web audit --prod

licenses:
	./scripts/third-party-licenses.sh

check-licenses:
	@scratch=$$(mktemp) && trap 'rm -f "$$scratch"' EXIT && \
		./scripts/third-party-licenses.sh "$$scratch" && \
		diff -u THIRD_PARTY_LICENSES "$$scratch" || { echo "THIRD_PARTY_LICENSES is out of date; run make licenses" >&2; exit 1; }

check-release-tools:
	python3 -m unittest discover -s scripts/tests -v
	shellcheck -x scripts/build.sh scripts/edge-candidate.sh scripts/check-edge-install.sh scripts/publish-edge.sh scripts/test-postgres-tools.sh \
		scripts/integration-cluster.sh
	go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12

# Real SQLite setup, restart, and backup restore through the production browser UI.
e2e-storage: build
	pnpm --dir web exec playwright test --config playwright.storage.config.ts
