FROM node:22-bookworm-slim AS web
WORKDIR /src
COPY web/package*.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build
FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/runeamp ./cmd/runeamp
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ffmpeg ca-certificates && rm -rf /var/lib/apt/lists/* \
 && groupadd -g 10001 runeamp && useradd -u 10001 -g 10001 -M -s /usr/sbin/nologin runeamp \
 && mkdir -p /data /uploads /music /app/web && chown -R 10001:10001 /data /uploads
COPY --from=build /out/runeamp /usr/local/bin/runeamp
COPY --from=web /src/dist /app/web
COPY LICENSE /app/LICENSE
USER 10001:10001
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/runeamp"]
