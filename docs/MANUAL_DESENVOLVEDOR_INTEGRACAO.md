# AllSpark Fiscal — Manual de Implementação e Integração para Desenvolvedores 🧾⚡

> **Guia Oficial de Integração da API Fiscal (v1.0)**  
> **Ambiente de Produção:** `https://allsparkfiscal.allsparkhub.com`  
> **Público-alvo:** Desenvolvedores de software, arquitetos de sistemas e equipes de ERP/PDV.

---

## 1. Visão Geral da Plataforma

O **AllSpark Fiscal** é uma plataforma SaaS moderna de alta performance voltada para automação de documentos fiscais eletrônicos brasileiros (**NF-e Modelo 55** e **NFC-e Modelo 65**).

### Principais Diferenciais da Arquitetura:
* **Assíncrona e Resiliente:** A API recebe o pedido com `202 Accepted` em milissegundos e processa a assinatura e transmissão através de filas RabbitMQ Quorum Queues dedicadas.
* **Comunicação Direta com a SEFAZ:** Integração nativa com os webservices de todas as 27 Secretarias de Fazenda estaduais.
* **Segurança de Nível Bancário:** Certificados digitais A1 cifrados com chave mestra via **AES-256-GCM** (Vault isolado). Tokens em hash SHA-256 no banco de dados.
* **Webhooks Confiáveis:** Disparo com garantia de entrega, retries automáticos com exponential backoff e assinatura de integridade **HMAC-SHA256**.
* **Proteção Anti-Bloqueio SEFAZ (Anti-656 Shield):** Rate limiting inteligente por CNPJ e governor que impede que rajadas de notas atinjam os limites de consumo indevido da SEFAZ.

---

## 2. Autenticação e Headers

Todas as requisições para a API devem incluir o token de API (`API_KEY`) fornecido no provisionamento do seu Tenant:

```http
Authorization: Bearer af_SUA_CHAVE_DE_API_AQUI
Content-Type: application/json
Accept: application/json
```

### Cabeçalhos Opcionais Recomendados:
* `X-Request-ID`: UUID gerado pelo seu sistema para rastreamento de requisições de ponta a ponta.
* `Idempotency-Key`: Chave única (ex: `order-12345`) para evitar duplicidade em reenvios de rede.

---

## 3. Análise do Cadastro da Empresa e Descoberta de Dados do CNPJ

Para emitir notas fiscais em nome de qualquer empresa, a empresa precisa ser registrada na API. 

### O que a API precisa para o cadastro completo:
```json
{
  "cnpj": "63687210000110",
  "name": "63.687.210 SILAS PAULO SILVA GOMES",
  "uf": "BA",
  "ie": "237997157",
  "crt": 4,
  "address": {
    "street": "2A TRAVESSA DO CAJU",
    "number": "1",
    "district": "PERIPERI",
    "city": "Salvador",
    "state": "BA",
    "postal_code": "40727815",
    "city_code": "2927408"
  }
}
```

### Análise da Descoberta Automática de CNPJ:
1. **Dados Federais (Receita Federal / BrasilAPI):**
   * A API permite consultar e autodescobrir: Razão Social, Nome Fantasia, Logradouro, Número, Bairro, CEP, Município, Código IBGE do Município (`city_code`) e Regime Tributário básico (se MEI ou Simples).
2. **Dados Estaduais (Inscrição Estadual — IE):**
   * **Atenção:** A Receita Federal não fornece com precisão a Inscrição Estadual (IE) atualizada para todos os estados, pois a IE pertence ao cadastro estadual de cada SEFAZ.
   * Na SEFAZ Bahia e na maioria dos estados, se o emitente de uma NF-e modelo 55 for informado com `"ie": "ISENTO"`, a SEFAZ rejeitará a nota com **`Rejeição 554: IE do Emitente informada como ISENTO indevidamente`**.
   * **Recomendação para o Desenvolvedor:** O desenvolvedor pode preencher os dados de endereço e razão social automaticamente via consulta do CNPJ, mas **sempre deve solicitar ou confirmar a Inscrição Estadual (IE)** do cliente conforme o cadastro no Sintegra ou CCC da SEFAZ.
3. **Código de Regime Tributário (CRT):**
   * `1`: Simples Nacional (Microempresa / EPP)
   * `2`: Simples Nacional com excesso de sublimite
   * `3`: Regime Normal (Lucro Presumido / Lucro Real)
   * `4`: Simples Nacional - Microempreendedor Individual (MEI)

---

## 4. Ciclo de Vida da Integração (Passo a Passo)

```mermaid
sequenceDiagram
    autonumber
    actor Dev as Sistema do Cliente (ERP)
    participant API as AllSpark Fiscal Gateway
    participant SEFAZ as SEFAZ Estadual
    participant Webhook as Endpoint Webhook do ERP

    Note over Dev,API: FASE 1: ONBOARDING DA EMPRESA
    Dev->>API: 1. POST /v1/companies (Cadastra CNPJ, IE, Endereço, CRT)
    Dev->>API: 2. POST /v1/companies/{id}/certificates (Upload PFX Base64 + Senha)
    Dev->>API: 3. PUT /v1/companies/{id}/fiscal/nfe (Habilita Série 1)
    Dev->>API: 4. POST /v1/companies/{id}/tax-profiles (Cria Regra Tributária Padrão)
    Dev->>API: 5. GET /v1/companies/{id}/readiness (Valida se está pronto)

    Note over Dev,Webhook: FASE 2: EMISSÃO CONTÍNUA
    Dev->>API: 6. POST /v1/orders/invoice (Envia Pedido + ref + webhook_url)
    API-->>Dev: Retorna HTTP 202 Accepted (ID do documento)
    API->>SEFAZ: Assina XML e transmite em tempo real
    SEFAZ-->>API: Retorna Protocolo 100 (Autorizado)
    API->>Webhook: Dispara Webhook assinado (document.authorized)
    Dev->>API: 7. GET /v1/documents/ref/{ref}/xml (Baixa XML nfeProc)
    Dev->>API: 8. GET /v1/documents/ref/{ref}/pdf (Baixa DANFE PDF)
```

---

## 5. Referência Detalhada dos Endpoints

### 5.1. Cadastrar a Empresa Emitente
```http
POST /v1/companies
```
```bash
curl -X POST https://allsparkfiscal.allsparkhub.com/v1/companies \
  -H "Authorization: Bearer SEU_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "cnpj": "63687210000110",
    "name": "63.687.210 SILAS PAULO SILVA GOMES",
    "uf": "BA",
    "ie": "237997157",
    "crt": 4,
    "address": {
      "street": "2A TRAVESSA DO CAJU",
      "number": "1",
      "district": "PERIPERI",
      "city": "Salvador",
      "state": "BA",
      "postal_code": "40727815",
      "city_code": "2927408"
    }
  }'
```
**Resposta (HTTP 201 Created):**
```json
{
  "data": {
    "id": "85699d35-0515-423c-ad09-20b3eaeb4f73",
    "cnpj": "63687210000110",
    "name": "63.687.210 SILAS PAULO SILVA GOMES",
    "uf": "BA",
    "ie": "237997157",
    "crt": 4,
    "contingency_mode": "auto"
  }
}
```

---

### 5.2. Enviar o Certificado Digital A1
O certificado `.pfx` deve ser convertido em string Base64 e enviado com a senha:

```http
POST /v1/companies/{company_id}/certificates
```
```bash
PFX_B64=$(base64 -w 0 certificado.pfx)

curl -X POST https://allsparkfiscal.allsparkhub.com/v1/companies/85699d35-0515-423c-ad09-20b3eaeb4f73/certificates \
  -H "Authorization: Bearer SEU_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{
    \"pfx_base64\": \"$PFX_B64\",
    \"password\": \"@S3nh4#f0rT3\"
  }"
```
**Resposta (HTTP 200 OK):**
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

### 5.3. Habilitar Série Fiscal (NF-e ou NFC-e)
```http
PUT /v1/companies/{company_id}/fiscal/nfe
```
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
> Para ambiente comercial com valor jurídico, altere `"environment": "production"`.

---

### 5.4. Criar Perfil Tributário Reutilizável
O perfil tributário simplifica a emissão para que o ERP não precise calcular tags complexas de impostos a cada item:

```http
POST /v1/companies/{company_id}/tax-profiles
```
```bash
curl -X POST https://allsparkfiscal.allsparkhub.com/v1/companies/85699d35-0515-423c-ad09-20b3eaeb4f73/tax-profiles \
  -H "Authorization: Bearer SEU_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "PADRAO_SIMPLES",
    "description": "Vendas Gerais Simples Nacional / MEI",
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

### 5.5. Consultar Prontidão da Empresa (`Readiness Check`)
Valida se todos os requisitos da empresa estão preenchidos antes de emitir:

```http
GET /v1/companies/{company_id}/readiness
```
```json
{
  "data": {
    "company_id": "85699d35-0515-423c-ad09-20b3eaeb4f73",
    "cnpj": "63687210000110",
    "ready_for_nfe": true,
    "has_valid_certificate": true,
    "certificate_days_remaining": 187,
    "pending_steps": null
  }
}
```

---

### 5.6. Consultar Status do Servidor da SEFAZ
Verifica em tempo real se o webservice da SEFAZ daquele estado está online e respondendo ao certificado da empresa:

```http
GET /v1/companies/{company_id}/sefaz/status
```
```json
{
  "data": {
    "status": "online",
    "status_code": "107",
    "reason": "Servico em Operacao",
    "uf": "BA"
  }
}
```

---

### 5.7. Emitir Nota Fiscal (`POST /v1/orders/invoice`)
Este é o endpoint principal utilizado no dia a dia. Você pode enviar a nota identificando seu próprio código de pedido (`ref`):

```http
POST /v1/orders/invoice
```
```bash
curl -X POST https://allsparkfiscal.allsparkhub.com/v1/orders/invoice \
  -H "Authorization: Bearer SEU_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "company_id": "85699d35-0515-423c-ad09-20b3eaeb4f73",
    "ref": "PEDIDO-ERP-9988",
    "operation": "issue",
    "webhook_url": "https://meu-erp.com/webhook/fiscal",
    "webhook_secret": "minha-chave-secreta-hmac",
    "tax_profile": "PADRAO_SIMPLES",
    "customer": {
      "name": "CLIENTE EXEMPLO LTDA",
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
        "sku": "PROD-001",
        "description": "NOTEBOOK DESENVOLVEDOR CORE I7 32GB",
        "ncm": "84713012",
        "quantity": "1.0000",
        "price": "3500.00"
      }
    ],
    "payment_method": "pix"
  }'
```

**Resposta Imediata da API (HTTP 202 Accepted):**
```json
{
  "data": {
    "id": "547f28a6-960c-408c-b096-44730017acf3",
    "ref": "PEDIDO-ERP-9988",
    "ref_url": "/v1/documents/ref/PEDIDO-ERP-9988",
    "status_url": "/v1/documents/547f28a6-960c-408c-b096-44730017acf3",
    "xml_url": "/v1/documents/547f28a6-960c-408c-b096-44730017acf3/xml",
    "pdf_url": "/v1/documents/547f28a6-960c-408c-b096-44730017acf3/pdf"
  }
}
```

---

### 5.8. Consultar o Retorno da Nota Fiscal
Você pode consultar pelo seu próprio código de pedido (`ref`):

```http
GET /v1/documents/ref/{ref}
```
**Resposta com Nota Autorizada:**
```json
{
  "data": {
    "id": "547f28a6-960c-408c-b096-44730017acf3",
    "ref": "PEDIDO-ERP-9988",
    "status": "authorized",
    "access_key": "29261163687210000110550010000000041236372404",
    "series": 1,
    "number": 4,
    "result": {
      "status": "authorized",
      "protocol": {
        "number": "129261000710494",
        "status_code": "100",
        "reason": "Autorizado o uso da NF-e"
      }
    },
    "xml_url": "/v1/documents/547f28a6-960c-408c-b096-44730017acf3/xml",
    "pdf_url": "/v1/documents/547f28a6-960c-408c-b096-44730017acf3/pdf"
  }
}
```

---

### 5.9. Baixar o XML Oficial Autorizado (`nfeProc`)
Retorna o XML completo com a nota, a assinatura e o carimbo do protocolo da SEFAZ:

```http
GET /v1/documents/ref/{ref}/xml
```
```bash
curl -H "Authorization: Bearer SEU_TOKEN" \
  https://allsparkfiscal.allsparkhub.com/v1/documents/ref/PEDIDO-ERP-9988/xml \
  -o nota_fiscal_autorizada.xml
```

---

### 5.10. Baixar o DANFE em PDF
Retorna o documento auxiliar (DANFE) pronto para impressão ou envio por e-mail:

```http
GET /v1/documents/ref/{ref}/pdf
```
```bash
curl -H "Authorization: Bearer SEU_TOKEN" \
  https://allsparkfiscal.allsparkhub.com/v1/documents/ref/PEDIDO-ERP-9988/pdf \
  -o danfe.pdf
```

---

### 5.11. Cancelar uma Nota Fiscal na SEFAZ
Permite cancelar uma nota fiscal que já foi autorizada pela SEFAZ:
* A justificativa deve conter entre 15 e 255 caracteres.
* A SEFAZ registra o evento oficial `110111`.

```http
POST /v1/documents/ref/{ref}/cancel
```
```bash
curl -X POST https://allsparkfiscal.allsparkhub.com/v1/documents/ref/PEDIDO-ERP-9988/cancel \
  -H "Authorization: Bearer SEU_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "reason": "Cancelamento solicitado pelo cliente antes do envio da mercadoria"
  }'
```
**Resposta de Cancelamento com Sucesso:**
```json
{
  "data": {
    "id": "547f28a6-960c-408c-b096-44730017acf3",
    "status": "canceled",
    "protocol": {
      "number": "129261005268018",
      "status_code": "135",
      "reason": "Evento registrado e vinculado a NF-e"
    }
  }
}
```

---

## 6. Webhooks e Assinatura de Segurança (HMAC-SHA256)

Quando a nota é processada ou seu status é alterado pela SEFAZ, o AllSpark Fiscal envia uma requisição `POST` para a `webhook_url` informada na emissão.

### Eventos Disparados:
* `document.authorized`: Nota fiscal aprovada pela SEFAZ (com número de protocolo e chave de acesso).
* `document.rejected`: Nota rejeitada pela SEFAZ (com código e motivo do erro).
* `document.canceled`: Nota cancelada com sucesso na SEFAZ.

### Validação da Assinatura no seu Servidor:
Cada webhook possui o cabeçalho:
```http
X-AllSpark-Signature: sha256=HEX_DA_ASSINATURA
X-AllSpark-Event: document.authorized
```

**Como validar em Node.js:**
```javascript
const crypto = require('crypto');

function verifyWebhook(payloadRaw, signatureHeader, secret) {
  const hash = 'sha256=' + crypto.createHmac('sha256', secret).update(payloadRaw).digest('hex');
  return crypto.timingSafeEqual(Buffer.from(hash), Buffer.from(signatureHeader));
}
```

**Como validar em Python:**
```python
import hmac, hashlib

def verify_webhook(payload_bytes, signature_header, secret_str):
    expected = "sha256=" + hmac.new(secret_str.encode(), payload_bytes, hashlib.sha256).hexdigest()
    return hmac.compare_digest(expected, signature_header)
```

---

## 7. Catálogo de Erros e Diagnósticos Explicativos

O AllSpark Fiscal segue o padrão **Fail-Explanatory**: nenhuma requisição com erro retorna mensagens genéricas sem ação recomendada.

| Código da API | Código SEFAZ | Causa Real | Ação Recomendada para o Desenvolvedor |
|---|:---:|---|---|
| `SEFAZ_100` | 100 | Autorizado | Nota autorizada com sucesso. |
| `SEFAZ_135` | 135 | Evento Vinculado | Cancelamento ou Carta de Correção homologado. |
| `SEFAZ_180` | 180 | CRT Diverge da Receita | Atualize o `crt` da empresa (`1` para Simples, `4` para MEI, `3` para Normal). |
| `SEFAZ_486` | 486 | Falta CNPJ Contador (BA) | A API injeta automaticamente o CNPJ `13937073000156` da SEFAZ Bahia no `autXML`. |
| `SEFAZ_554` | 554 | IE informada como ISENTO | O emitente deve informar sua Inscrição Estadual ativa (consulte no Sintegra/CCC). |
| `SEFAZ_204` | 204 | Duplicidade de NF-e | A nota já foi autorizada na SEFAZ. Não emita novamente; consulte pelo ref ou ID. |
| `INVALID_CANCEL_REASON` | - | Justificativa curta | Informe justificativa entre 15 e 255 caracteres. |
| `INVALID_DOCUMENT_STATE` | - | Cancelar nota não autorizada | Somente notas em estado `authorized` podem ser canceladas. |
| `UNAUTHORIZED` | - | Token inválido | Verifique o cabeçalho `Authorization: Bearer af_...`. |

---

## 8. Exemplos Práticos de Código

### Exemplo em Node.js (TypeScript / JavaScript):
```javascript
const axios = require('axios');

const client = axios.create({
  baseURL: 'https://allsparkfiscal.allsparkhub.com',
  headers: {
    'Authorization': 'Bearer af_cBKACw1-xFvCspPFWzQLQmb2huLooVtWkavh4vTygE4',
    'Content-Type': 'application/json'
  }
});

async function emitirNota() {
  const pedido = {
    company_id: '85699d35-0515-423c-ad09-20b3eaeb4f73',
    ref: 'PEDIDO-NODE-001',
    operation: 'issue',
    webhook_url: 'https://meu-erp.com/webhook',
    tax_profile: 'PADRAO_SIMPLES',
    customer: {
      name: 'CLIENTE TESTE LTDA',
      tax_id: '11144477735',
      ie_indicator: 9,
      address: {
        street: 'Av Paulista', number: '1000', district: 'Bela Vista',
        city: 'Sao Paulo', state: 'SP', postal_code: '01310100', city_code: '3550308'
      }
    },
    items: [
      { sku: 'ITEM-1', description: 'LICENCA DE SOFTWARE ANUAL', ncm: '85234990', quantity: '1.0000', price: '1200.00' }
    ],
    payment_method: 'pix'
  };

  const response = await client.post('/v1/orders/invoice', pedido);
  console.log('Nota enviada com sucesso! ID:', response.data.data.id);
}

emitirNota();
```

### Exemplo em Python:
```python
import requests

API_URL = "https://allsparkfiscal.allsparkhub.com"
HEADERS = {
    "Authorization": "Bearer af_cBKACw1-xFvCspPFWzQLQmb2huLooVtWkavh4vTygE4",
    "Content-Type": "application/json"
}

def consultar_nota(ref: str):
    res = requests.get(f"{API_URL}/v1/documents/ref/{ref}", headers=HEADERS)
    data = res.json()["data"]
    print(f"Status da Nota: {data['status']}")
    if data["status"] == "authorized":
        print(f"Chave de Acesso: {data['access_key']}")
        print(f"Protocolo SEFAZ: {data['result']['protocol']['number']}")

def baixar_danfe_pdf(ref: str, output_path: str):
    res = requests.get(f"{API_URL}/v1/documents/ref/{ref}/pdf", headers=HEADERS)
    with open(output_path, "wb") as f:
        f.write(res.content)
    print(f"DANFE PDF salvo em {output_path}")

consultar_nota("SILAS-SEFAZ-ISSUE-004")
```

---

## 9. Suporte e Contato

* **Documentação Oficial:** [github.com/devsistem4s/Allsparkfiscal](https://github.com/devsistem4s/Allsparkfiscal)
* **Status do Ambiente:** `https://allsparkfiscal.allsparkhub.com/healthz`
* **AllSpark Hub** · SPOLTEC
