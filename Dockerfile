FROM node:26-alpine@sha256:0b36e8c136b94cd4fcf02188228e76c31ad5872eef3fec8cbd2eee500cfd9e80 AS web
WORKDIR /src/web
RUN npm install -g pnpm@11.21.0
COPY web/package.json web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY api /src/api
COPY web ./
RUN pnpm build

FROM golang:1.27-alpine@sha256:8a5910f31396cd4d89662f56c68b3ae31d374308270a1c3bd96672ee5ed43414 AS go
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY --from=web /src/web/dist ./internal/httpapi/assets
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /console ./cmd/console

FROM gcr.io/distroless/static-debian13:nonroot@sha256:e2e927ec666bae08560abb3c55d0659eceabb657f56b6782ab500a9fc7f555e3
COPY --from=go /console /console
EXPOSE 8090
ENTRYPOINT ["/console"]
CMD ["--mode=live", "--listen=0.0.0.0:8090", "--config=/etc/console/enrollment.yaml"]
