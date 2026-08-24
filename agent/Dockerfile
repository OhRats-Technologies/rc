FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY main.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /relay-agent .

FROM alpine:3.22
RUN apk add --no-cache ca-certificates
COPY --from=build /relay-agent /usr/local/bin/relay-agent
VOLUME /data
ENV RELAY_STATE_DIR=/data
ENTRYPOINT ["relay-agent"]

