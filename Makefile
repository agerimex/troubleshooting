VERSION=1

# The regression fixture's isolated stack (docker-compose.test.yml). Its
# targets never touch the development stack or its data.
TEST_STACK=docker compose -p troubleshooting-test -f docker-compose.test.yml
FIXTURE=cd fixtures/regression && go run .
FIXTURE_ADDRS=-api http://127.0.0.1:28094 -otlp 127.0.0.1:24317 -sender 127.0.0.1:20055

.PHONY: all images network up down test test-go test-ui \
	test-stack-up test-stack-down test-stack-reset fixture-load fixture-check ui-e2e save upload

# Builds the images and creates the shared network. It does not start
# anything: run "make up" (or docker compose up -d) afterwards.
all: images network

# Go images build from the repository root so they use the local log-sender and protos.
images:
	docker build --file log-analysis/Dockerfile --tag docker-log-analysis:${VERSION} .
	cd ./log-ui && docker build --tag docker-log-ui:${VERSION} .
	docker build --file log-receiver/Dockerfile --tag docker-log-receiver:${VERSION} .

# Safe to repeat: creates the network only if it is missing.
network:
	docker network inspect troubleshooting_network >/dev/null 2>&1 || docker network create troubleshooting_network

up: network
	docker compose up -d --wait

down:
	docker compose stop

# Unit tests of every module; no database needed.
test: test-go test-ui

test-go:
	cd protos && go test ./...
	cd log-sender && go test ./...
	cd log-receiver && go test ./...
	cd log-analysis && go test ./...
	cd fixtures/regression && go vet ./...

test-ui:
	cd log-ui && npm ci && npm run type-check && npm run test:unit

test-stack-up:
	$(TEST_STACK) up -d --wait

test-stack-down:
	$(TEST_STACK) stop

# Deletes the test stack's containers and its own volume only.
test-stack-reset:
	$(TEST_STACK) down -v

# Sends a fresh fixture run (new base time and ids) to the test stack.
fixture-load:
	$(FIXTURE) load $(FIXTURE_ADDRS)

# Checks the loaded run, replays it, interrupts and recreates the test
# ClickHouse, and writes fixtures/regression/results/report.md.
fixture-check:
	$(FIXTURE) check $(FIXTURE_ADDRS) -limited-api http://127.0.0.1:28096 -replay -outage -recreate -retention

# Browser checks of the UI against a mocked API (no stack needed).
ui-e2e:
	cd log-ui && npx playwright install chromium && npm run test:e2e

save:
	docker save -o docker-log-analysis-${VERSION}.tar docker-log-analysis:${VERSION}
	docker save -o docker-log-ui-${VERSION}.tar docker-log-ui:${VERSION}
	docker save -o docker-log-receiver-${VERSION}.tar docker-log-receiver:${VERSION}

upload:
	#scp docker-log-analysis-${VERSION}.tar ${SERVER}:${SERVER_PATH}
	#scp docker-log-ui-${VERSION}.tar ${SERVER}:${SERVER_PATH}
	scp docker-log-receiver-${VERSION}.tar ${SERVER}:${SERVER_PATH}
