# AllSpark Fiscal 🧾⚡

> **API de Emissão de NF-e e NFC-e para o ecossistema AllSpark Hub**  
> Plataforma SaaS multi-tenant para emissão de Notas Fiscais Eletrônicas (NF-e modelo 55) com stack Go + PHP NFePHP + Docker Swarm.

[![Production](https://img.shields.io/badge/produção-allsparkfiscal.allsparkhub.com-green)](https://allsparkfiscal.allsparkhub.com/healthz)
[![Stack](https://img.shields.io/badge/stack-Docker%20Swarm-blue)](infra/docker-stack.yml)
[![Tests](https://img.shields.io/badge/testes%20E2E-11%2F11%20PASS-brightgreen)](scripts/test-live-prod.py)

---

## 🏗️ Arquitetura

```
┌─────────────────────────────────────────────────────────────┐
│                   Docker Swarm (VPS AllSpark)               │
│                                                             │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌───────────┐  │
│  │  API Go  │  │ Worker   │  │  Engine  │  │Dispatcher │  │
│  │ (2 répl) │  │ Go(2répl)│  │PHP(2répl)│  │  (1 répl) │  │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘  └─────┬─────┘  │
│       │             │              │               │        │
│  ┌────▼─────────────▼──────┐  ┌───▼───────────────▼──┐    │
│  │   RabbitMQ (Quorum Q)   │  │   PostgreSQL 15       │    │
│  └─────────────────────────┘  └──────────────────────┘    │
│  ┌────────────┐  ┌────────────────────────────────────┐    │
│  │   Redis    │  │  Volume: /data/objects (XML + PDF) │    │
│  └────────────┘  └────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────┘
```

**Componentes:**
| Serviço | Tecnologia | Função |
|---|---|---|
| `api` | Go 1.23 | Gateway REST, autenticação, roteamento |
| `worker` | Go 1.23 | Consome fila RabbitMQ, orquestra emissão |
| `engine` | PHP 8.4 + NFePHP | Assina XML (XMLDSig), gera DANFE PDF, comunica SEFAZ |
| `dispatcher` | Go 1.23 | Despacha Webhooks (HMAC-SHA256, 3 retries) |
| `postgres` | PostgreSQL 15 | Banco de dados principal (multi-tenant) |
| `rabbitmq` | RabbitMQ 3.13 | Fila de jobs fiscais (Quorum Queue) |
| `redis` | Redis 7 | Cache de sessões e rate-limiting |
| `migrate` | Go 1.23 | Migrations automáticas (run-once) |

---

## 🚀 Endpoints Principais

Base URL: `https://allsparkfiscal.allsparkhub.com`

### Saúde
```
GET  /healthz          → {"data":{"status":"ok"}}
GET  /readyz           → {"data":{"ready":true}}
```

### Empresas
```
POST   /v1/companies                              → Cadastrar empresa emitente
GET    /v1/companies/{id}                         → Consultar empresa
POST   /v1/companies/{id}/certificates            → Enviar certificado A1 (pfx_base64 + password)
PUT    /v1/companies/{id}/fiscal/nfe              → Configurar série NF-e
PUT    /v1/companies/{id}/fiscal/nfce             → Configurar série NFC-e
POST   /v1/companies/{id}/tax-profiles            → Criar perfil tributário
GET    /v1/companies/{id}/readiness               → Verificar prontidão para emissão
```

### Emissão
```
POST   /v1/orders/invoice                         → Emitir NF-e (retorna 202 Accepted)
```

### Documentos
```
GET    /v1/documents/{id}                         → Consultar por ID
GET    /v1/documents/ref/{ref}                    → Consultar pelo seu código de pedido
GET    /v1/documents/ref/{ref}/xml                → Baixar XML assinado
GET    /v1/documents/ref/{ref}/pdf                → Baixar DANFE em PDF
POST   /v1/documents/ref/{ref}/webhook/resend     → Reenviar webhook
POST   /v1/documents/ref/{ref}/cancel             → Cancelar nota (apenas authorized)
GET    /v1/documents                              → Listar documentos (paginado)
```

### MCP (AI Agent Integration)
```
GET    /v1/mcp                                    → Info do servidor MCP
POST   /v1/mcp                                    → Chamadas JSON-RPC 2.0 ao servidor MCP
```

---

## 📦 Formato do Payload de Emissão (NF-e)

```json
{
  "company_id": "UUID da empresa emitente",
  "ref": "SEU-CODIGO-PEDIDO-001",
  "operation": "generate",
  "webhook_url": "https://seu-erp.com/webhook/fiscal",
  "webhook_secret": "chave-hmac-secreta",
  "tax_profile": "NOME_DO_PERFIL_TRIBUTARIO",
  "customer": {
    "name": "NF-E EMITIDA EM AMBIENTE DE HOMOLOGACAO - SEM VALOR FISCAL",
    "tax_id": "CPF_OU_CNPJ_DO_CLIENTE",
    "ie_indicator": 9,
    "address": {
      "street": "Rua do Cliente",
      "number": "100",
      "district": "Bairro",
      "city": "Salvador",
      "state": "BA",
      "postal_code": "40000000",
      "city_code": "2927408"
    }
  },
  "items": [
    {
      "sku": "SKU-PRODUTO",
      "description": "DESCRICAO DO PRODUTO",
      "ncm": "84713012",
      "quantity": "1.0000",
      "price": "100.00"
    }
  ],
  "payment_method": "pix"
}
```

> ⚠️ Em homologação, o campo `customer.name` **deve** ser exatamente: `"NF-E EMITIDA EM AMBIENTE DE HOMOLOGACAO - SEM VALOR FISCAL"` (exigência da SEFAZ).

---

## 🔐 Segurança

| Camada | Implementação |
|---|---|
| **Autenticação** | Bearer Token (SHA-256 no banco, nunca em plaintext) |
| **Multi-tenancy** | Isolamento por `tenant_id` em toda query SQL |
| **Certificados** | Cifrados com AES-256-GCM via Vault interno |
| **Webhooks** | Assinados com HMAC-SHA256 (`X-AllSpark-Signature`) |
| **Rede** | Zero portas expostas exceto 443 (HTTPS via Traefik) |
| **RBAC** | Escopos de permissão por recurso no servidor MCP |
| **Anti-IDOR** | Validação de `tenant_id` impede acesso cruzado entre clientes |

---

## 🧪 Testes E2E em Produção

```bash
# Instalar dependências
pip install requests

# Executar suite completa (11 testes)
cd scripts
python test-live-prod.py
```

**Resultado esperado:**
```
✅ PASS: GET /healthz
✅ PASS: GET /readyz
✅ PASS: GET /v1/mcp
✅ PASS: POST /v1/mcp (tools/list)
✅ PASS: POST /v1/companies
✅ PASS: POST /v1/companies/{id}/certificates
✅ PASS: PUT /v1/companies/{id}/fiscal/nfe
✅ PASS: GET /v1/companies/{id}/readiness
✅ PASS: POST /v1/orders/invoice
✅ PASS: GET /v1/documents/ref/{ref}
✅ PASS: GET /v1/documents/ref/{ref}/xml
─────────────────────────────
11/11 testes passaram — Sistema OPERACIONAL
```

---

## 🐳 Deploy

### Pré-requisitos
- Docker Swarm inicializado na VPS
- Acesso via `ssh vpsallspark`
- Imagens no GHCR: `ghcr.io/spoltec/allsparkfiscal-*`

### Subir / Atualizar a Stack
```bash
# No servidor via SSH
ssh vpsallspark

# Fazer pull das últimas imagens e re-deploy
docker stack deploy -c /opt/allsparkfiscal/docker-stack.yml allsparkfiscal --with-registry-auth

# Verificar serviços
docker service ls | grep allsparkfiscal

# Logs de um serviço específico
docker service logs --tail 50 allsparkfiscal_api
docker service logs --tail 50 allsparkfiscal_worker
docker service logs --tail 50 allsparkfiscal_engine
```

### Variáveis de Ambiente Críticas
| Variável | Valor | Descrição |
|---|---|---|
| `ENABLE_ISSUANCE` | `true` | Habilita pipeline de emissão |
| `ENABLE_PRODUCTION` | `false` / `true` | `false` = homologação SEFAZ; `true` = produção real |
| `ALLOW_TEST_CERTIFICATES` | `true` | Aceita certificados sem validação ICP-Brasil completa |
| `DATABASE_URL` | `postgres://...` | Conexão com PostgreSQL |
| `AMQP_URL` | `amqp://...` | Conexão com RabbitMQ |

---

## 📋 Fluxo de Status de um Documento

```
queued → processing → validated (homologação)
                   ↘  authorized (produção, com protocolo SEFAZ)
                        ↓
                      cancelled (cancelamento via SEFAZ)
```

> Em modo **homologação**: `validated` = XML assinado + DANFE gerado, **sem protocolo SEFAZ**.  
> Em modo **produção**: `authorized` = NF-e transmitida e autorizada pela SEFAZ com protocolo `nProt`.  
> Apenas documentos `authorized` com protocolo podem ser **cancelados**.

---

## 📄 Documentação Adicional

- [Guia Passo a Passo de Integração do Cliente](docs/GUIA_INTEGRACAO_CLIENTE.md)
- [Análise de Testes e Segurança](docs/TESTES_E_SEGURANCA.md)
- [Stack Docker Swarm](infra/docker-stack.yml)
- [Script de Testes E2E](scripts/test-live-prod.py)

---

## 🏢 Projeto

**AllSpark Hub** · SPOLTEC  
Repositório: [github.com/SPOLTEC/Allsparkfiscal](https://github.com/SPOLTEC/Allsparkfiscal)  
Produção: [allsparkfiscal.allsparkhub.com](https://allsparkfiscal.allsparkhub.com)
