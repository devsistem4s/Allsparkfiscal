# Referência recuperada da conversa

Origem: Criar API de nota fiscal, 6ab9017d-7784-83e9-aa6f-e5cb726c58ee.
A ferramenta limita cada mensagem a 20.000 caracteres. Este registro é parcial, truncado na seção 45; a documentação consolidada em docs/README.md é a especificação de trabalho. O conteúdo abaixo é referência histórica, não prova de funcionalidades implementadas.

Perfeito. Vou transformar tudo que definimos em um **prompt mestre de implementação**, para você entregar a um agente de desenvolvimento e ele entender arquitetura, endpoints, segurança, filas, Docker, banco, motores fiscais e critérios de qualidade sem depender desta conversa.

:::writing{variant="document" id="71463" title="Prompt Mestre — Implementação do AllSpark Fiscal"}
# PROMPT MESTRE — ALLSPARK FISCAL

Você é responsável por arquitetar e implementar o **AllSpark Fiscal**, uma plataforma/API fiscal brasileira multi-tenant, segura, assíncrona e horizontalmente escalável.

O projeto será desenvolvido em:

`/home/silane/Documentos/sistema/AllSpark Fiscal`

Antes de implementar qualquer componente, leia este documento inteiro.

Não simplifique requisitos críticos de segurança, consistência fiscal, multi-tenancy ou processamento assíncrono.

---

# 1. OBJETIVO

Construir uma API fiscal capaz de centralizar:

- NF-e modelo 55;
- NFC-e modelo 65;
- NFS-e;
- futuramente CT-e;
- MDF-e;
- NFCom;
- eventos fiscais;
- distribuição DF-e.

A plataforma deverá:

- receber documentos via REST API;
- validar dados;
- gerar XML;
- validar XSD;
- assinar digitalmente;
- transmitir aos autorizadores;
- interpretar respostas;
- controlar retries;
- armazenar XML autorizado;
- gerar DANFE/DANFSe em PDF;
- armazenar documentos;
- disponibilizar consulta;
- realizar cancelamentos/eventos;
- enviar webhooks;
- manter auditoria completa.

Meta inicial:

**mais de 50.000 documentos fiscais/dia.**

Essa quantidade NÃO deve ser tratada como limite arquitetural.

Projetar para escala horizontal e bursts significativamente maiores que a média diária.

---

# 2. PRINCÍPIO FUNDAMENTAL

O cliente da API deverá precisar fazer basicamente:

```text
autenticar
    ↓
selecionar empresa
    ↓
enviar documento
    ↓
receber ID
    ↓
acompanhar processamento
    ↓
receber webhook
    ↓
obter XML/PDF
```

Toda complexidade fiscal fica dentro do AllSpark.

---

# 3. ARQUITETURA

Arquitetura esperada:

```text
                         INTERNET
                            │
                            ▼
                    Load Balancer/WAF
                            │
                            ▼
                       API Gateway
                            │
             ┌──────────────┴─────────────┐
             │                            │
             ▼                            ▼
       Authentication                 Fiscal API
             │                            │
             └─────────────┬──────────────┘
                           │
                           ▼
                       PostgreSQL
                           │
           ┌───────────────┼──────────────┐
           │               │              │
           ▼               ▼              ▼
         Redis          RabbitMQ        S3/MinIO
                           │
          ┌────────────────┼────────────────┐
          │                │                │
          ▼                ▼                ▼
     NF-e Workers     NFC-e Workers    NFS-e Workers
          │                │                │
          └────────────────┼────────────────┘
                           ▼
                  Fiscal Provider Layer
                           │
            ┌──────────────┼─────────────┐
            ▼              ▼             ▼
         NFePHP       NFS-e Nacional   Providers
            │              │
            ▼              ▼
          SEFAZ       ADN/Prefeituras
```

A API HTTP deve ser stateless.

---

# 4. STACK

Preferencialmente:

```text
API principal: Go

Banco:
PostgreSQL

Cache/locks:
Redis

Broker:
RabbitMQ

Object Storage:
S3 compatível
MinIO em desenvolvimento

Containers:
Docker

Desenvolvimento:
Docker Compose

Produção:
Docker/Kubernetes-ready

Observabilidade:
OpenTelemetry
Prometheus
Grafana
Loki
Tempo
```

---

# 5. MOTORES FISCAIS

NÃO implementar toda comunicação fiscal do zero.

Pesquisar e integrar projetos open source maduros.

Prioridade inicial:

```text
nfephp-org/sped-nfe
nfephp-org/sped-da
```

Avaliar também alternativas atuais antes da decisão final.

NF-e/NFC-e deverão inicialmente utilizar NFePHP por meio de um adapter.

NUNCA acoplar a API diretamente ao NFePHP.

Criar abstração:

```text
FiscalProvider
```

Conceitualmente:

```text
build()
validate()
sign()
authorize()
query()
cancel()
correct()
voidNumber()
status()
```

Estrutura:

```text
fiscal-providers/
    nfephp/
    nfse-national/
    municipal/
```

Dessa forma um motor poderá ser substituído futuramente sem alterar a API pública.

---

# 6. MULTI-TENANCY

Hierarquia:

```text
Tenant
   │
   ├── Company A
   ├── Company B
   └── Company C
```

Uma integração/ERP poderá administrar centenas ou milhares de empresas.

Toda entidade fiscal deve possuir obrigatoriamente:

```text
tenant_id
company_id
```

quando aplicável.

NUNCA confiar somente em `company_id` recebido do cliente.

Sempre validar:

```text
token
 ↓
tenant
 ↓
permissões
 ↓
company_id
 ↓
empresa pertence ao tenant?
```

Caso contrário:

```http
403 Forbidden
```

Nenhum tenant poderá acessar:

- empresas;
- certificados;
- documentos;
- XML;
- PDF;
- webhooks;
- eventos;
- logs;

de outro tenant.

Implementar testes automatizados específicos para isolamento multi-tenant.

---

# 7. AUTENTICAÇÃO

Fluxo inicial:

```text
KEY
+
USERNAME
+
PASSWORD
       ↓
POST /v1/auth/token
       ↓
ACCESS TOKEN temporário
```

Endpoint:

```http
POST /v1/auth/token
```

Request:

```json
{
  "key": "ask_live_xxxxxxxxx",
  "username": "integracao@cliente.com",
  "password": "senha"
}
```

Resposta:

```json
{
  "access_token": "...",
  "refresh_token": "...",
  "token_type": "Bearer",
  "expires_in": 900,
  "expires_at": "...",

  "tenant": {
    "id": "tnt_..."
  },

  "scopes": [
    "companies:read",
    "nfe:read",
    "nfe:write"
  ]
}
```

Access token inicialmente:

```text
15 minutos
```

Criar também:

```http
POST /v1/auth/refresh
POST /v1/auth/revoke
```

Refresh tokens deverão possuir rotação.

Nunca armazenar senha em texto puro.

Utilizar:

```text
Argon2id
```

API Keys também não deverão ser armazenadas integralmente em texto puro.

Preparar arquitetura para posteriormente suportar:

```text
client_credentials
```

para integrações machine-to-machine.

---

# 8. AUTORIZAÇÃO

Implementar RBAC + scopes.

Exemplos:

```text
companies:read
companies:write

certificates:read
certificates:write

nfe:read
nfe:write

nfce:read
nfce:write

nfse:read
nfse:write

webhooks:read
webhooks:write
```

---

# 9. HEADERS

Requisições fiscais utilizarão:

```http
Authorization: Bearer TOKEN
X-Company-ID: cmp_...
Idempotency-Key: UUID-OU-REFERENCIA
X-Request-ID: opcional
Content-Type: application/json
```

---

# 10. CERTIFICADOS DIGITAIS

Inicialmente suportar:

```text
A1
PFX
P12
```

Certificados são dados extremamente sensíveis.

NUNCA armazenar:

```text
PFX puro
senha pura
private key pura
```

no banco.

Implementar criptografia de envelope.

Preferencialmente:

```text
AES-256-GCM
+
KMS/Vault
```

Compatível futuramente com:

```text
AWS KMS
Google Cloud KMS
Azure Key Vault
HashiCorp Vault
```

Banco guarda:

```text
certificate_id
tenant_id
company_id

encrypted_certificate
encrypted_password
encrypted_data_key

serial_number
issuer
subject
tax_id

valid_from
valid_until
thumbprint

created_at
updated_at
revoked_at
```

Fluxo:

```text
upload
 ↓
validação
 ↓
extração metadata
 ↓
verificação CNPJ
 ↓
criptografia
 ↓
persistência
 ↓
descarte dos dados originais
```

Uso:

```text
worker
 ↓
obtém material criptografado
 ↓
descriptografa somente em memória
 ↓
assina documento
 ↓
descarta dados sensíveis
```

Nunca registrar certificado/senha em logs.

---

# 11. EMPRESAS

Endpoints:

```text
POST   /v1/companies
GET    /v1/companies
GET    /v1/companies/{id}
PATCH  /v1/companies/{id}
DELETE /v1/companies/{id}
```

DELETE deve ser lógico quando existirem registros fiscais.

Certificado:

```text
PUT    /v1/companies/{id}/certificate
GET    /v1/companies/{id}/certificate
DELETE /v1/companies/{id}/certificate
```

Configuração:

```text
GET /v1/companies/{id}/fiscal

PUT /v1/companies/{id}/fiscal/nfe
PUT /v1/companies/{id}/fiscal/nfce
PUT /v1/companies/{id}/fiscal/nfse
```

---

# 12. EMISSÃO NF-e

Endpoint:

```http
POST /v1/nfe
```

Deve aceitar:

```text
external_id
operação
destinatário
endereço
itens
NCM
CEST
CFOP
GTIN
unidade
quantidade
valores
descontos
frete
seguro
ICMS
IPI
PIS
COFINS
FCP
DIFAL
IBS
CBS
IS
transportadora
cobrança
duplicatas
pagamentos
intermediador
referências fiscais
informações adicionais
```

O contrato deve acompanhar os layouts fiscais atuais, inclusive Reforma Tributária.

NÃO reproduzir simplesmente as tags XML na API pública.

Criar um modelo amigável e versionável.

O Fiscal Provider será responsável pela transformação:

```text
AllSpark JSON
      ↓
Internal Fiscal Model
      ↓
XML SEFAZ
```

---

# 13. PROCESSAMENTO ASSÍNCRONO

NUNCA manter a requisição HTTP aguardando toda a emissão.

Fluxo:

```text
POST /v1/nfe
      ↓
autenticação
      ↓
autorização
      ↓
validação
      ↓
idempotência
      ↓
persistência
      ↓
queue
      ↓
202 Accepted
```

Exemplo:

```json
{
  "id": "nfe_...",
  "external_id": "PED-928282",
  "type": "nfe",
  "status": "pending",
  "processing_status": "queued",
  "company_id": "cmp_...",
  "environment": "production",
  "created_at": "...",

  "links": {
    "self": "/v1/nfe/nfe_...",
    "events": "/v1/nfe/nfe_.../events"
  }
}
```

---

# 14. WORKER

Posteriormente:

```text
RabbitMQ
   ↓
Worker
   ↓
carrega empresa
   ↓
carrega certificado
   ↓
monta XML
   ↓
valida XSD
   ↓
assina
   ↓
SEFAZ
   ↓
processa resposta
   ↓
armazena XML
   ↓
gera DANFE
   ↓
armazena PDF
   ↓
atualiza documento
   ↓
gera evento
   ↓
webhook
```

---

# 15. CONSULTA NF-e

```text
GET /v1/nfe
GET /v1/nfe/{id}
GET /v1/nfe/{id}/xml
GET /v1/nfe/{id}/pdf
GET /v1/nfe/{id}/files
GET /v1/nfe/{id}/events
```

Filtros:

```text
external_id
status
company_id
access_key
created_from
created_to
```

Usar cursor pagination.

---

# 16. VALIDAÇÃO

Criar:

```http
POST /v1/nfe/validate
```

Executar:

```text
JSON validation
business validation
fiscal validation
XML generation
XSD validation
```

Não transmitir à SEFAZ.

Não consumir número fiscal.

---

# 17. EVENTOS NF-e

Implementar:

```text
POST /v1/nfe/{id}/cancel

POST /v1/nfe/{id}/correction-letter

POST /v1/nfe/void-number-range
```

Operações externas também deverão ser assíncronas quando apropriado.

---

# 18. NFC-e

Criar interface consistente:

```text
POST /v1/nfce
GET  /v1/nfce
GET  /v1/nfce/{id}

POST /v1/nfce/validate

GET /v1/nfce/{id}/xml
GET /v1/nfce/{id}/pdf
GET /v1/nfce/{id}/events

POST /v1/nfce/{id}/cancel
```

Suportar:

```text
CSC
QR Code
DANFE NFC-e
```

---

# 19. NFS-e

Criar:

```text
POST /v1/nfse
GET  /v1/nfse
GET  /v1/nfse/{id}

POST /v1/nfse/validate

GET /v1/nfse/{id}/xml
GET /v1/nfse/{id}/pdf
GET /v1/nfse/{id}/events

POST /v1/nfse/{id}/cancel
POST /v1/nfse/{id}/replace
```

A API pública não deverá expor desnecessariamente qual provider municipal está sendo utilizado.

Priorizar NFS-e padrão nacional.

Adicionar providers municipais conforme necessário.

---

# 20. DOCUMENT ENDPOINT

Também preparar modelo genérico:

```http
POST /v1/documents
GET /v1/documents/{id}
GET /v1/documents/{id}/events
```

para futuras categorias fiscais.

---

# 21. IDEMPOTÊNCIA

Obrigatória na emissão.

Header:

```http
Idempotency-Key: pedido-928282-nfe
```

Chave lógica:

```text
tenant_id
company_id
operation
idempotency_key
```

Persistir:

```text
request_hash
document_id
created_at
expires_at
```

Mesma chave + mesmo payload:

```text
retornar operação existente
```

Mesma chave + payload diferente:

```http
409 Conflict
```

Nunca emitir duas notas por retry HTTP acidental.

---

# 22. BATCH

Implementar:

```http
POST /v1/batches
GET /v1/batches/{id}
GET /v1/batches/{id}/documents
```

Exemplo de retorno:

```json
{
  "id": "batch_...",
  "status": "processing",

  "summary": {
    "total": 5000,
    "queued": 100,
    "processing": 20,
    "authorized": 4800,
    "rejected": 70,
    "failed": 10
  }
}
```

Não transformar milhares de documentos em uma única operação de worker.

Fragmentar internamente.

---

# 23. IMPORTAÇÕES GRANDES

Preparar:

```text
POST /v1/imports
GET  /v1/imports/{id}
```

Para dezenas/centenas de milhares de documentos, permitir upload para object storage e processamento assíncrono.

Não depender de JSON HTTP gigantesco.

---

# 24. RABBITMQ

Filas inicialmente:

```text
fiscal.nfe.issue
fiscal.nfce.issue
fiscal.nfse.issue

fiscal.nfe.cancel
fiscal.nfe.correction

fiscal.pdf.generate

fiscal.webhook.dispatch
```

DLQ:

```text
fiscal.nfe.issue.dlq
fiscal.nfce.issue.dlq
fiscal.nfse.issue.dlq

fiscal.webhook.dlq
```

Mensagens devem ser idempotentes.

Nunca assumir exactly-once delivery.

Projetar para:

```text
at-least-once
+
idempotent consumers
```

---

# 25. RETRIES

Diferenciar:

```text
erro permanente
```

de:

```text
erro temporário
```

Por exemplo:

```text
NCM inválido
→ NÃO retry

SEFAZ timeout
→ retry

SEFAZ indisponível
→ retry

certificado expirado
→ NÃO retry automático infinito
```

Aplicar exponential backoff + jitter.

Exemplo:

```text
5 s
30 s
2 min
10 min
30 min
```

Depois DLQ quando aplicável.

---

# 26. CIRCUIT BREAKER

Implementar por autorizador/provider.

Exemplos:

```text
SEFAZ-SP
SEFAZ-BA
SVRS
SVC
NFS-e Nacional
provider municipal
```

Estados:

```text
CLOSED
OPEN
HALF_OPEN
```

Evitar tempestades contra serviços indisponíveis.

---

# 27. BACKPRESSURE

Se chegarem:

```text
50.000 solicitações
```

rapidamente:

```text
API aceita
 ↓
persistência
 ↓
fila absorve
 ↓
workers processam conforme capacidade
```

Não disparar dezenas de milhares de conexões simultaneamente contra SEFAZ.

---

# 28. STATUS

Estados internos possíveis:

```text
created
validated
queued
processing
building_xml
signing
sending
awaiting_authorization
authorized

validation_failed
rejected

retry_scheduled
provider_unavailable

failed
dead_letter

cancellation_pending
cancelled

correction_pending
corrected
```

API pública poderá simplificar:

```text
pending
processing
authorized
rejected
cancelled
failed
```

Não expor detalhes internos instáveis desnecessariamente.

---

# 29. REJEIÇÃO FISCAL

Não tratar rejeição SEFAZ automaticamente como erro HTTP 500.

Exemplo:

```json
{
  "status": "rejected",

  "rejection": {
    "authority": "SEFAZ",
    "code": "539",
    "message": "...",
    "received_at": "..."
  }
}
```

Preservar código e mensagem originais da autoridade.

---

# 30. PADRÃO DE ERRO

Todas as APIs devem utilizar:

```json
{
  "error": {
    "type": "validation_error",
    "code": "INVALID_REQUEST",
    "message": "A requisição contém dados inválidos.",

    "details": [
      {
        "field": "items[0].ncm",
        "code": "INVALID_NCM",
        "message": "NCM inválido."
      }
    ],

    "request_id": "req_..."
  }
}
```

HTTP:

```text
200 OK
201 Created
202 Accepted

400 Bad Request
401 Unauthorized
403 Forbidden
404 Not Found
409 Conflict
422 Unprocessable Entity
429 Too Many Requests

500 Internal Server Error
503 Service Unavailable
```

---

# 31. WEBHOOKS

Endpoints:

```text
POST   /v1/webhooks
GET    /v1/webhooks
GET    /v1/webhooks/{id}
PATCH  /v1/webhooks/{id}
DELETE /v1/webhooks/{id}

POST /v1/webhooks/{id}/test
POST /v1/webhooks/{id}/rotate-secret
```

Eventos:

```text
document.created
document.processing

nfe.authorized
nfe.rejected
nfe.cancelled
nfe.corrected

nfce.authorized
nfce.rejected
nfce.cancelled

nfse.authorized
nfse.rejected
nfse.cancelled

certificate.expiring
certificate.expired
```

Payload:

```json
{
  "id": "evt_...",
  "type": "nfe.authorized",
  "created_at": "...",

  "data": {
    "document_id": "nfe_...",
    "company_id": "cmp_...",
    "external_id": "PED-123",
    "status": "authorized",
    "access_key": "..."
  }
}
```

Assinar webhook usando HMAC.

Headers:

```text
AllSpark-Event-ID
AllSpark-Timestamp
AllSpark-Signature
```

Proteger contra replay attacks usando timestamp.

---

# 32. WEBHOOK RETRY

Implementar algo equivalente a:

```text
imediato
1 min
5 min
30 min
2 h
6 h
24 h
```

Registrar todas as tentativas.

---

# 33. OBJECT STORAGE

XML/PDF devem ir para object storage.

Desenvolvimento:

```text
MinIO
```

Produção:

```text
S3 compatible
```

Exemplo:

```text
tenant/
 company/
   2026/
    09/
      nfe/
        access-key/
          request.json
          signed.xml
          authorized.xml
          danfe.pdf
          events/
```

Não tornar bucket público.

Downloads deverão ser autenticados ou utilizar signed URLs de curta duração.

O XML autorizado é o artefato fiscal principal.

PDF é derivado.

---

# 34. POSTGRESQL

Criar inicialmente:

```text
tenants

users
roles
permissions
user_roles

api_keys
refresh_tokens

companies
company_settings

certificates

documents
document_items
document_events

nfe_documents
nfce_documents
nfse_documents

fiscal_sequences

batches
batch_items

imports

idempotency_keys

webhook_endpoints
webhook_events
webhook_deliveries

fiscal_requests
fiscal_responses

audit_logs
```

Utilizar migrations.

Não permitir alteração manual do schema em produção.

---

# 35. NUMERAÇÃO

NUNCA:

```sql
SELECT MAX(number) + 1
```

Criar controle transacional:

```text
company_id
document_type
environment
series
next_number
```

Concorrência deve ser segura.

Dois workers nunca podem receber acidentalmente o mesmo número fiscal.

---

# 36. IDENTIFICADORES

Preferencialmente utilizar:

```text
UUIDv7
```

com prefixos externos amigáveis:

```text
tnt_
cmp_
usr_
key_
cert_

nfe_
nfce_
nfse_

batch_
evt_
wh_
req_
```

---

# 37. REDIS

Usar para:

```text
rate limiting
cache
distributed locks
temporary state
provider health cache
```

Não usar Redis como fonte definitiva de registros fiscais.

---

# 38. PAGINAÇÃO

Utilizar cursor pagination:

```http
GET /v1/nfe?limit=100&cursor=xxx
```

Resposta:

```json
{
  "data": [],
  "pagination": {
    "next_cursor": "...",
    "has_more": true
  }
}
```

---

# 39. RATE LIMITING

Implementar por:

```text
tenant
API key
IP quando aplicável
endpoint
```

Possibilitar planos diferentes futuramente.

Headers:

```text
RateLimit-Limit
RateLimit-Remaining
RateLimit-Reset
```

---

# 40. OBSERVABILIDADE

OpenTelemetry em toda a plataforma.

Cada operação deve permitir correlação por:

```text
request_id
trace_id
tenant_id
company_id
document_id
batch_id
```

Nunca registrar secrets.

---

# 41. MÉTRICAS

No mínimo:

```text
http_requests_total
http_request_duration

documents_created_total
documents_authorized_total
documents_rejected_total
documents_failed_total

queue_depth
queue_processing_duration

worker_processing_duration
worker_failures_total

sefaz_requests_total
sefaz_latency
sefaz_errors_total

webhook_delivery_total
webhook_success_total
webhook_failure_total

certificate_expiration_days
```

---

# 42. HEALTH

Criar:

```text
GET /health
GET /ready
```

Internamente:

```text
GET /internal/metrics
```

Não expor informações sensíveis nos health checks.

---

# 43. SEGURANÇA

Implementar defesa em profundidade.

No mínimo:

```text
TLS

WAF

rate limiting

RBAC

scopes

Argon2id

KMS/Vault

AES-256-GCM

secret rotation

audit logs

network segmentation

containers non-root

read-only filesystem quando possível

dependency scanning

container scanning

SAST

DAST

CSRF quando aplicável

CORS restritivo

security headers
```

---

# 44. LOGS

NUNCA registrar:

```text
password
refresh token
access token completo
API key completa
certificado PFX
senha PFX
private key
KMS key material
```

Aplicar redaction automática.

---

# 45. AUDITORIA

Registrar:

```text
actor
tenant
company
action
resource
IP
user-agent
timestamp
request_id
metadata segura
```

Eventos:

```text
company.created

certificate.created
certificate.updated
certificate.deleted

api_key.created
api_key.revoked

document.created
document.authorized
document.rejected
document.canc