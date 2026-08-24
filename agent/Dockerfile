FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 go build -v -trimpath -ldflags="-s -w" -o /ohrats-rc .

FROM alpine:3.22
RUN apk add --no-cache ca-certificates
COPY --from=build /ohrats-rc /usr/local/bin/ohrats-rc
VOLUME /data
ENV RC_STATE_DIR=/data
ENTRYPOINT ["ohrats-rc"]
CMD ["run"]

