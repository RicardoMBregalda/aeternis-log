# Sistema de Gerenciamento de Logs - Arquitetura Híbrida

Sistema de gerenciamento de logs desenvolvido como trabalho de conclusão de curso (TCC), comparando arquitetura tradicional baseada em PostgreSQL com arquitetura híbrida utilizando MongoDB, Hyperledger Fabric v2.4.9 e blockchain.

## Arquitetura

### Arquitetura Tradicional
- PostgreSQL 15 em cluster primary-standby
- Replicação streaming síncrona
- Failover automático
- API REST em Go 1.18 com framework Gin

### Arquitetura Híbrida
- MongoDB v5.0.21 para armazenamento off-chain
- Hyperledger Fabric v2.4 para blockchain (consenso Raft)
- Write-Ahead Log (WAL) para durabilidade (0% perda de dados)
- Redis para cache (opcional)
- Merkle Tree para integridade criptográfica
- API REST em Go 1.18 com framework Gin
- Chaincode em Go para registro de hashes no ledger

## Requisitos

- Docker 24.x
- Docker Compose 2.0+
- Go 1.18+ (para API de testes)
- Python 3.10+ (para análise de resultados)
- Ubuntu 22.04 LTS (recomendado, via WSL2 no Windows)
- 8 vCPUs (AMD Ryzen 7 5700X3D ou equivalente)
- 16GB RAM DDR4
- 100GB SSD NVMe

## Instalação

### 1. Clonar Repositório

```bash
git clone <repository-url>
cd tcc-log-management
```

### 2. Compilar API Go

```bash
cd testing/go-api
make build
```

Ou usando Docker:

```bash
cd testing/go-api
docker-compose up -d
```

### 3. Instalar Dependências Python (para análise de resultados)

```bash
cd testing
pip install -r requirements.txt
```

### 4. Configurar Variáveis de Ambiente

As configurações estão em `testing/go-api/config.yaml`. Exemplo:

```yaml
server:
  port: 5001
  mode: release

mongodb:
  uri: "mongodb://localhost:27017"
  database: "logdb"

postgres:
  host: "localhost"
  port: 5432
  database: "logdb"

fabric:
  peer_endpoint: "localhost:7051"
  channel: "logchannel"
```

## Uso

### Arquitetura Tradicional (PostgreSQL)

#### Iniciar

```bash
cd traditional-architecture
./start-traditional.sh
```

O script irá:
- Iniciar containers PostgreSQL primary e standby
- Configurar replicação streaming
- Criar banco de dados e tabelas

#### Testar

```bash
./test-traditional.sh
```

#### Parar

```bash
./stop-traditional.sh
```

### Arquitetura Híbrida (MongoDB + Fabric)

#### Iniciar

```bash
cd hybrid-architecture
./fabric-network/start-network.sh
```

O script irá:
- Gerar artefatos criptográficos
- Iniciar rede Hyperledger Fabric
- Criar canal e instalar chaincode
- Iniciar MongoDB e Redis

#### Iniciar API Go

```bash
cd testing/go-api
make run
```

Ou usando o binário compilado:

```bash
cd testing/go-api
./bin/log-api
```

A API estará disponível em `http://localhost:5001`

#### Testar

```bash
cd hybrid-architecture
./fabric-network/test-network.sh
```

#### Parar

```bash
cd hybrid-architecture
./fabric-network/stop-network.sh
```

## Testes de Performance

### Executar Todos os Cenários (S1-S9)

```bash
cd testing
./run_all_tests.sh
```

### Executar Cenário Específico

```bash
cd testing
python src/performance_tester.py --scenario S5 --architecture hybrid --api-url http://localhost:5001
```

### Cenários Disponíveis

| Cenário | Volume | Taxa (logs/s) | Descrição |
|---------|--------|---------------|-----------|
| S1 | 10.000 | 100 | Baixo Volume + Baixa Taxa |
| S2 | 10.000 | 1.000 | Baixo Volume + Média Taxa |
| S3 | 10.000 | 10.000 | Baixo Volume + Alta Taxa |
| S4 | 100.000 | 100 | Médio Volume + Baixa Taxa |
| S5 | 100.000 | 1.000 | Médio Volume + Média Taxa |
| S6 | 100.000 | 10.000 | Médio Volume + Alta Taxa |
| S7 | 1.000.000 | 100 | Alto Volume + Baixa Taxa |
| S8 | 1.000.000 | 1.000 | Alto Volume + Média Taxa |
| S9 | 1.000.000 | 10.000 | Alto Volume + Alta Taxa |

### Análise de Resultados

```bash
cd testing/src
python analyze_results.py
```

Os resultados serão salvos em:
- `testing/src/results/` (JSON e CSV)
- `testing/results/` (relatórios consolidados)

## API REST (Go)

### Documentação Swagger

Após iniciar a API, acesse a documentação interativa:

**http://localhost:5001/swagger/index.html**

### Endpoints Principais

#### Logs
- `POST /api/v1/logs` - Criar novo log
- `POST /api/v1/logs/batch` - Criar múltiplos logs
- `GET /api/v1/logs/:id` - Buscar log específico
- `GET /api/v1/logs` - Listar logs com filtros (source, severity, timestamp)

#### Merkle Tree & Blockchain
- `POST /api/v1/merkle/batch` - Criar batch com Merkle Tree
- `GET /api/v1/merkle/batch/:id` - Consultar batch
- `POST /api/v1/merkle/verify` - Verificar integridade via Merkle proof
- `POST /api/v1/blockchain/submit` - Submeter hash para blockchain

#### Sistema
- `GET /api/v1/health` - Health check
- `GET /api/v1/metrics` - Métricas Prometheus

### Exemplo de Uso

```bash
# Criar log
curl -X POST http://localhost:5001/api/v1/logs \
  -H "Content-Type: application/json" \
  -d '{
    "timestamp": "2025-11-16T10:30:00Z",
    "source": "auth-service",
    "severity": "INFO",
    "message": "User login successful",
    "metadata": {
      "userId": "12345",
      "ip": "192.168.1.100"
    }
  }'

# Listar logs com filtro
curl "http://localhost:5001/api/v1/logs?source=auth-service&severity=ERROR&limit=10"

# Verificar integridade Merkle
curl -X POST http://localhost:5001/api/v1/merkle/verify \
  -H "Content-Type: application/json" \
  -d '{
    "batchId": "batch-123",
    "logId": "log-456"
  }'
```

## Estrutura do Projeto

```
tcc-log-management/
├── hybrid-architecture/        # Arquitetura híbrida
│   ├── chaincode/             # Smart contract Fabric (Go)
│   │   └── logchaincode.go
│   └── fabric-network/        # Configurações da rede
│       ├── configtx.yaml
│       ├── docker-compose.yml
│       └── scripts/
├── traditional-architecture/   # Arquitetura PostgreSQL
│   ├── docker-compose.yml
│   └── scripts/               # Scripts SQL
├── testing/                   # Framework de testes
│   ├── go-api/                # API REST em Go
│   │   ├── cmd/api/           # Entrypoint da aplicação
│   │   ├── internal/          # Lógica de negócio
│   │   │   ├── handlers/      # HTTP handlers
│   │   │   ├── services/      # Serviços de log
│   │   │   ├── database/      # Clients MongoDB/PostgreSQL
│   │   │   ├── fabric/        # Integração Fabric SDK
│   │   │   ├── merkle/        # Merkle Tree
│   │   │   ├── wal/           # Write-Ahead Log
│   │   │   ├── models/        # Modelos de dados
│   │   │   ├── middleware/    # Middlewares HTTP
│   │   │   └── cache/         # Implementação Redis
│   │   ├── docs/              # Swagger docs
│   │   ├── Dockerfile
│   │   ├── Makefile
│   │   └── config.yaml
│   ├── src/                   # Scripts Python (análise)
│   │   ├── analyze_results.py
│   │   └── performance_tester.py
│   ├── scripts/               # Scripts auxiliares
│   ├── results/               # Resultados dos testes
│   └── config.py              # Configurações Python
└── tcc/                       # Documentação LaTeX (TCC)
    └── *.tex
```

## Otimizações Implementadas

### API Go (Ambas Arquiteturas)
- **Concorrência nativa:** Goroutines para processamento paralelo
- **Performance:** Compilação nativa, baixo consumo de CPU (0.4-30%)
- **Connection pooling:** MongoDB (max 100 conexões), PostgreSQL (max 20)
- **Framework Gin:** Router de alta performance
- **Drivers nativos:** pgx (PostgreSQL), mongo-driver (MongoDB)

### Arquitetura Híbrida
- **Write-Ahead Log (WAL):** Garantia de 0% de perda de dados
  - Syscalls `Write` + `Sync` para fsync atômico
  - Durabilidade antes de qualquer processamento
- **Merkle Tree:** Batching automático de 50 logs
- **Fabric SDK Go:** Integração nativa com Hyperledger Fabric
- **Índices MongoDB:** Compostos em timestamp, source, severity
- **Throughput:** Até 585 logs/s (cenário 1M logs)
- **Latência P50:** 3.62-246ms (varia com volume)

### Arquitetura Tradicional
- **Replicação streaming:** Síncrona para alta consistência
- **Connection pooling:** Otimizado para concorrência Go
- **Índices PostgreSQL:** B-tree em colunas críticas
- **Prepared statements:** Queries pré-compiladas
- **Throughput:** Até 977 logs/s (cenário 1M logs)
- **Latência P50:** 1.34-4.49ms
- **Trade-off:** 38.17% perda de dados em falha primária (sem WAL)

## Testes de Tolerância a Falhas

```bash
cd testing
./run_fault_tolerance.sh
```

Cenários testados:
- **Falha primário PostgreSQL:** 38.17% perda de dados (sem WAL)
- **Falha rede Fabric:** 0% perda de dados (WAL protege)
- **Falha MongoDB:** 0% perda de dados (WAL + replicação)
- **Recuperação:** Tempo de failover < 10s

### Resultados Comparativos

| Métrica | PostgreSQL | Híbrida (MongoDB + Fabric) |
|---------|------------|----------------------------|
| **Throughput máx** | 977 logs/s | 585 logs/s |
| **Latência P50** | 1.34-4.49ms | 3.62-246ms |
| **CPU médio** | 0.4-0.83% | 0.57-30.37% |
| **RAM médio** | 122-138 MB | 77-121 MB |
| **Perda de dados** | 38.17% | 0% |
| **Imutabilidade** | ❌ | ✅ (blockchain) |

## Tecnologias Utilizadas

### Backend
- **Go 1.18:** Linguagem principal da API
- **Gin Framework:** Router HTTP de alta performance
- **fabric-sdk-go v1.0.0:** SDK oficial Hyperledger Fabric
- **pgx v5.4:** Driver PostgreSQL nativo
- **mongo-driver v1.13.4:** Driver MongoDB oficial
- **gopsutil v3.23:** Monitoramento de recursos

### Blockchain & Databases
- **Hyperledger Fabric v2.4:** Blockchain permissionada
- **MongoDB v5.0.21:** Banco NoSQL para logs
- **PostgreSQL v15:** Banco relacional tradicional
- **CouchDB:** State database do Fabric

### DevOps & Análise
- **Docker 24.x:** Containerização
- **Python 3.10:** Scripts de análise
- **Pandas v2.0:** Processamento de dados
- **Matplotlib v3.7:** Visualização de gráficos

## Licença

Este projeto está licenciado sob a Licença Apache 2.0 - veja o arquivo [LICENSE](LICENSE) para detalhes.
