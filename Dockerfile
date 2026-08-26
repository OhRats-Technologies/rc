ARG RC_RELEASE_BUILDER_PLATFORM=linux/amd64
FROM --platform=${RC_RELEASE_BUILDER_PLATFORM} golang:1.25.13-alpine AS agent-build
WORKDIR /src
COPY agent/go.mod agent/go.sum ./
RUN go mod download
COPY agent ./
RUN mkdir -p /out \
 && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -v -trimpath -ldflags="-s -w -buildid=" -o /out/ohrats-rc-linux-amd64 . \
 && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -v -trimpath -ldflags="-s -w -buildid=" -o /out/ohrats-rc-linux-arm64 . \
 && CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -v -trimpath -ldflags="-s -w -buildid=" -o /out/ohrats-rc-darwin-amd64 . \
 && CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -v -trimpath -ldflags="-s -w -buildid=" -o /out/ohrats-rc-darwin-arm64 .

FROM scratch AS release
COPY --from=agent-build /out /

FROM agent-build AS agent-verified
COPY release/manifest.json /release/manifest.json
COPY release/manifest.sig /release/manifest.sig
RUN go run ./cmd/verify-release /release/manifest.json /release/manifest.sig /out 0.9.2

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
COPY --from=agent-verified /out ./static/downloads
COPY release/manifest.json ./static/downloads/release.json
COPY release/manifest.sig ./static/downloads/release.json.sig
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
