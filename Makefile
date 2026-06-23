# ============================================================================
# aeternis-log — orchestration
# Brings up the blockchain (Hyperledger Fabric) and the API (Go + MongoDB + Redis) together.
# The Fabric network and the API share the Docker network $(NETWORK).
# ============================================================================

SHELL := /bin/bash

NETWORK    ?= aeternislog_network
FABRIC_DIR := hybrid-architecture/fabric-network
API_DIR    := api

.DEFAULT_GOAL := help

# ----------------------------------------------------------------------------
# General
# ----------------------------------------------------------------------------

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

.PHONY: up
up: blockchain api ## Bring up EVERYTHING in the right order: blockchain then API
	@echo ""
	@echo "Stack is up."
	@echo "   API:       http://localhost:5001"
	@echo "   Health:    http://localhost:5001/health"
	@echo "   Swagger:   http://localhost:5001/swagger/index.html"
	@echo "   Dashboard: http://localhost:8088"

.PHONY: down
down: ## Stop EVERYTHING (API + blockchain), keep the data
	-cd $(API_DIR) && docker compose down
	-cd $(FABRIC_DIR) && docker-compose down

.PHONY: restart
restart: down up ## Restart everything

.PHONY: status
status: ## List the containers attached to the project network
	@docker ps --filter "network=$(NETWORK)" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"

# ----------------------------------------------------------------------------
# Individual components
# ----------------------------------------------------------------------------

.PHONY: network
network: ## Create the shared Docker network (idempotent)
	@docker network inspect $(NETWORK) >/dev/null 2>&1 \
		|| { echo "Creating network $(NETWORK)..."; docker network create $(NETWORK); }

.PHONY: blockchain
blockchain: network ## Bring up only the Hyperledger Fabric network (generates crypto, channel, chaincode)
	@echo "Bringing up the Fabric network..."
	@cd $(FABRIC_DIR) && yes n | ./start-network.sh

.PHONY: api
api: network ## Bring up the API stack (build + MongoDB + Redis + API + dashboard)
	@echo "Bringing up the API..."
	@cd $(API_DIR) && docker compose up -d --build

.PHONY: dashboard
dashboard: network ## Bring up only the integrity dashboard (static, nginx on :8088)
	@cd $(API_DIR) && docker compose up -d dashboard
	@echo "Dashboard is up: http://localhost:8088"

.PHONY: dev
dev: network ## Bring up only MongoDB + Redis (to run the API natively / for tests)
	@cd $(API_DIR) && docker compose up -d mongodb redis
	@echo "MongoDB and Redis are up. Run the API with: make run"

.PHONY: api-logs
api-logs: ## Follow the API logs
	@cd $(API_DIR) && docker compose logs -f go-api

.PHONY: dashboard-logs
dashboard-logs: ## Follow the dashboard (nginx) logs
	@cd $(API_DIR) && docker compose logs -f dashboard

.PHONY: blockchain-logs
blockchain-logs: ## Follow the Fabric CLI logs (channel/chaincode setup)
	@docker logs -f cli

# ----------------------------------------------------------------------------
# Native development (no Docker for the API)
# ----------------------------------------------------------------------------

.PHONY: build
build: ## Build the API natively
	@cd $(API_DIR) && go build ./...

.PHONY: run
run: ## Run the API natively (requires MongoDB up — see `make dev`)
	@cd $(API_DIR) && go run ./cmd/api

.PHONY: test
test: ## Run the API Go tests
	@cd $(API_DIR) && go test ./...

.PHONY: vet
vet: ## Run go vet on the API
	@cd $(API_DIR) && go vet ./...

.PHONY: smoke
smoke: ## Run the black-box end-to-end test against the running dev API (no auth)
	@KEY= KEY2= EXPECT_CHANNEL=logchannel BASE=http://localhost:5001 \
		METRICS=http://localhost:9090/metrics MONGO=aeternislog-mongodb \
		bash $(API_DIR)/scripts/e2e-test.sh

# ----------------------------------------------------------------------------
# Cleanup
# ----------------------------------------------------------------------------

.PHONY: clean
clean: ## Stop everything and REMOVE the volumes (deletes Mongo/Redis/WAL data)
	-cd $(API_DIR) && docker compose down -v
	-cd $(FABRIC_DIR) && docker-compose down -v
	-docker network rm $(NETWORK) 2>/dev/null || true

.PHONY: clean-blockchain
clean-blockchain: ## Full reset of the Fabric network (recreates crypto and artifacts)
	@cd $(FABRIC_DIR) && yes n | ./start-network.sh --clean
