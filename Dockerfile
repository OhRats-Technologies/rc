FROM oven/bun:1.4.0-alpine AS app
WORKDIR /app
COPY package.json bun.lock ./
RUN bun install --frozen-lockfile
COPY server.ts tsconfig.json ./
COPY src ./src
COPY web ./web
RUN bun run build

FROM oven/bun:1.4.0-alpine
WORKDIR /app
COPY --from=app /app/dist ./dist
COPY public/install.sh ./static/install.sh
COPY docker-entrypoint.sh /usr/local/bin/rc-entrypoint
COPY docker/sshd_config /etc/ssh/sshd_config_rc
COPY docker/rc-ssh-authorized /usr/local/bin/rc-ssh-authorized
COPY docker/rc-ssh-bridge /usr/local/bin/rc-ssh-bridge
RUN apk add --no-cache openssh-server openssh-sftp-server su-exec tini \
 && addgroup -S rc && adduser -S -G rc -h /home/rc -s /bin/sh rc \
 && passwd -d rc \
 && mkdir -p /data \
 && chown -R rc:rc /data \
 && chmod 0755 /usr/local/bin/rc-entrypoint /usr/local/bin/rc-ssh-authorized /usr/local/bin/rc-ssh-bridge
ENV PORT=3000 DATA_DIR=/data STATIC_DIR=/app/static PUBLIC_URL=https://rc.ohrats.party NODE_ENV=production ASSET_DIR=/app/dist/assets
EXPOSE 3000
WORKDIR /app/dist
ENTRYPOINT ["/sbin/tini", "--", "/usr/local/bin/rc-entrypoint"]
CMD ["bun", "server/server.js"]
