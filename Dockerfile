# Build stages run on the build platform and cross-compile for the target platform.

FROM --platform=$BUILDPLATFORM node:26-alpine@sha256:6bd9ab43c0ec0190ca7b2d193906cb8b7edf6e7f4afa43ddeccd6452710bffb5 AS web
# Node.js 26 no longer bundles Corepack. npm ci verifies the bootstrap archive against
# the lockfile; Corepack verifies pnpm against the hash in web/package.json.
COPY tools/corepack/package.json tools/corepack/package-lock.json /opt/corepack/
RUN npm ci --prefix /opt/corepack --ignore-scripts --no-audit --no-fund
ENV PATH="/opt/corepack/node_modules/.bin:${PATH}" \
    COREPACK_ENABLE_DOWNLOAD_PROMPT=0 \
    COREPACK_DEFAULT_TO_LATEST=0
WORKDIR /src/web
COPY web/package.json web/pnpm-lock.yaml ./
RUN corepack install && pnpm install --frozen-lockfile
COPY api /src/api
COPY web ./
RUN pnpm build

FROM --platform=$BUILDPLATFORM golang:1.27.4-alpine@sha256:0a5f7fe796599ff487605987432e7619ae45beeb2fba4c2ce0b907879932aebf AS go
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY --from=web /src/web/dist ./internal/httpapi/assets
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=""
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath \
    -ldflags="-s -w -X github.com/kubebao/openbao-operator-console/internal/buildinfo.Version=${VERSION} -X github.com/kubebao/openbao-operator-console/internal/buildinfo.Commit=${COMMIT}" \
    -o /console ./cmd/console

FROM gcr.io/distroless/static-debian13:nonroot@sha256:aafe993665bf6200db38a02c11a9351e782a84b6a45e7f33b6695638c88e3931
ARG VERSION=dev
ARG COMMIT=""
LABEL org.opencontainers.image.title="OpenBao Operator Console" \
      org.opencontainers.image.description="Read-only multi-cluster fleet console for the OpenBao Operator" \
      org.opencontainers.image.source="https://github.com/kubebao/openbao-operator-console" \
      org.opencontainers.image.url="https://github.com/kubebao/openbao-operator-console" \
      org.opencontainers.image.documentation="https://github.com/kubebao/openbao-operator-console/tree/main/docs" \
      org.opencontainers.image.vendor="KubeBao" \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}"
COPY LICENSE THIRD_PARTY_LICENSES /licenses/
COPY --from=go /console /console
USER 65532:65532
EXPOSE 8090
ENTRYPOINT ["/console"]
CMD ["--mode=live", "--listen=0.0.0.0:8090", "--config=/etc/console/enrollment.yaml"]
