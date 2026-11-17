.PHONY: build frontend dev test check test-e2e test-integration clean

# Production assets are committed, so this target needs only Go.
build:
	CGO_ENABLED=0 go build -trimpath -o bin/sciurus ./cmd/sciurus

frontend:
	cd web && yarn install --frozen-lockfile && yarn build

dev: build
	./scripts/dev.sh

test:
	go test -race ./...
	cd web && yarn test

check:
	go vet ./...
	cd web && yarn run check && yarn examples:check

test-e2e: frontend
	cd web && yarn test:e2e

test-integration:
	REQUIRE_MYSQL=1 ./test.sh

clean:
	rm -f bin/sciurus
