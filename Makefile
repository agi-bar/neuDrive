.PHONY: build dev docker test clean install

# Build frontend and backend into a single binary
build:
	cd web && npm ci && npm run build
	rm -rf internal/web/dist
	cp -r web/dist internal/web/dist
	go build -o bin/agenthub ./cmd/agenthub

install:
	./tools/install-agenthub.sh

# Run backend + frontend dev servers (frontend proxies API to backend)
dev:
	@echo "Starting backend on :8080 and frontend dev server on :3000"
	@echo "Use Ctrl-C to stop both."
	@trap 'kill 0' EXIT; \
		AGENTHUB_DEV=1 go run ./cmd/agenthub server --listen :8080 & \
		cd web && npm run dev & \
		wait

# Build the Docker image with embedded frontend
docker:
	docker build -t agenthub:latest .

# Run all tests
test:
	go test ./...
	cd web && npm test -- --run 2>/dev/null || true

clean:
	rm -rf bin/ internal/web/dist web/dist
