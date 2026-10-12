.PHONY: build test run docker-up docker-down tidy sdk-build playground-dev

GATEWAY_DIR := packages/gateway
SDK_DIR := packages/sdk
GO_SDK_DIR := packages/go-sdk
PLAYGROUND_DIR := packages/playground

build:
	cd $(GATEWAY_DIR) && go build -o ../../bin/gateway ./cmd/gateway

tidy:
	cd $(GATEWAY_DIR) && go mod tidy
	cd $(GO_SDK_DIR) && go mod tidy

test:
	cd $(GATEWAY_DIR) && go test ./...
	cd $(GO_SDK_DIR) && go test ./...
	cd $(SDK_DIR) && npm test

sdk-build:
	cd $(SDK_DIR) && npm install && npm run build

playground-dev: sdk-build
	cd $(PLAYGROUND_DIR) && npm install && npm run dev

run: build
	cd $(GATEWAY_DIR) && \
	JWT_SECRET=$${JWT_SECRET:-dev-secret-xxx} \
	BUCKET_CONFIG='[{"name":"uploads","endpoint":"http://localhost:9000","publicEndpoint":"http://localhost:9000","accessKey":"rustfsadmin","secretKey":"rustfsadmin","cdnDomain":"http://localhost:8081/uploads","useSSL":false}]' \
	go run ./cmd/gateway

docker-up:
	docker compose --env-file .env -f docker/docker-compose.yml up -d --build

docker-down:
	docker compose --env-file .env -f docker/docker-compose.yml down
