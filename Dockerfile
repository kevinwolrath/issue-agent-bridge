FROM golang:1.26.6-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/bridge ./cmd/bridge

FROM alpine:3.24
RUN addgroup -g 10001 bridge && adduser -D -u 10001 -G bridge bridge \
    && mkdir /data && chown bridge:bridge /data
COPY --from=build /out/bridge /usr/local/bin/bridge
USER bridge
WORKDIR /data
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/bridge"]
