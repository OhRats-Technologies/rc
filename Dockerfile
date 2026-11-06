FROM oven/bun:1.4.4 AS web
WORKDIR /src
COPY package.json bun.lock tsconfig.json ./
RUN bun install --frozen-lockfile
COPY web ./web
RUN bun run build:client

FROM golang:1.27-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY pkg ./pkg
COPY cmd ./cmd
RUN CGO_ENABLED=1 go build -trimpath -o /out/rc-server ./cmd/rc-server \
 && CGO_ENABLED=1 go build -trimpath -o /out/rc ./cmd/rc

FROM debian:bookworm-slim
RUN apt-get update \
 && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
      ca-certificates curl gosu procps tini \
 && rm -rf /var/lib/apt/lists/* \
 && groupadd --gid 10001 rc \
 && useradd --uid 10001 --gid 10001 --home-dir /home/rc --create-home --shell /bin/sh rc \
 && passwd -d rc \
 && mkdir -p /app/assets /data \
 && chown -R rc:rc /data

COPY --from=build /out/rc-server /usr/local/bin/rc-server
COPY --from=build /out/rc /usr/local/bin/rc
COPY --from=web /src/dist/assets /app/assets
COPY docker-entrypoint.sh /usr/local/bin/rc-entrypoint

RUN chmod 0755 /usr/local/bin/rc-server /usr/local/bin/rc /usr/local/bin/rc-entrypoint

ENV PORT=3000 \
    RC_DATA_DIR=/data \
    RC_ASSETS_DIR=/app/assets \
    PUBLIC_URL=https://rc.ohrats.party

EXPOSE 3000
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD ["/usr/local/bin/rc-server", "--healthcheck"]
WORKDIR /app
ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/rc-entrypoint"]
CMD ["/usr/local/bin/rc-server"]
