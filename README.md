# Allsparkfiscal

**API Fiscal Nacional Multi-Tenant de Alta Escala (50.000+ Notas/Dia)** para emissão, simulação, validação XSD, assinatura ICP-Brasil A1, DANFE/DANFCE PDF, eventos SEFAZ, Webhooks assinados (`HMAC-SHA256`) e **Servidor MCP Nativo (`/v1/mcp`)** compatível com **Claude, ChatGPT, Gemini, Cursor e Windsurf**.

Construída em **Go 1.27 + PHP 8.3 (`nfephp-org/sped-nfe v5.2.11` + `sped-da`) + PostgreSQL 17 + RabbitMQ 4.1 (Quorum Queues) + Redis 7.4**, pronta para **Docker Compose** e **Docker Swarm + GitHub Container Registry (`ghcr.io`)**.

---

## 1. Tipos de Notas e Operações Emitidas (27 UFs do Brasil)

| Modelo / Operação | Código | Descrição | Status |
|---|---|---|---|
| **NF-e** | **Modelo 55** (`kind: "nfe"`) | Nota Fiscal Eletrônica de Produtos e Mercadorias (B2B, B2C, E-commerce, Indústria, Atacado) | **100% Pronto** |
| **NFC-e** | **Modelo 65** (`kind: "nfce"`) | Nota Fiscal de Consumidor Eletrônica (Varejo Presencial, PDV, Restaurantes, Supermercados com QR Code) | **100% Pronto** |
| **Reforma Tributária 2026** | `PL_010_V1.30` | Suporte completo às tags `<IBSCBS>`, `<IBSCBSTot>`, `<IS>` e `<ISTot>` (IBS Estadual, IBS Municipal, CBS Federal e Imposto Seletivo) | **100% Pronto** |
| **DIFAL Interestadual** | `<ICMSUFDest>` | Partilha de ICMS e Fundo de Combate à Pobreza (`FCP`) para consumidor final em outra UF (`CFOP 6108`) | **100% Pronto** |
| **Finalidades NF-e** | `purpose: 1..4` | `1`: Normal, `2`: Complementar, `3`: Ajuste, `4`: Devolução de Mercadoria (`referenced_keys`) | **100% Pronto** |
| **Eventos Fiscais SEFAZ** | Eventos Oficiais | Cancelamento (`110111`), Carta de Correção — CC-e (`110110`), Inutilização (`sefazInutiliza`), Manifestação do Destinatário — MDe (`210200`..`210240`) e Distribuição DF-e (`sefazDistDFe`) | **100% Pronto** |

---

## 2. Como Começar Rapidamente (Docker Compose Local)

```sh
# 1. Inicializar segredos locais (.env e var/secrets/)
python3 scripts/init-local.py

# 2. Subir a stack com 4 réplicas de worker (8 goroutines concorrentes cada = 32 workers paralelos)
docker compose up -d --build --scale worker=4

# 3. Verificar saúde
curl http://127.0.0.1:8088/healthz
curl http://127.0.0.1:8088/readyz

# 4. Criar um Tenant e gerar sua API Key (Bearer af_...)
docker compose run --rm --no-deps -e ROLE=tenant-create -e TENANT_NAME='Minha Empresa' api
```

---

## 3. Fluxo de Emissão Simplificada (Em 3 Passos)

### Passo 1 — Cadastrar Empresa, Série e Certificado Digital A1
1. `POST /v1/companies` (com CNPJ, IE, CRT, endereço IBGE e opcionalmente `logo_base64` para o DANFE PDF e `contingency_mode: "auto"`).
2. `PUT /v1/companies/{id}/fiscal/nfe` (`{"environment": "homologation", "series": 1, "enabled": true}`).
3. `POST /v1/companies/{id}/certificates` (`{"pfx_base64": "...", "password": "..."}`).
4. `GET /v1/companies/{id}/readiness` -> Confirma `ready_for_nfe: true`.

### Passo 2 — Emitir Nota pelo Pedido (`POST /v1/orders/invoice`) com `ref` e `webhook_url`
```bash
curl -X POST http://127.0.0.1:8088/v1/orders/invoice \
  -H "Authorization: Bearer af_sua_chave" \
  -H "Content-Type: application/json" \
  -d '{
    "company_id": "UUID_DA_EMPRESA",
    "ref": "PEDIDO-1001",
    "operation": "issue",
    "webhook_url": "https://meusistema.com.br/webhooks/nfe",
    "webhook_secret": "meu-segredo-hmac",
    "customer": {
      "tax_id": "11144477735",
      "name": "CLIENTE EXEMPLO",
      "ie_indicator": 9,
      "address": {
        "street": "Av Tancredo Neves",
        "number": "500",
        "district": "Caminho das Arvores",
        "city_code": "2927408",
        "city": "Salvador",
        "state": "BA",
        "postal_code": "41820021"
      }
    },
    "items": [
      {
        "sku": "PROD-01",
        "description": "NOTEBOOK PRO 16GB",
        "ncm": "84713012",
        "quantity": "1.0000",
        "price": "4500.00"
      }
    ]
  }'
```

### Passo 3 — Receber o Webhook Assinado ou Consultar por `ref`
- **Status e Chave de 44 dígitos**: `GET /v1/documents/ref/PEDIDO-1001`
- **Download do XML**: `GET /v1/documents/ref/PEDIDO-1001/xml`
- **Download do DANFE PDF**: `GET /v1/documents/ref/PEDIDO-1001/pdf`

---

## 4. Como Funciona a Verificação do Webhook e Proteção Anti-Bloqueio

Sempre que uma nota muda de estado (`authorized`, `validated`, `rejected`, `canceled`, `failed`), a AllSpark Fiscal envia um `POST` para o `webhook_url` da nota e para os webhooks cadastrados em `/v1/webhooks`:

- **Cabeçalhos enviados**:
  - `X-AllSpark-Event`: ex. `document.authorized`
  - `X-AllSpark-Delivery`: UUID único da entrega
  - `X-AllSpark-Timestamp`: Timestamp Unix da requisição
  - `X-AllSpark-Signature` e `X-AllSpark-Signature-256`: `sha256=<hmac_sha256(webhook_secret, raw_body)>`
- **Proteção contra Bloqueios em Rajadas de Notas**:
  - **Semáforo de Concorrência de Webhooks**: limita chamadas HTTP simultâneas de saída para nunca derrubar ou acionar bloqueios de WAF/Cloudflare no servidor do cliente.
  - **Retentativa Inteligente com Backoff Exponencial e `Retry-After`**: se o servidor do cliente responder `429`, `502`, `503` ou `504`, realiza até 3 tentativas com recuo exponencial automático.
  - **Escudo Redis Anti-656 SEFAZ**: controla conexões mTLS simultâneas por CNPJ (`máx. 5` simultâneas e `20/s` por CNPJ), pausa preventivamente se detectar sequência de rejeições antes do limite de 30 da NT 2018.002 e aplica disjuntor automático de 62 minutos caso a SEFAZ retorne `cStat 656`.

### Exemplo de Verificação de Assinatura do Webhook (Node.js / Python / Go)
- **Em Go**: use `store.VerifyWebhookSignature(secret, r.Header.Get("X-AllSpark-Signature-256"), rawBody)`
- **Em Python**:
  ```python
  import hmac, hashlib
  expected = "sha256=" + hmac.new(secret.encode(), raw_body, hashlib.sha256).hexdigest()
  assert hmac.compare_digest(expected, request.headers.get("X-AllSpark-Signature-256", ""))
  ```

---

## 5. Produção: Deploy no Docker Swarm, GHCR, Traefik e Portainer

O AllSpark Fiscal já está em produção no cluster Docker Swarm sob o domínio oficial com SSL automático via Let's Encrypt:
- **Endpoint Oficial**: `https://allsparkfiscal.allsparkhub.com`
- **Healthcheck Público**: `https://allsparkfiscal.allsparkhub.com/healthz`
- **Servidor MCP Nativo**: `https://allsparkfiscal.allsparkhub.com/v1/mcp`
- **Painel Portainer**: `https://portainer.allsparkhub.com` (Stack `allsparkfiscal`)

### Comandos de Deploy e Atualização Contínua (Zero Downtime)
```sh
# Deploy / Atualização direta na VPS a partir das imagens mais recentes do GHCR:
make deploy-vps
# ou:
sh infra/swarm-deploy.sh --vps

# Na VPS:
docker stack services allsparkfiscal
docker service scale allsparkfiscal_worker=8 allsparkfiscal_engine=4 allsparkfiscal_api=4
docker service logs -f allsparkfiscal_api
```

---

## 6. Documentação Completa e Coleções

| Documento / Arquivo | Descrição |
|---|---|
| [docs/MANUAL_PASSO_A_PASSO_INTEGRACAO_E_TIPOS_DE_NOTAS.md](docs/MANUAL_PASSO_A_PASSO_INTEGRACAO_E_TIPOS_DE_NOTAS.md) | **Manual passo a passo do zero à 1ª nota, todos os tipos de notas e como integrar em E-commerce, PDV, ERP e SaaS** |
| [docs/GUIA_MCP_CLAUDE_CHATGPT_GEMINI.md](docs/GUIA_MCP_CLAUDE_CHATGPT_GEMINI.md) | **Guia completo de configuração do Servidor MCP (`/v1/mcp` + STDIO Bridge) para Claude, ChatGPT, Gemini, Cursor e Windsurf** |
| [docs/GUIA_COMPLETO_API_E_ERROS.md](docs/GUIA_COMPLETO_API_E_ERROS.md) | **Referência de todos os endpoints, Webhooks e Catálogo de Erros Acionáveis ("Qual foi o erro" e "O que fazer")** |
| [docs/ANALISE_CONCORRENTES_ESCALABILIDADE_SWARM.md](docs/ANALISE_CONCORRENTES_ESCALABILIDADE_SWARM.md) | **Benchmark de mercado, limites SEFAZ (NT 2018.002 / cStat 656) e arquitetura para 50.000+ notas/dia** |
| [openapi/allspark-fiscal.postman_collection.json](openapi/allspark-fiscal.postman_collection.json) | **Coleção Postman v2.1 completa com 8 pastas, exemplos de todas as notas e respostas salvas** |
| [openapi/allspark-ai-actions.openapi.json](openapi/allspark-ai-actions.openapi.json) | **Especificação OpenAPI 3.1 para ChatGPT Custom GPTs (Actions) e Google Gemini Extensions** |
| [infra/README.md](infra/README.md) | **Guia de infraestrutura Docker Swarm, segredos e auto-scaling** |

---

## 7. Suíte de Testes e Verificação

```sh
go test -race ./... && go vet ./...
sh scripts/test-engine.sh
python3 scripts/test-company-api.py
python3 scripts/test-scalable-features.py
```
