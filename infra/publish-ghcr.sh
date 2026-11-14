#!/bin/sh
# ==============================================================================
# AllSpark Fiscal — Build e Publicação de Imagens no GitHub Container Registry (GHCR)
# Lê credenciais do arquivo .env (GITHUB_USER, GITHUB_TOKEN, GHCR_REGISTRY, IMAGE_TAG)
# ==============================================================================
set -eu

cd "$(dirname "$0")/.."

if [ -f .env ]; then
  set -a
  # shellcheck disable=SC1091
  . ./.env
  set +a
fi

GITHUB_USER="${GITHUB_USER:-SPOLTEC}"
GHCR_REGISTRY="${GHCR_REGISTRY:-ghcr.io/spoltec/allsparkfiscal}"
IMAGE_TAG="${IMAGE_TAG:-latest}"
GIT_SHA="$(git rev-parse --short HEAD 2>/dev/null || echo local)"

if [ -z "${GITHUB_TOKEN:-}" ]; then
  echo "ERRO: Variável GITHUB_TOKEN não definida no .env ou no ambiente." >&2
  exit 1
fi

echo "==> [1/5] Verificando titular do GITHUB_TOKEN e autenticando no GHCR (ghcr.io)..."
TOKEN_OWNER="$(curl -s -H "Authorization: Bearer $GITHUB_TOKEN" https://api.github.com/user | sed -n 's/.*"login":[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1 || true)"
if [ -n "$TOKEN_OWNER" ]; then
  echo "    Token autenticado na conta GitHub: @$TOKEN_OWNER"
fi
LOGIN_USER="${TOKEN_OWNER:-$GITHUB_USER}"
printf "%s" "$GITHUB_TOKEN" | docker login ghcr.io -u "$LOGIN_USER" --password-stdin

if [ "${PUSH_GIT:-true}" = "true" ] && [ -n "${GITHUB_REPO:-}" ]; then
  echo "==> [2/5] Enviando código-fonte para o GitHub ($GITHUB_REPO -> main)..."
  AUTH_B64="$(printf "x-access-token:%s" "$GITHUB_TOKEN" | base64 -w0)"
  if ! git -c "http.https://github.com/.extraheader=AUTHORIZATION: basic $AUTH_B64" push -u origin main; then
    echo "    [AVISO] Não foi possível dar push em $GITHUB_REPO com o token de @$TOKEN_OWNER." >&2
    echo "    -> Adicione @$TOKEN_OWNER como Collaborator em $GITHUB_REPO/settings/access ou use um PAT gerado na conta $GITHUB_USER." >&2
  fi
fi

API_IMG="${GHCR_REGISTRY}-api"
ENGINE_IMG="${GHCR_REGISTRY}-engine"

echo "==> [3/5] Construindo imagem da API/Worker/Dispatcher ($API_IMG:$IMAGE_TAG)..."
docker build \
  --label "org.opencontainers.image.source=https://github.com/SPOLTEC/Allsparkfiscal" \
  -t "${API_IMG}:${IMAGE_TAG}" \
  -t "${API_IMG}:${GIT_SHA}" \
  -t "allspark-fiscal-api:local" \
  -f infra/Dockerfile .

echo "==> [4/5] Construindo imagem do Motor Fiscal NFePHP v5.2.11 ($ENGINE_IMG:$IMAGE_TAG)..."
docker build \
  --label "org.opencontainers.image.source=https://github.com/SPOLTEC/Allsparkfiscal" \
  -t "${ENGINE_IMG}:${IMAGE_TAG}" \
  -t "${ENGINE_IMG}:${GIT_SHA}" \
  -t "allspark-fiscal-engine:local" \
  ./engines/nfephp

echo "==> [5/5] Enviando imagens para o GHCR ($GHCR_REGISTRY)..."
if ! docker push "${API_IMG}:${IMAGE_TAG}"; then
  echo "" >&2
  echo "ERRO DE PERMISSÃO NO GHCR: O token atual pertence a '@$TOKEN_OWNER', que ainda não tem permissão 'create_package' no namespace '$GHCR_REGISTRY'." >&2
  echo "Como resolver:" >&2
  echo "  1. Na conta SPOLTEC, adicione '@$TOKEN_OWNER' como colaborador do repositório https://github.com/SPOLTEC/Allsparkfiscal (Settings -> Collaborators), OU" >&2
  echo "  2. Gere um Classic PAT (escopos: repo, write:packages, read:packages) logado diretamente na conta SPOLTEC e atualize GITHUB_TOKEN no arquivo .env." >&2
  exit 1
fi
docker push "${API_IMG}:${GIT_SHA}"
docker push "${ENGINE_IMG}:${IMAGE_TAG}"
docker push "${ENGINE_IMG}:${GIT_SHA}"

echo ""
echo "Imagens publicadas com sucesso no GHCR:"
echo "  - ${API_IMG}:${IMAGE_TAG} (${GIT_SHA})"
echo "  - ${ENGINE_IMG}:${IMAGE_TAG} (${GIT_SHA})"

