FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /bin/simulator ./cmd/simulator

FROM alpine:3.21
RUN apk add --no-cache ca-certificates wget
COPY --from=build /bin/simulator /usr/local/bin/simulator
EXPOSE 8081
USER nobody
HEALTHCHECK --interval=5s --timeout=3s --retries=20 CMD wget -qO- http://127.0.0.1:8081/health || exit 1
ENTRYPOINT ["/usr/local/bin/simulator"]
