FROM oven/bun:1.4.4 AS web
WORKDIR /src
COPY package.json bun.lock tsconfig.json ./
RUN bun install --frozen-lockfile
COPY web ./web
COPY public ./public
COPY scripts/copy-installers.ts ./scripts/copy-installers.ts
RUN bun run build:client

FROM golang:1.26.11-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
# wasmtime-go ships LLVM LTO bitcode (.llvmbc/.llvmcmd) inside its static
# archive. Go 1.26.11+ rejects those sections in linker inputs, and a normal
# non-LTO link never uses them, so strip them before building.
RUN apt-get update -qq && apt-get install -y -qq binutils >/dev/null \
 && case "$(go env GOARCH)" in amd64) W=x86_64 ;; arm64) W=aarch64 ;; *) echo "unsupported arch" >&2; exit 1 ;; esac \
 && LIB=$(go env GOMODCACHE)/github.com/bytecodealliance/wasmtime-go/v50@v50.0.0/build/linux-$W/libwasmtime.a \
 && cp "$LIB" /tmp/libwasmtime.a && chmod u+w /tmp/libwasmtime.a \
 && objcopy --remove-section=.llvmbc --remove-section=.llvmcmd /tmp/libwasmtime.a \
 && chmod u+w "$LIB" && cp /tmp/libwasmtime.a "$LIB"
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
