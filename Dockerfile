FROM oven/bun:1.4.0 AS web
WORKDIR /src
COPY package.json bun.lock tsconfig.json ./
RUN bun install --frozen-lockfile
COPY web ./web
RUN bun run build:client

FROM rust:1.98-bookworm AS rust
WORKDIR /src
COPY Cargo.toml Cargo.lock ./
COPY crates ./crates
COPY public ./public
RUN cargo build --locked --release -p rc-server --bin rc-server --bin rc-ssh-helper

FROM debian:bookworm-slim
RUN apt-get update \
 && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
      ca-certificates curl gosu openssh-server tini \
 && rm -rf /var/lib/apt/lists/* \
 && groupadd --gid 10001 rc \
 && useradd --uid 10001 --gid 10001 --home-dir /home/rc --create-home --shell /bin/sh rc \
 && passwd -d rc \
 && mkdir -p /app/assets /data /run/sshd \
 && chown -R rc:rc /data

COPY --from=rust /src/target/release/rc-server /usr/local/bin/rc-server
COPY --from=rust /src/target/release/rc-ssh-helper /usr/local/bin/rc-ssh-helper
COPY --from=web /src/dist/assets /app/assets
COPY docker-entrypoint.sh /usr/local/bin/rc-entrypoint
COPY docker/sshd_config /etc/ssh/sshd_config_rc
COPY docker/rc-ssh-authorized /usr/local/bin/rc-ssh-authorized
COPY docker/rc-ssh-bridge /usr/local/bin/rc-ssh-bridge

RUN chmod 0755 \
      /usr/local/bin/rc-server \
      /usr/local/bin/rc-ssh-helper \
      /usr/local/bin/rc-entrypoint \
      /usr/local/bin/rc-ssh-authorized \
      /usr/local/bin/rc-ssh-bridge

ENV PORT=3000 \
    DATA_DIR=/data \
    STATIC_DIR=/app/assets \
    PUBLIC_URL=https://rc.ohrats.party \
    RC_SSH_DAEMON_PORT=2222 \
    RC_SSH_INTERNAL_PORT=3001

EXPOSE 3000
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD ["/usr/local/bin/rc-server", "--healthcheck"]
WORKDIR /app
ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/rc-entrypoint"]
CMD ["/usr/local/bin/rc-server"]
