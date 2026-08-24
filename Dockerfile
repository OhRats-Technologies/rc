FROM golang:1.25-alpine AS agent
WORKDIR /src
COPY agent/go.mod agent/go.sum ./
RUN go mod download
COPY agent/*.go ./
RUN mkdir -p /out \
 && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/ohrats-relay-linux-amd64 . \
 && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o /out/ohrats-relay-linux-arm64 . \
 && CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/ohrats-relay-darwin-amd64 . \
 && CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o /out/ohrats-relay-darwin-arm64 .

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
COPY --from=agent /out ./static/downloads
RUN mkdir -p /data
ENV PORT=3000 DATA_DIR=/data STATIC_DIR=/app/static PUBLIC_URL=https://relay.ohrats.party NODE_ENV=production ASSET_DIR=/app/dist/assets
EXPOSE 3000
WORKDIR /app/dist
CMD ["bun", "server/server.js"]

