FROM golang:1.25-alpine AS agent
WORKDIR /src
COPY agent/go.mod agent/go.sum ./
RUN go mod download
COPY agent/main.go ./
RUN mkdir -p /out \
 && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/relay-agent-linux-amd64 . \
 && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o /out/relay-agent-linux-arm64 . \
 && CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/relay-agent-darwin-amd64 . \
 && CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o /out/relay-agent-darwin-arm64 .

FROM oven/bun:1.2.21-alpine
WORKDIR /app
COPY server.ts package.json ./
COPY public ./public
COPY --from=agent /out ./public/downloads
RUN mkdir -p /data
ENV PORT=3000 DATA_DIR=/data PUBLIC_URL=https://relay.ohrats.party
EXPOSE 3000
CMD ["bun", "run", "server.ts"]

