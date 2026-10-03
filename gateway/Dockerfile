FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go test -race ./...
RUN CGO_ENABLED=0 go build -o /bin/gateway ./cmd/gateway

FROM alpine:3.21
RUN apk add --no-cache ca-certificates wget
COPY --from=build /bin/gateway /usr/local/bin/gateway
EXPOSE 8080 9000
USER nobody
HEALTHCHECK --interval=5s --timeout=3s --retries=10 CMD wget -qO- http://127.0.0.1:8080/health || exit 1
ENTRYPOINT ["/usr/local/bin/gateway"]
