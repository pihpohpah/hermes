# Сборка фронтенд-ассетов: app.min.js / app.min.css собираются в образе,
# поэтому деплой не зависит от того, что собрано локально.
FROM node:22-alpine AS assets
WORKDIR /src
COPY web ./web
RUN npm install --no-save esbuild@0.25.11 \
    && ./node_modules/.bin/esbuild web/static/js/app/app.js \
        --bundle --minify --format=esm \
        --outfile=web/static/js/app/app.min.js \
    && ./node_modules/.bin/esbuild web/static/css/app.css \
        --minify \
        --outfile=web/static/css/app.min.css

# Сборка
FROM golang:1.27-alpine AS build
WORKDIR /src
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/hermes ./cmd/server

# Финальный образ
FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=build /out/hermes /app/hermes
COPY --from=assets /src/web /app/web
ENTRYPOINT ["/app/hermes"]
