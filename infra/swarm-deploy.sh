#!/bin/sh
# ==============================================================================
# AllSpark Fiscal — Script Completo de Build GHCR, Secrets e Deploy no Docker Swarm
# Suporta deploy local ou remoto na VPS (passando --vps)
# ==============================================================================
set -eu

cd "$(dirname "$0")/.."

if [ -f .env ]; then
  set -a
  # shellcheck disable=SC1091
  . ./.env
  set +a
fi

STACK_NAME="${STACK_NAME:-allsparkfiscal}"
GHCR_REGISTRY="${GHCR_REGISTRY:-ghcr.io/devsistem4s/allsparkfiscal}"
IMAGE_TAG="${IMAGE_TAG:-latest}"
VPS_HOST="${VPS_HOST:-vpsallspark}"
export STACK_NAME GHCR_REGISTRY IMAGE_TAG

if [ "${1:-}" = "--vps" ]; then
  echo "==> [1/3] Construindo imagens e publicando no GHCR ($GHCR_REGISTRY:$IMAGE_TAG)..."
  docker tag "ghcr.io/spoltec/allsparkfiscal-api:latest" "${GHCR_REGISTRY}-api:${IMAGE_TAG}" 2>/dev/null || true
  docker tag "ghcr.io/spoltec/allsparkfiscal-engine:latest" "${GHCR_REGISTRY}-engine:${IMAGE_TAG}" 2>/dev/null || true
  docker push "${GHCR_REGISTRY}-api:${IMAGE_TAG}"
  docker push "${GHCR_REGISTRY}-engine:${IMAGE_TAG}"

  echo "==> [2/3] Sincronizando stack.yml na VPS ($VPS_HOST)..."
  scp infra/docker-stack.yml "${VPS_HOST}:/root/allsparkfiscal-deploy/stack.yml"

  echo "==> [3/3] Executando deploy na VPS ($VPS_HOST)..."
  ssh "${VPS_HOST}" "sh /root/allsparkfiscal-deploy/deploy.sh"
  exit 0
fi

echo "==> [1/4] Verificando estado do Docker Swarm local..."
if [ "$(docker info --format '{{.Swarm.LocalNodeState}}' 2>/dev/null || echo inactive)" != "active" ]; then
  echo "Inicializando Docker Swarm no nó local..."
  docker swarm init >/dev/null 2>&1 || docker swarm init --advertise-addr 127.0.0.1
fi

echo "==> [2/4] Garantindo segredos locais e registrando no Docker Swarm..."
sync_secret() {
  secret_name="$1"
  secret_file="$2"
  if [ ! -f "$secret_file" ]; then
    echo "ERRO: Arquivo de segredo ausente: $secret_file" >&2
    exit 1
  fi
  if ! docker secret inspect "$secret_name" >/dev/null 2>&1; then
    docker secret create "$secret_name" "$secret_file" >/dev/null
    echo "  + Segredo criado no Swarm: $secret_name"
  else
    echo "  = Segredo já existe no Swarm: $secret_name"
  fi
}

sync_secret "allspark_db_password"        "var/secrets/db_password"
sync_secret "allspark_db_admin_password"  "var/secrets/db_admin_password"
sync_secret "allspark_rabbit_password"    "var/secrets/rabbit_password"
sync_secret "allspark_redis_password"     "var/secrets/redis_password"
sync_secret "allspark_engine_token"       "var/secrets/engine_token"
sync_secret "allspark_master_key"         "var/secrets/master_key"
sync_secret "allspark_icp_ca"             "var/secrets/icp_ca"

if ! docker config inspect allspark_postgres_init >/dev/null 2>&1; then
  docker config create allspark_postgres_init infra/postgres-init.sh >/dev/null 2>&1 || true
fi

echo "==> [3/4] Construindo e publicando imagens no GHCR ($GHCR_REGISTRY:$IMAGE_TAG)..."
if [ "${1:-}" = "--no-push" ]; then
  docker build -t "${GHCR_REGISTRY}-api:${IMAGE_TAG}" -f infra/Dockerfile .
  docker build -t "${GHCR_REGISTRY}-engine:${IMAGE_TAG}" ./engines/nfephp
else
  sh infra/publish-ghcr.sh
fi

echo "==> [4/4] Publicando stack '$STACK_NAME' no Docker Swarm..."
docker stack deploy --with-registry-auth --resolve-image always -c infra/docker-stack.yml "$STACK_NAME"

echo ""
echo "Stack '$STACK_NAME' implantada com sucesso!"
echo "Comandos úteis:"
echo "  docker stack services $STACK_NAME"
echo "  docker service scale ${STACK_NAME}_worker=12 ${STACK_NAME}_engine=6 ${STACK_NAME}_api=4"
echo "  docker service logs -f ${STACK_NAME}_api"
