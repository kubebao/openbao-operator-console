# Build stages run on the build platform and cross-compile for the target platform.

FROM --platform=$BUILDPLATFORM node:26-alpine@sha256:0b36e8c136b94cd4fcf02188228e76c31ad5872eef3fec8cbd2eee500cfd9e80 AS web
# Node.js 26 no longer bundles Corepack. Install a pinned Corepack, which then provides the
# pnpm version declared in the packageManager field of web/package.json.
ENV COREPACK_ENABLE_DOWNLOAD_PROMPT=0
RUN npm install --global --no-audit --no-fund corepack@0.36.0 && corepack enable pnpm
WORKDIR /src/web
COPY web/package.json web/pnpm-lock.yaml ./
RUN corepack install && pnpm install --frozen-lockfile
COPY api /src/api
COPY web ./
RUN pnpm build

FROM --platform=$BUILDPLATFORM golang:1.27.2-alpine@sha256:f92b6ef800e499660581efdabdf25d9d817a9d124eaf900924f0504e7e27e12d AS go
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

FROM gcr.io/distroless/static-debian13:nonroot@sha256:e2e927ec666bae08560abb3c55d0659eceabb657f56b6782ab500a9fc7f555e3
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
