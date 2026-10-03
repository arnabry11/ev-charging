IMAGE := ev-charging-gateway

.PHONY: test lint generate build

test:
	docker build --target build -t $(IMAGE):test .
	docker run --rm $(IMAGE):test go test -race ./...

lint:
	docker run --rm -v "$(CURDIR):/app" -w /app golangci/golangci-lint:v2.5.0 golangci-lint run

generate:
	docker run --rm -v "$(CURDIR):/src" -w /src sqlc/sqlc:1.30.0 generate

build:
	docker build -t $(IMAGE):local .
