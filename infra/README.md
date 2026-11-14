# Infraestrutura e Alta Escala — Docker Swarm (`infra/`)

Esta pasta contém a infraestrutura de containerização, observabilidade e orquestração em **Docker Swarm** da **AllSpark Fiscal**, projetada para operar com alta disponibilidade, isolamento de segredos e capacidade para **50.000+ a 500.000+ notas fiscais/dia**.

## Arquivos da Pasta `infra/`

| Arquivo | Função |
|---|---|
| [Dockerfile](Dockerfile) | Build multi-stage em Go (`golang:1.27-bookworm` -> `distroless/static-debian12:nonroot`) que compila o binário único `/allspark` para os papéis `api`, `worker`, `dispatcher` e `migrate`. |
| [docker-stack.yml](docker-stack.yml) | Definição completa da stack **Docker Swarm** (`version: "3.9"`), com redes `overlay` criptografáveis (`backend` interna, `egress` para SEFAZ e `edge` para entrada HTTP), Docker Secrets nativos, limites de CPU/RAM, `start-first` zero-downtime rolling updates e réplicas pré-dimensionadas. |
| [swarm-deploy.sh](swarm-deploy.sh) | Script automatizado que inicializa o Swarm (se inativo), provisiona os 7 segredos no cofre do Docker Swarm, constrói as imagens `allspark-fiscal-api:local` e `allspark-fiscal-engine:local` e executa `docker stack deploy`. |
| [postgres-init.sh](postgres-init.sh) | Script de inicialização de roles de menor privilégio no PostgreSQL (`allspark_owner` para migrações e `allspark` para execução da aplicação). |
| [observability/prometheus.yml](observability/prometheus.yml) | Configuração de coleta de métricas Prometheus na porta `:9090` da API. |

---

## Como Subir ou Atualizar em Docker Swarm

```bash
chmod +x infra/swarm-deploy.sh
sh infra/swarm-deploy.sh
```

## Como Escalar Horizontalmente sob Demanda (Alta Demanda / Black Friday)

No Docker Swarm, todos os serviços stateless (`api`, `worker`, `dispatcher`, `engine`) escalam instantaneamente sem queda de conexão:

```bash
# Escalar para 16 workers, 8 motores PHP e 4 instâncias de API
docker service scale \
  allspark-fiscal_worker=16 \
  allspark-fiscal_engine=8 \
  allspark-fiscal_api=4 \
  allspark-fiscal_dispatcher=3
```

### Dimensionamento Recomendado por Volume Diário

| Volume Diário | Pico Comercial | `api` (réplicas) | `dispatcher` | `worker` (réplicas) | `engine` (réplicas) |
|---|---|---|---|---|---|
| Até **10.000 notas/dia** | ~5 notas/seg | 2 | 1 | 4 | 2 |
| **50.000 notas/dia** | ~15–25 notas/seg | 3 | 2 | 8 | 4 |
| **200.000+ notas/dia** | ~60–100 notas/seg | 6 | 4 | 20 | 10 |

> **Importante para Cluster Multi-Node (Vários Servidores no Swarm):**
> Quando os serviços `api` e `worker` rodarem em máquinas físicas/VMs separadas, configure `STORAGE_DRIVER=s3` apontando para MinIO, AWS S3 ou Cloudflare R2 para que todas as réplicas leiam e gravem os XMLs/PDFs criptografados no mesmo bucket compartilhado.

