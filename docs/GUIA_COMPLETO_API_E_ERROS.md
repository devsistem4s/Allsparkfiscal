# Guia Completo da API Fiscal & Catálogo de Erros Acionáveis — AllSpark Fiscal

Este documento segue o padrão das melhores referências do mercado brasileiro (inspirado na documentação e Postman Collection da **Focus NFe**, **NotaaS**, **Spedy** e **TecnoSpeed PlugNotas**), apresentando:
1. **Introdução, Autenticação, Ambientes e Idempotência (`ref` e `Idempotency-Key`)**
2. **Fluxo Assíncrono de Alta Escala (50.000+ notas/dia) e Webhooks (`webhook_url` por requisição ou global)**
3. **Referência de Todos os Endpoints da API (Iniciantes e Avançados)**
4. **Servidor MCP Nativo (`/v1/mcp`) para Agentes de IA**
5. **Catálogo Completo de Erros Simplificados ("Qual foi o erro" e "O que fazer")**

---

## 1. Introdução e Conceitos Fundamentais

A **AllSpark Fiscal** é uma API REST multi-tenant para emissão, validação, consulta e gestão de documentos fiscais eletrônicos (**NF-e modelo 55** e **NFC-e modelo 65**) em todas as **27 Unidades Federativas do Brasil** (incluindo Reforma Tributária **IBS/CBS/IS** e **DIFAL**).

### 1.1. Autenticação
Todas as chamadas aos endpoints `/v1/*` exigem autenticação via Bearer Token no cabeçalho HTTP:
```http
Authorization: Bearer af_live_...
Content-Type: application/json
```

### 1.2. Ambientes (`environment`)
- `homologation`: Ambiente de testes da SEFAZ (`tpAmb = 2`), sem valor fiscal.
- `production`: Ambiente de produção com validade jurídica (`tpAmb = 1`).

### 1.3. Referência Externa (`ref`) e Idempotência (`Idempotency-Key`)
Para evitar duplicidade de notas (Rejeição SEFAZ `539` / `204`), você pode identificar cada nota pelo ID do pedido no seu sistema (`ref`):
- Passe `"ref": "PEDIDO-1001"` no corpo do `POST /v1/documents` ou `POST /v1/orders/invoice` (ou `?ref=PEDIDO-1001` na URL).
- Depois, consulte, baixe o XML/PDF, cancele ou emita Carta de Correção diretamente usando a sua referência:
  - `GET /v1/documents/ref/PEDIDO-1001`
  - `GET /v1/documents/ref/PEDIDO-1001/xml`
  - `GET /v1/documents/ref/PEDIDO-1001/pdf`
  - `POST /v1/documents/ref/PEDIDO-1001/cancel`
  - `POST /v1/documents/ref/PEDIDO-1001/correction-letter`

---

## 2. Webhooks de Retorno (Por Requisição ou Cadastrados)

Como a autorização na SEFAZ é processada de forma assíncrona por filas de alta vazão (RabbitMQ + Pool de Workers), você **não precisa fazer polling**. A API notifica seu sistema automaticamente assim que a nota é aprovada (`authorized`), validada (`validated`), rejeitada (`rejected`) ou cancelada (`cancelled`).

### 2.1. Opção A — Webhook Direto na Requisição da Nota (`webhook_url`)
Basta enviar o campo `webhook_url` (e opcionalmente `webhook_secret`) no próprio JSON de emissão da nota (`POST /v1/documents` ou `POST /v1/orders/invoice`):

```json
{
  "company_id": "bb7cbc40-79d5-4830-8a75-78c31fa764d3",
  "ref": "PEDIDO-9001",
  "webhook_url": "https://api.meusistema.com.br/webhooks/nfe",
  "webhook_secret": "meu-segredo-hmac",
  "operation": "issue",
  "invoice": { ... }
}
```

### 2.2. Opção B — Webhook Global da Empresa ou Tenant (`POST /v1/webhooks`)
Você também pode registrar URLs fixas para receber eventos de todas as notas:
```http
POST /v1/webhooks
{
  "company_id": "bb7cbc40-79d5-4830-8a75-78c31fa764d3",
  "url": "https://api.meusistema.com.br/webhooks/fiscal",
  "secret": "whsec_segredo_empresa",
  "events": ["document.authorized", "document.rejected", "document.cancelled"]
}
```

### 2.3. Formato do Payload Entregue no seu Webhook e Assinatura `HMAC-SHA256`
A AllSpark Fiscal faz um `POST` para sua URL com os cabeçalhos:
- `X-AllSpark-Event`: `document.authorized`, `document.validated`, `document.rejected` ou `document.cancelled`
- `X-AllSpark-Signature`: `sha256=<assinatura_hmac_sha256_do_body>`

Corpo enviado ao seu servidor:
```json
{
  "event": "document.authorized",
  "document_id": "f5b53809-3817-40e9-a893-f0c2e01c2794",
  "company_id": "bb7cbc40-79d5-4830-8a75-78c31fa764d3",
  "ref": "PEDIDO-9001",
  "kind": "nfe",
  "environment": "production",
  "series": 1,
  "number": 1042,
  "status": "authorized",
  "access_key": "29261111222333000181550010000010421849201928",
  "xml_url": "/v1/documents/f5b53809-3817-40e9-a893-f0c2e01c2794/xml",
  "pdf_url": "/v1/documents/f5b53809-3817-40e9-a893-f0c2e01c2794/pdf",
  "result": {
    "protocol": "129260000123456",
    "status": "authorized",
    "errors": []
  },
  "timestamp": "2026-11-14T11:20:00Z"
}
```

---

## 3. Mapa Completo de Endpoints da API

### 3.1. Saúde e Capacidades
| Método | Endpoint | Descrição |
|---|---|---|
| `GET` | `/healthz` | Liveness check do container da API |
| `GET` | `/readyz` | Readiness check (verifica PostgreSQL e Redis) |
| `GET` | `/v1/capabilities` | Recursos ativos (Reforma Tributária, DIFAL, Webhooks, MCP, Escudo Anti-656) |
| `GET` | `/v1/states` | Lista todas as 27 UFs brasileiras, códigos IBGE e autorizadores SEFAZ |

### 3.2. Empresas, Certificados A1, Séries e Perfis Tributários
| Método | Endpoint | Descrição |
|---|---|---|
| `POST` | `/v1/companies` | Cadastra empresa emitente (suporta `logo_base64` para DANFE e `contingency_mode`) |
| `GET` | `/v1/companies` | Lista empresas com paginação por cursor |
| `GET` | `/v1/companies/{id}` | Detalhes da empresa emitente |
| `PATCH` | `/v1/companies/{id}` | Atualiza dados cadastrais, `logo_base64` ou `contingency_mode` |
| `DELETE` | `/v1/companies/{id}` | Desativa (arquiva) uma empresa emitente |
| `PUT` | `/v1/companies/{id}/fiscal/{kind}` | Configura série, numeração inicial, CSC (NFC-e) e ambiente (`nfe` ou `nfce`) |
| `GET` | `/v1/companies/{id}/fiscal` | Consulta configurações fiscais da empresa |
| `POST` | `/v1/companies/{id}/certificates` | Upload seguro do Certificado Digital ICP-Brasil A1 (`.pfx` em base64, criptografia envelope AES-GCM) |
| `GET` | `/v1/companies/{id}/certificate` | Metadados e validade do certificado ativo |
| `DELETE` | `/v1/companies/{id}/certificate` | Revoga/desativa o certificado atual da empresa |
| `GET` | `/v1/certificates/expiring?days=30` | **Monitor de Vencimento**: lista certificados de todo o tenant que vencem nos próximos `N` dias |
| `POST` | `/v1/companies/{id}/tax-profiles` | Cria/atualiza um **Perfil Tributário** reutilizável (ex.: `REVENDA_PADRAO`, `PRODUCAO_PROPRIA`) |
| `GET` | `/v1/companies/{id}/tax-profiles` | Lista perfis tributários da empresa |

### 3.3. Emissão de Notas Fiscais (Modo Iniciante e Modo Avançado)
| Método | Endpoint | Descrição |
|---|---|---|
| `POST` | `/v1/orders/invoice` | **Modo Ultra-Simplificado (Order-to-Invoice)**: envie apenas cliente, itens (SKU, descrição, quantidade, preço) e opcionalmente `tax_profile`, `ref` e `webhook_url`. |
| `POST` | `/v1/documents` | **Modo Nacional Simplificado ou Avançado**: aceita payload mínimo (Smart Defaults) ou tributação completa (`ICMS`, `ICMSUFDest`/DIFAL, `IPI`, `PIS`, `COFINS`, `IBSCBS`, `IS`). |
| `POST` | `/v1/documents/bulk` | Enfileiramento em lote (até 50 notas por chamada HTTP). |
| `POST` | `/v1/documents/validate` | Dry-run síncrono: valida regras fiscais e calcula impostos/totais sem persistir nem consumir numeração. |

### 3.4. Consulta, XML, DANFE PDF e Eventos Fiscais (Por `id` ou por `ref`)
| Método | Endpoint por UUID (`{id}`) | Endpoint por Referência (`{ref}`) | Descrição |
|---|---|---|---|
| `GET` | `/v1/documents/{id}` | `/v1/documents/ref/{ref}` | Consulta status, chave de 44 dígitos, protocolo e erros explicados |
| `GET` | `/v1/documents/{id}/xml` | `/v1/documents/ref/{ref}/xml` | Download direto do XML assinado/autorizado (`application/xml`) |
| `GET` | `/v1/documents/{id}/pdf` | `/v1/documents/ref/{ref}/pdf` | Download direto do DANFE / DANFCE em PDF com logotipo (`application/pdf`) |
| `POST` | `/v1/documents/{id}/cancel` | `/v1/documents/ref/{ref}/cancel` | Cancelamento de NF-e/NFC-e (Evento `110111`) |
| `POST` | `/v1/documents/{id}/correction-letter` | `/v1/documents/ref/{ref}/correction-letter` | Carta de Correção Eletrônica — CC-e (Evento `110110`) |
| `POST` | `/v1/documents/{id}/query` | — | Consulta protocolo/situação atual na SEFAZ (protegido contra Rejeição `656`) |
| `POST` | `/v1/documents/{id}/reconcile` | — | Reconciliação segura de notas pendentes de retorno |
| `POST` | `/v1/documents/{id}/webhook/resend` | `/v1/documents/ref/{ref}/webhook/resend` | Reenvia imediatamente o Webhook da nota fiscal |

### 3.5. Operações SEFAZ por Empresa e Fechamento Contábil
| Método | Endpoint | Descrição |
|---|---|---|
| `GET` | `/v1/companies/{id}/sefaz/status` | Consulta disponibilidade (`sefazStatus`) da SEFAZ da UF da empresa |
| `POST` | `/v1/companies/{id}/void-range` | Inutilização de faixa de numeração (`sefazInutiliza`) |
| `POST` | `/v1/companies/{id}/dfe/sync` | Distribuição DF-e (`sefazDistDFe`) por `ult_nsu`, `nsu` ou `access_key` (com trava de 1h contra `cStat 137/656`) |
| `POST` | `/v1/companies/{id}/dfe/{access_key}/manifest` | **Manifestação do Destinatário (MDe)**: `ciencia` (`210210`), `confirmacao` (`210200`), `desconhecimento` (`210220`), `nao_realizada` (`210240`) |
| `GET` | `/v1/companies/{id}/backups/{YYYY-MM}.zip` | **Fechamento Contábil Mensal**: baixa arquivo `.zip` com todos os XMLs, PDFs e `resumo-contador.json` do mês |

### 3.6. Consultas Auxiliares e Diagnóstico (`/v1/utils/*`)
| Método | Endpoint | Descrição |
|---|---|---|
| `GET` | `/v1/utils/errors` | Lista o catálogo completo de erros com `message` (qual foi o erro) e `action` (o que fazer) |
| `GET` | `/v1/utils/errors/{code}` | Explica detalhadamente um código de erro da API ou `cStat` da SEFAZ (ex.: `SEFAZ_539`, `SEFAZ_656`) |
| `GET` | `/v1/utils/cfop` | Lista CFOPs de operações internas, interestaduais, devolução e exportação |
| `GET` | `/v1/utils/ncm` | Consulta códigos NCM frequentes |
| `GET` | `/v1/utils/cep/{cep}` | Normaliza e valida formato de CEP |
| `GET` | `/v1/utils/cnpj/{cnpj}` | Valida dígito verificador de CNPJ (numérico atual e alfanumérico 2026+) |

---

## 4. Servidor MCP Nativo (`/v1/mcp`) para Agentes de IA

A AllSpark Fiscal inclui um servidor **Model Context Protocol (MCP)** nativo via JSON-RPC 2.0 no endpoint `/v1/mcp`:
- `GET /v1/mcp`: Retorna o manifesto do servidor MCP e a lista de ferramentas (`tools`).
- `POST /v1/mcp`: Executa métodos JSON-RPC 2.0 (`initialize`, `tools/list`, `tools/call`):
  - `diagnose_fiscal_error`: recebe qualquer código (`SEFAZ_539`, `SEFAZ_656`, `CERTIFICATE_REQUIRED`) e devolve a explicação humana e o passo a passo de resolução.
  - `get_document_status`: consulta uma nota fiscal por `id` ou `ref`.
  - `resend_document_webhook`: dispara novamente o webhook de uma nota fiscal.
  - `list_brazilian_states`: lista todas as 27 UFs e seus autorizadores SEFAZ.

---

## 5. Catálogo de Respostas Simplificadas de Erros ("Qual foi o erro" + "O que fazer")

Todas as respostas de erro da API e todas as rejeições da SEFAZ retornam um objeto estruturado padronizado contendo:
- `code`: Código identificador único do erro (ex.: `SEFAZ_539`, `CERTIFICATE_REQUIRED`).
- `sefaz_code`: Código numérico oficial `cStat` da SEFAZ (quando originado na SEFAZ).
- `field`: Campo do JSON relacionado ao erro (quando aplicável).
- `message`: **Qual foi o erro** em linguagem clara e direta.
- `action`: **O que fazer** passo a passo para corrigir o problema.
- `retryable`: Indica se a tentativa pode ser repetida automaticamente (`true`) ou exige correção nos dados (`false`).

### 5.1. Exemplo de Resposta de Erro Simplificada
```json
{
  "data": null,
  "error": {
    "code": "SEFAZ_539",
    "sefaz_code": "539",
    "message": "Duplicidade de NF-e com diferença na Chave de Acesso [cStat 539].",
    "action": "A série e número desta nota já foram utilizados anteriormente na SEFAZ com outra chave de acesso. Consulte a nota original ou avance a numeração da série em PUT /v1/companies/{id}/fiscal/nfe.",
    "source": "sefaz",
    "retryable": false
  },
  "request_id": "9f8c7b6a-1234-4567-89ab-cdef01234567"
}
```

### 5.2. Tabela de Códigos de Erro da API e Rejeições SEFAZ Mais Comuns

| Código (`code`) | `cStat` SEFAZ | Qual foi o erro (`message`) | O que fazer para resolver (`action`) |
|---|---|---|---|
| `UNAUTHORIZED` | — | Credencial de acesso ausente, inválida ou revogada. | Envie o header `Authorization: Bearer af_...` com uma chave ativa do seu tenant. |
| `FORBIDDEN` | — | Sua chave de API não possui permissão (scope) para executar esta operação. | Utilize uma chave de API com permissão de escrita (`*:write`) para o recurso solicitado. |
| `RATE_LIMITED` | — | Limite de requisições ou proteção anti-bloqueio SEFAZ atingido. | Aguarde alguns segundos antes de repetir. Para consultas SEFAZ, evite consultar a mesma nota repetidamente (prevenção de Rejeição 656). |
| `FISCAL_CONFIG_REQUIRED` | — | A empresa ainda não possui configuração fiscal ativa para este modelo (`nfe` ou `nfce`). | Configure a série e o ambiente em `PUT /v1/companies/{id}/fiscal/nfe` (ou `nfce`) antes de emitir. |
| `CERTIFICATE_REQUIRED` | — | A empresa não possui um certificado digital A1 ativo e dentro da validade. | Faça o upload do arquivo `.pfx`/`.p12` e senha em `POST /v1/companies/{id}/certificates`. |
| `CERTIFICATE_COMPANY_MISMATCH` | — | O CNPJ do certificado digital A1 não pertence ao CNPJ da empresa cadastrada ou está vencido. | Envie o certificado e-CNPJ A1 correspondente à mesma raiz de CNPJ da empresa e dentro do prazo de validade. |
| `FISCAL_VALIDATION_FAILED` | — | O XML da nota fiscal não passou na validação do Schema XSD oficial (`PL_010_V1.30`). | Confira códigos fiscais (`CFOP`, `NCM`, `CST`/`CSOSN`, `cClassTrib`) ou use o modo simplificado omitindo `taxes` para preenchimento seguro. |
| `SEFAZ_204` | `204` | Duplicidade de NF-e [cStat 204]. | Esta exata chave de acesso já foi autorizada na SEFAZ. Chame `POST /v1/documents/{id}/reconcile` ou `/query` para recuperar o protocolo e gerar o XML autorizado (`nfeProc`). |
| `SEFAZ_539` | `539` | Duplicidade de NF-e com diferença na Chave de Acesso [cStat 539]. | A série e número desta nota já foram utilizados anteriormente na SEFAZ com outra chave de acesso. Avance a numeração da série em `PUT /v1/companies/{id}/fiscal/nfe`. |
| `SEFAZ_215` | `215` | Falha no Schema XML na SEFAZ [cStat 215]. | Verifique caracteres especiais ou campos obrigatórios da UF no endereço/itens. |
| `SEFAZ_225` | `225` | Falha no Schema XML do lote de NFe [cStat 225]. | Verifique o preenchimento dos campos de tributação do item (`ICMS`, `PIS`, `COFINS`, `IBSCBS`). |
| `SEFAZ_301` | `301` | Uso Denegado: Irregularidade fiscal do emitente [cStat 301]. | A Inscrição Estadual da empresa emitente está com irregularidade perante a SEFAZ estadual. Regularize a IE junto ao contador/SEFAZ. |
| `SEFAZ_302` | `302` | Uso Denegado: Irregularidade fiscal do destinatário [cStat 302]. | A Inscrição Estadual do destinatário está bloqueada/suspensa na SEFAZ de destino. |
| `SEFAZ_501` | `501` | Prazo de cancelamento superior ao previsto na Legislação [cStat 501]. | O prazo regulamentar para cancelamento normal expirou (geralmente 24h na NF-e e 30 min na NFC-e). |
| `SEFAZ_594` | `594` | O número de sequência do evento informado é maior que o permitido [cStat 594]. | Para Carta de Correção (`CC-e`), envie `sequence` sequencial (1, 2, 3... até 20). |
| `SEFAZ_656` | `656` | Consumo Indevido pelo aplicativo da empresa [cStat 656 — NT 2018.002]. | A SEFAZ bloqueou temporariamente o CNPJ/IP por excesso de consultas repetidas ou lotes rejeitados idênticos. O escudo Redis da AllSpark Fiscal ativou pausa automática de 60 minutos para liberar o CNPJ na SEFAZ sem renovar o bloqueio. |
| `SEFAZ_783` | `783` | NFC-e não permite operação interestadual ou comércio exterior [cStat 783]. | Para vendas interestaduais (`idDest=2`), emita uma NF-e modelo 55 (`kind: "nfe"`) em vez de NFC-e (`kind: "nfce"`). |
| `SEFAZ_897` | `897` | Código numérico (`cNF`) em formato inválido ou igual ao número da nota [cStat 897]. | A AllSpark Fiscal já gera `cNF` pseudoaleatório derivado do UUID; se importou XML externo, altere o `cNF`. |
| `SEFAZ_999` | `999` | Erro não catalogado na SEFAZ Estadual [cStat 999]. | Trata-se de instabilidade interna temporária nos servidores da SEFAZ. O worker repetirá automaticamente ou você pode ativar contingência (`contingency_mode: "auto"`). |

