# AllSpark Fiscal — Guia Passo a Passo de Integração do Cliente (Validação Real)

> **Documento Oficial de Integração & Homologação Real**  
> **CNPJ Validado:** `63.687.210/0001-10` (63.687.210 SILAS PAULO SILVA GOMES)  
> **Ambiente Oficial:** `https://allsparkfiscal.allsparkhub.com`  
> **Certificado Validado:** A1 ICP-Brasil (AC SOLUTI Multipla v5, Válido até 21/05/2027)

---

## 1. Resumo da Execução Real com o seu CNPJ

Realizamos a homologação completa utilizando o seu CNPJ oficial e o seu Certificado Digital A1 real localizado em `/home/silane/Documentos/certificado.pfx`.

```mermaid
sequenceDiagram
    autonumber
    actor Cliente as Desenvolvedor / ERP
    participant API as AllSpark Gateway (HTTPS)
    participant Auth as Auth & Engine Cripto
    participant Queue as RabbitMQ Quorum Queue
    participant Worker as Worker Go + Engine NFePHP
    participant Storage as Object Storage (/data/objects)
    participant Webhook as Webhook Receiver

    Cliente->>API: 1. Cadastra Empresa (CNPJ 63687210000110)
    API-->>Cliente: Empresa criada (ID: 85699d35-0515...)
    Cliente->>API: 2. Upload Certificado A1 + Senha
    API->>Auth: Valida ICP-Brasil e cifra com AES-256-GCM
    Auth-->>Cliente: Certificado Válido até 21/05/2027
    Cliente->>API: 3. Configura Série Fiscal 1 (NF-e)
    Cliente->>API: 4. Consulta Readiness (ready_for_nfe: true)
    Cliente->>API: 5. Emite Pedido (SILAS-NF-001) + Webhook
    API->>Queue: Enfileira Job Fiscal (202 Accepted)
    Queue->>Worker: Consome Job
    Worker->>Worker: Assina XML (XMLDSig) + Gera DANFE PDF
    Worker->>Storage: Grava XML e PDF
    Worker->>Webhook: Dispara Webhook assinado (HMAC-SHA256)
    Cliente->>API: 6. Baixa XML Assinado & DANFE em PDF
```

### Resultados da Emissão Real:
* **ID da Empresa:** `85699d35-0515-423c-ad09-20b3eaeb4f73`
* **ID do Certificado:** `29df2faf-8877-45ff-b5ee-cfc0bfda75d3` (Fingerprint: `54889febf199fbbec3...`)
* **Chave de Acesso da NF-e (44 dígitos gerados):**
  $$\mathbf{29261163687210000110550010000000011464467475}$$
  *(Contém UF 29, Ano 26, Mês 11, seu CNPJ 63687210000110, Modelo 55, Série 1, Número 1)*
* **XML Assinado:** Gerado com `5.992 bytes`, contendo a assinatura digital RSA-SHA1 com o seu certificado real da AC SOLUTI.
* **DANFE Oficial:** Renderizado em PDF com `15.130 bytes` e salvo localmente em [`danfe_silas_real.pdf`](file:///home/silane/Documentos/sistema/AllSpark%20Fiscal/danfe_silas_real.pdf).

---

## 2. Passo a Passo Completo que Todo Cliente Deve Seguir

Abaixo está o tutorial exato com requisições manuais `curl` e payloads JSON para qualquer cliente integrar ao AllSpark Fiscal.

### Passo 1: Autenticação
Todas as requisições exigem o cabeçalho:
```http
Authorization: Bearer af_cBKACw1-xFvCspPFWzQLQmb2huLooVtWkavh4vTygE4
Content-Type: application/json
```

---

### Passo 2: Cadastrar a Empresa Emitente
O cliente cadastra os dados fiscais e o endereço da empresa:

```bash
curl -X POST https://allsparkfiscal.allsparkhub.com/v1/companies \
  -H "Authorization: Bearer SEU_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "cnpj": "63687210000110",
    "name": "63.687.210 SILAS PAULO SILVA GOMES",
    "uf": "BA",
    "ie": "ISENTO",
    "crt": 1,
    "address": {
      "street": "2a Travessa do Caju",
      "number": "SN",
      "district": "Periperi",
      "city_code": "2927408",
      "city": "Salvador",
      "state": "BA",
      "postal_code": "40727815"
    }
  }'
```
**Resposta de Sucesso (HTTP 201 Created):**
```json
{
  "data": {
    "id": "85699d35-0515-423c-ad09-20b3eaeb4f73",
    "cnpj": "63687210000110",
    "name": "63.687.210 SILAS PAULO SILVA GOMES",
    "uf": "BA",
    "ie": "ISENTO",
    "crt": 1,
    "contingency_mode": "auto"
  }
}
```

---

### Passo 3: Enviar o Certificado Digital A1
O cliente converte o arquivo `.pfx` em Base64 e envia junto com a senha:

```bash
PFX_B64=$(base64 -w 0 /caminho/para/certificado.pfx)

curl -X POST https://allsparkfiscal.allsparkhub.com/v1/companies/85699d35-0515-423c-ad09-20b3eaeb4f73/certificates \
  -H "Authorization: Bearer SEU_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{
    \"pfx_base64\": \"$PFX_B64\",
    \"password\": \"@S3nh4#f0rT3\"
  }"
```
**Resposta de Sucesso (HTTP 200 OK):**
```json
{
  "data": {
    "id": "29df2faf-8877-45ff-b5ee-cfc0bfda75d3",
    "fingerprint": "54889febf199fbbec334f617a07d66350367f217caf41e1f686c0ea8a28b3c7a",
    "expires_at": "2027-05-21T19:06:00Z"
  }
}
```

---

### Passo 4: Habilitar a Série Fiscal (NF-e ou NFC-e)
Define a série inicial e o ambiente (homologação ou produção):

```bash
curl -X PUT https://allsparkfiscal.allsparkhub.com/v1/companies/85699d35-0515-423c-ad09-20b3eaeb4f73/fiscal/nfe \
  -H "Authorization: Bearer SEU_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "environment": "homologation",
    "series": 1,
    "enabled": true
  }'
```

---

### Passo 5: Criar um Perfil Tributário Reutilizável (Opcional, mas Recomendado)
Evita que o cliente precise enviar alíquotas complexas em cada pedido:

```bash
curl -X POST https://allsparkfiscal.allsparkhub.com/v1/companies/85699d35-0515-423c-ad09-20b3eaeb4f73/tax-profiles \
  -H "Authorization: Bearer SEU_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "PADRAO_SIMPLES",
    "description": "Venda e Servicos Simples Nacional",
    "ncm_default": "84713012",
    "cfop_internal": "5102",
    "cfop_interstate": "6102",
    "taxes": {
      "icms": {"origin": 0, "csosn": "102"},
      "pis": {"cst": "49"},
      "cofins": {"cst": "49"}
    }
  }'
```

---

### Passo 6: Verificar Prontidão da Empresa (`Readiness Check`)
Antes de emitir, o cliente valida se falta algum requisito:

```bash
curl -H "Authorization: Bearer SEU_TOKEN" \
  https://allsparkfiscal.allsparkhub.com/v1/companies/85699d35-0515-423c-ad09-20b3eaeb4f73/readiness
```
**Resposta:**
```json
{
  "data": {
    "ready_for_nfe": true,
    "has_valid_certificate": true,
    "certificate_days_remaining": 187,
    "pending_steps": null
  }
}
```

---

### Passo 7: Emitir uma Nota Fiscal (`POST /v1/orders/invoice`)
O cliente envia o pedido de forma intuitiva, passando o seu código de referência (`ref`) e a URL do Webhook:

```bash
curl -X POST https://allsparkfiscal.allsparkhub.com/v1/orders/invoice \
  -H "Authorization: Bearer SEU_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "company_id": "85699d35-0515-423c-ad09-20b3eaeb4f73",
    "ref": "SILAS-NF-001",
    "operation": "generate",
    "webhook_url": "https://meu-erp.com/webhook",
    "webhook_secret": "chave-secreta-hmac-123",
    "tax_profile": "PADRAO_SIMPLES",
    "customer": {
      "name": "NF-E EMITIDA EM AMBIENTE DE HOMOLOGACAO - SEM VALOR FISCAL",
      "tax_id": "11144477735",
      "ie_indicator": 9,
      "address": {
        "street": "Avenida Tancredo Neves",
        "number": "500",
        "district": "Caminho das Arvores",
        "city": "Salvador",
        "state": "BA",
        "postal_code": "41820021",
        "city_code": "2927408"
      }
    },
    "items": [
      {
        "sku": "PROD-NOTEBOOK-DEV",
        "description": "NOTEBOOK DESENVOLVEDOR ALLSPARK CORE I7 32GB SSD 1TB",
        "ncm": "84713012",
        "quantity": "1.0000",
        "price": "3500.00"
      }
    ],
    "payment_method": "pix"
  }'
```
**Resposta Imediata (HTTP 202 Accepted):**
```json
{
  "data": {
    "id": "87fa2a33-08ae-4400-81cd-c44eba503e2b",
    "ref": "SILAS-NF-001",
    "status_url": "/v1/documents/87fa2a33-08ae-4400-81cd-c44eba503e2b",
    "ref_url": "/v1/documents/ref/SILAS-NF-001",
    "xml_url": "/v1/documents/87fa2a33-08ae-4400-81cd-c44eba503e2b/xml",
    "pdf_url": "/v1/documents/87fa2a33-08ae-4400-81cd-c44eba503e2b/pdf"
  }
}
```

---

### Passo 8: Consultar o Retorno da Nota Fiscal
A qualquer momento, o cliente pode consultar pelo seu próprio código de pedido (`ref`):

```bash
curl -H "Authorization: Bearer SEU_TOKEN" \
  https://allsparkfiscal.allsparkhub.com/v1/documents/ref/SILAS-NF-001
```
**Resposta Completa:**
```json
{
  "data": {
    "id": "87fa2a33-08ae-4400-81cd-c44eba503e2b",
    "ref": "SILAS-NF-001",
    "status": "validated",
    "access_key": "29261163687210000110550010000000011464467475",
    "series": 1,
    "number": 1,
    "xml_url": "/v1/documents/87fa2a33-08ae-4400-81cd-c44eba503e2b/xml",
    "pdf_url": "/v1/documents/87fa2a33-08ae-4400-81cd-c44eba503e2b/pdf"
  }
}
```

---

### Passo 9: Baixar o XML Oficial Assinado
```bash
curl -H "Authorization: Bearer SEU_TOKEN" \
  https://allsparkfiscal.allsparkhub.com/v1/documents/ref/SILAS-NF-001/xml \
  -o nota_fiscal.xml
```

---

### Passo 10: Baixar o DANFE em PDF
```bash
curl -H "Authorization: Bearer SEU_TOKEN" \
  https://allsparkfiscal.allsparkhub.com/v1/documents/ref/SILAS-NF-001/pdf \
  -o danfe.pdf
```

---

### Passo 11: Reenviar o Webhook (se o ERP do cliente estava offline)
```bash
curl -X POST -H "Authorization: Bearer SEU_TOKEN" \
  https://allsparkfiscal.allsparkhub.com/v1/documents/ref/SILAS-NF-001/webhook/resend
```

---

### Passo 12: Cancelamento da Nota Fiscal
Conforme exigido pela legislação tributária (MOC SEFAZ):
* A justificativa deve conter entre 15 e 255 caracteres.
* Apenas notas autorizadas com chave de acesso e protocolo podem ser canceladas via evento SEFAZ `110111`.

```bash
curl -X POST https://allsparkfiscal.allsparkhub.com/v1/documents/ref/SILAS-NF-001/cancel \
  -H "Authorization: Bearer SEU_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "reason": "Cancelamento referente a teste de homologacao fiscal do cliente"
  }'
```

---

## 3. Validação de Erros Comuns e Respostas Explicativas da API

Durante a sessão, validamos intencionalmente todos os cenários de erro da API para garantir que o cliente nunca fique perdido sem saber o que fazer:

| Cenário de Erro | Código HTTP | Código AllSpark | Mensagem Retornada | Ação Explicativa Sugerida |
|---|:---:|---|---|---|
| **Payload do Certificado com chave errada** | `422` | `INVALID_CERTIFICATE` | *"Informe certificado A1 e senha."* | Use `pfx_base64` e `password`. |
| **Certificado pertencente a outro CNPJ** | `422` | `CERTIFICATE_COMPANY_MISMATCH` | *"O certificado deve pertencer ao CNPJ cadastrado."* | Confira o CNPJ do `.pfx` com a empresa. |
| **Justificativa de Cancelamento curta** | `422` | `INVALID_CANCEL_REASON` | *"A justificativa de cancelamento deve ter entre 15 e 255 caracteres."* | Expanda o texto da justificativa. |
| **Tentar Cancelar nota não autorizada** | `409` | `INVALID_DOCUMENT_STATE` | *"Somente documentos autorizados podem ser cancelados."* | Notas em `validated` ou `queued` não possuem protocolo SEFAZ para cancelamento. |
| **Token ausente ou inválido** | `401` | `UNAUTHORIZED` | *"Credencial de acesso ausente ou inválida."* | Envie o header `Authorization: Bearer af_...`. |
| **Tentar acessar dados de outro Tenant** | `404` | `NOT_FOUND` | *"Recurso não encontrado."* | Bloqueio automático de invasão horizontal (Anti-IDOR). |


---

## 4. Evidências de Webhooks Entregues (Tabela `webhook_deliveries` — Produção)

| Referência do Pedido | Evento | Status HTTP | Sucesso | Data/Hora |
|---|---|:---:|:---:|---|
| `PEDIDO-CURL-MANUAL-001` | `document.validated` | `200` | ✅ | 15/11/2026 15:24:13 (reenvio manual) |
| `PEDIDO-CURL-MANUAL-001` | `document.validated` | `200` | ✅ | 15/11/2026 15:21:48 (emissão original) |
| `SILAS-NF-001` | `document.validated` | `200` | ✅ | 15/11/2026 15:14:28 |
| `PEDIDO-WH-VAL-1794754890` | `document.validated` | `200` | ✅ | 15/11/2026 15:01:50 |
| `PEDIDO-WH-VAL-1794754890` | `document.validated` | `200` | ✅ | 15/11/2026 15:01:34 |

> [!NOTE]
> Webhooks são assinados com **HMAC-SHA256** no header `X-AllSpark-Signature`. O receptor deve validar essa assinatura antes de processar o payload.

---

## 5. Resumo dos Documentos Emitidos (Sessão de Homologação Real)

| Referência | Status | Chave de Acesso (44 dígitos) | Data |
|---|---|---|---|
| `SILAS-NF-001` | `validated` | `29261163687210000110550010000000011464467475` | 15/11/2026 |
| `E2E-PROD-169592` | `validated` | `29261111222333000181550010000000011540764491` | 15/11/2026 |
| `PEDIDO-WH-VAL-1794754890` | `validated` | `29261111222333000181550010000000011718555374` | 15/11/2026 |
| `PEDIDO-LIVE-001` | `validated` | `29261111222333000181550010000000011389098424` | 15/11/2026 |
| `PEDIDO-CURL-MANUAL-001` | `validated` | `29261163687210000110550010000000011649469784` | 15/11/2026 |

> [!IMPORTANT]
> Sistema em **modo homologação** (`ENABLE_PRODUCTION=false`). XML e DANFE gerados com certificado real, mas sem transmissão à SEFAZ. Para produção real mude para `ENABLE_PRODUCTION=true` no Swarm.

---

## 6. Próximos Passos para Produção Real

1. Ativar `ENABLE_PRODUCTION=true` na stack → NF-e transmitida e status `authorized` com protocolo SEFAZ
2. Configurar webhook para URL real do ERP (não httpbin.org)
3. Testar cancelamento real após `authorized` com `POST /cancel` + `reason` (15-255 chars)
4. NFC-e → requer CSC e CSCid obtidos junto à SEFAZ estadual
5. Monitorar via Portainer + dashboard Prometheus/Grafana no servidor
