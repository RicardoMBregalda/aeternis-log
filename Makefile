# ============================================================================
# tcc-log-management — orquestração
# Sobe a blockchain (Hyperledger Fabric) e a API (Go + MongoDB + Redis) juntas.
# A rede Fabric e a API compartilham a rede Docker $(NETWORK).
# ============================================================================

SHELL := /bin/bash

NETWORK    ?= tcc_log_network
FABRIC_DIR := hybrid-architecture/fabric-network
API_DIR    := api

.DEFAULT_GOAL := help

# ----------------------------------------------------------------------------
# Geral
# ----------------------------------------------------------------------------

.PHONY: help
help: ## Mostra esta ajuda
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

.PHONY: up
up: blockchain api ## Sobe TUDO na ordem certa: blockchain depois API
	@echo ""
	@echo "✅ Stack no ar."
	@echo "   API:     http://localhost:5001"
	@echo "   Health:  http://localhost:5001/health"
	@echo "   Swagger: http://localhost:5001/swagger/index.html"

.PHONY: down
down: ## Para TUDO (API + blockchain), mantém os dados
	-cd $(API_DIR) && docker compose down
	-cd $(FABRIC_DIR) && docker-compose down

.PHONY: restart
restart: down up ## Reinicia tudo

.PHONY: status
status: ## Lista os containers ligados à rede do projeto
	@docker ps --filter "network=$(NETWORK)" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"

# ----------------------------------------------------------------------------
# Componentes individuais
# ----------------------------------------------------------------------------

.PHONY: network
network: ## Cria a rede Docker compartilhada (idempotente)
	@docker network inspect $(NETWORK) >/dev/null 2>&1 \
		|| { echo "🌐 Criando rede $(NETWORK)..."; docker network create $(NETWORK); }

.PHONY: blockchain
blockchain: network ## Sobe só a rede Hyperledger Fabric (gera cripto, canal, chaincode)
	@echo "🔗 Subindo a rede Fabric..."
	@cd $(FABRIC_DIR) && yes n | ./start-network.sh

.PHONY: api
api: network ## Sobe só a API (build + MongoDB + Redis + API)
	@echo "🚀 Subindo a API..."
	@cd $(API_DIR) && docker compose up -d --build

.PHONY: dev
dev: network ## Sobe só MongoDB + Redis (para rodar a API nativamente / testes)
	@cd $(API_DIR) && docker compose up -d mongodb redis
	@echo "MongoDB e Redis no ar. Rode a API com: make run"

.PHONY: api-logs
api-logs: ## Segue os logs da API
	@cd $(API_DIR) && docker compose logs -f go-api

.PHONY: blockchain-logs
blockchain-logs: ## Segue os logs do CLI do Fabric (setup de canal/chaincode)
	@docker logs -f cli

# ----------------------------------------------------------------------------
# Desenvolvimento nativo (sem Docker para a API)
# ----------------------------------------------------------------------------

.PHONY: build
build: ## Compila a API nativamente
	@cd $(API_DIR) && go build ./...

.PHONY: run
run: ## Roda a API nativamente (precisa de MongoDB no ar — ver `make dev`)
	@cd $(API_DIR) && go run ./cmd/api

.PHONY: test
test: ## Roda os testes Go da API
	@cd $(API_DIR) && go test ./...

.PHONY: vet
vet: ## Roda go vet na API
	@cd $(API_DIR) && go vet ./...

# ----------------------------------------------------------------------------
# Limpeza
# ----------------------------------------------------------------------------

.PHONY: clean
clean: ## Para tudo e REMOVE os volumes (apaga dados do Mongo/Redis/WAL)
	-cd $(API_DIR) && docker compose down -v
	-cd $(FABRIC_DIR) && docker-compose down -v
	-docker network rm $(NETWORK) 2>/dev/null || true

.PHONY: clean-blockchain
clean-blockchain: ## Reset completo da rede Fabric (recria cripto e artefatos)
	@cd $(FABRIC_DIR) && yes n | ./start-network.sh --clean
