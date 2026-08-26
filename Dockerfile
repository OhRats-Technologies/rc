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
RUN apk add --no-cache su-exec \
 && addgroup -S rc && adduser -S -G rc rc \
 && mkdir -p /data \
 && chown -R rc:rc /data \
 && chmod 0755 /usr/local/bin/rc-entrypoint
ENV PORT=3000 DATA_DIR=/data STATIC_DIR=/app/static PUBLIC_URL=https://rc.ohrats.party NODE_ENV=production ASSET_DIR=/app/dist/assets
EXPOSE 3000
WORKDIR /app/dist
ENTRYPOINT ["/usr/local/bin/rc-entrypoint"]
CMD ["bun", "server/server.js"]
