FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go test -race ./...
RUN CGO_ENABLED=0 go build -o /bin/mock-upi ./cmd/mock-upi

FROM alpine:3.21
RUN apk add --no-cache ca-certificates wget
COPY --from=build /bin/mock-upi /usr/local/bin/mock-upi
EXPOSE 8082
USER nobody
HEALTHCHECK --interval=5s --timeout=3s --retries=10 CMD wget -qO- http://127.0.0.1:8082/health || exit 1
ENTRYPOINT ["/usr/local/bin/mock-upi"]
