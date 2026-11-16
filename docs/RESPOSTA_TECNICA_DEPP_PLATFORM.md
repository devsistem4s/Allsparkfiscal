# AllSpark Fiscal — Parecer Técnico e Resposta Oficial à DEPP Platform

> **Destinatário:** Equipe de Engenharia e Produto — DEPP Platform  
> **Remetente:** Equipe AllSpark Fiscal  
> **Data:** 15 de novembro de 2026  
> **Referência:** Solicitação de Ajustes na API AllSpark Fiscal (Base Postman v0.4.0)  
> **Status:** Homologado e Validado em Produção na SEFAZ

---

## 0. Confirmações Pedidas

### 0.1 Produção (`environment=production`)
- **Status:** **LIBERADO E DISPONÍVEL IMEDIATAMENTE.**
- **Ambiente:** A AllSpark Fiscal opera com transmissão ativa em dois modos configuráveis por empresa via `PUT /v1/companies/{id}/fiscal/nfe`:
  - `environment: "production"` (`tpAmb=1` — Produção com validade jurídica e comercial)
  - `environment: "homologation"` (`tpAmb=2` — Ambiente de homologação da SEFAZ para testes)
- **UFs Homologadas:** **Todas as 27 UFs do Brasil** (AC, AL, AM, AP, BA, CE, DF, ES, GO, MA, MG, MS, MT, PA, PB, PE, PI, PR, RJ, RN, RO, RR, RS, SC, SE, SP, TO). O motor tributário utiliza comunicação nativa atualizada com os servidores autorizadores estaduais e ambientes nacionais (SVRS, SVAN e SEFAZ autorizadoras próprias como BA, SP, MG, PR, RS, GO, MT, MS).
- **Validação Real:** A API realizou transmissões reais em ambiente de produção com certificado ICP-Brasil A1 próprio na SEFAZ Bahia, incluindo autorização com `cStat 100`, cancelamento com `cStat 135`, download de XML `nfeProc` assinado e DANFE em PDF.

### 0.2 Divergência de Documentação (`COMPANY_API.md`)
- O arquivo `COMPANY_API.md` continha um trecho legado que não refletia a versão corrente. O documento foi corrigido.
- **Recursos 100% ativos em Produção:**
  - ✅ **Cancelamento:** `POST /v1/documents/{id}/cancel` (evento SEFAZ 110111)
  - ✅ **Carta de Correção Eletrônica (CC-e):** `POST /v1/documents/{id}/cce` (evento SEFAZ 110110)
  - ✅ **Inutilização de Numeração:** `POST /v1/companies/{id}/void-range`
  - ✅ **Webhooks de Notificação:** Ativos via mensageria assíncrona RabbitMQ com assinatura HMAC-SHA256 (`X-AllSpark-Signature`) e retentativa exponencial.
  - ✅ **Download de XML (`nfeProc`) e DANFE (PDF):** Disponíveis via endpoints de artefatos.

### 0.3 Limites e Políticas Operacionais
- **Tamanho Máximo do Documento (`invoice`):** Elevado para **2 MiB** por payload JSON (suporta notas de até 990 itens, declarações extensas de importação, rastreabilidade de lotes e anexos).
- **Rate Limits da API:**
  - **Requisições Gerais:** 300 requisições/minuto por token de API (burst tolerado de até 600 req/min).
  - **Lotes em Massa (`POST /v1/documents/bulk`):** Até 50 notas fiscais por requisição (HTTP 207 Multi-Status individualizado).
  - **Distribuição DF-e (`POST /v1/companies/{id}/dfe/sync`):** Regra estrita da NT 2014.002 com proteção anti-bloqueio SEFAZ (trava automática de 61 minutos em cache Redis caso a SEFAZ retorne `cStat 137` - "Nenhum documento localizado" ou `cStat 656` - "Consumo indevido").
  - **Consulta de Protocolo por Chave:** Máximo de 8 consultas/hora por chave (NT 2018.002).
- **Política de Retentativas e Confiabilidade:**
  - Fila persistente com RabbitMQ e Dead-Letter Queue (DLQ).
  - Erros de infraestrutura / instabilidade temporária da SEFAZ (`retryable: true`) sofrem backoff exponencial (10s, 30s, 1m, 5m, 15m).
  - Erros de negócio ou rejeição de regras fiscais da SEFAZ (ex: `cStat 204`, `539`, `215`) são determinísticos (`retryable: false`); a nota assume status `rejected` imediatamente e o webhook é disparado com código e motivo da SEFAZ.

---

## 1. DF-e: Notas Recebidas (Distribuição por NSU & MDe)

### 1.1 Sincronização e Listagem Paginada
O fluxo de busca nacional via `sefazDistDFe` opera em segundo plano ou sob demanda:

- **Disparo de Sincronização:** `POST /v1/companies/{id}/dfe/sync`
  - Body opcional: `{"last_nsu": 0, "max_documents": 50}`
  - Resposta `202 Accepted` com `sync_id`:
  ```json
  {
    "data": {
      "sync_id": "9b1deb4d-3b7d-4bad-9bdd-2b0d7b3dcb6d",
      "status": "processing",
      "requested_at": "2026-11-15T21:00:00Z"
    }
  }
  ```

- **Listagem Paginada:** `GET /v1/companies/{id}/dfe/documents?since_nsu=&status=&from=&to=&cursor=`
  - Retorno em exata conformidade com a estrutura solicitada pelo DEPP:
  ```json
  {
    "data": {
      "items": [
        {
          "access_key": "29261163687210000110550010000000041236372404",
          "nsu": "000000000000123",
          "model": "55",
          "issuer": {
            "tax_id": "13937073000156",
            "name": "DISTRIBUIDORA BAHIA LTDA",
            "ie": "123456789"
          },
          "issued_at": "2026-11-10T10:00:00-03:00",
          "total": "1234.56",
          "status": "authorized",
          "manifest_status": "ciencia",
          "xml_available": true
        }
      ],
      "next_cursor": "eyJpZCI6MTIzfQ==",
      "last_nsu": "000000000000123"
    }
  }
  ```

- **CT-e (Modelo 57):** Suportado no mesmo endpoint. O campo `model` retorna `"57"` e o parser extrai o tomador/remetente e valores do XML `procCTe`.

### 1.2 Download de XML e Busca por Chave
- `GET /v1/companies/{id}/dfe/{access_key}`: Busca documento específico por chave de 44 dígitos.
- `GET /v1/companies/{id}/dfe/{access_key}/xml`:
  - Se o XML completo estiver disponível (`nfeProc` / `procCTe`): Retorna `200 OK` com `Content-Type: application/xml`.
  - Se apenas o resumo estiver disponível (`resNFe`): Retorna `409 Conflict`:
  ```json
  {
    "error": {
      "code": "DFE_XML_PENDING_MANIFEST",
      "message": "Apenas o resumo (resNFe) está disponível. Registre a manifestação de 'ciencia' ou 'confirmacao' para a SEFAZ disponibilizar o XML completo.",
      "action": "Execute POST /v1/companies/{id}/dfe/{access_key}/manifest com type='ciencia'"
    }
  }
  ```

### 1.3 Manifestação do Destinatário (MDe)
- Endpoint: `POST /v1/companies/{id}/dfe/{access_key}/manifest`
- **Valores aceitos de `type` e mapeamento SEFAZ:**
  - `ciencia` ➔ Evento `210210` (Ciência da Emissão)
  - `confirmacao` ➔ Evento `210200` (Confirmação da Operação)
  - `desconhecimento` ➔ Evento `210220` (Desconhecimento da Operação)
  - `nao_realizada` ➔ Evento `210240` (Operação não Realizada)
- **Regra do campo `justification`:**
  - **Obrigatório** para `desconhecimento` e `nao_realizada`.
  - Validação de tamanho: Mínimo de 15 e máximo de 255 caracteres (regra de rejeição da SEFAZ).
  - Opcional/ignorado para `ciencia` e `confirmacao`.

---

## 2. Novos Campos em `invoice`

### 2.1 Finalidades (`purpose`) e Documentos Referenciados
- **Valores de `purpose` suportados:**
  - `1`: Normal
  - `2`: Complementar (exige pelo menos 1 documento referenciado)
  - `3`: Ajuste
  - `4`: Devolução / Retorno de Mercadorias (exige obrigatoriamente referência à nota de origem)
- **Estrutura de `referenced_documents[]`:**
  Permite referenciar NF-e, NF de Produtor Rural (papel), Cupom Fiscal (ECF) e CT-e:
  ```json
  "referenced_documents": [
    {
      "type": "nfe",
      "access_key": "29261163687210000110550010000000041236372404"
    },
    {
      "type": "nfp",
      "state": "BA",
      "year_month": "2610",
      "tax_id": "12345678901",
      "ie": "123456789",
      "model": "04",
      "series": 1,
      "number": 1234
    },
    {
      "type": "cf",
      "model": "2D",
      "ecf_number": 1,
      "coo": 123456
    },
    {
      "type": "cte",
      "access_key": "29261163687210000110570010000000041236372404"
    }
  ]
  ```

---

### 2.2 NF-e de Entrada Emitida pela Própria Empresa (`direction=0`)
No leiaute da SEFAZ:
- **Emitente (`emit`):** É sempre a empresa cadastrada no AllSpark (seu cliente no ERP).
- **Destinatário (`dest`):** É a contraparte da operação informada em `recipient`:
  - Se for **compra de produtor rural:** CPF + IE de produtor rural.
  - Se for **retorno/devolução de pessoa física:** CPF sem IE (`ie_indicator: 9`).
  - Se for **importação de mercadorias:** Nome da empresa estrangeira no exterior, sem CNPJ/CPF (`tax_id: ""`), país exterior (`country_code: "..."`).
- **Campos fiscais automáticos:**
  - `ide.tpNF`: Fixado em `0` (Entrada).
  - `ide.idDest`: Calculado automaticamente conforme o endereço do remetente (`1` para mesma UF, `2` para outra UF, `3` para exterior).
  - `ide.indFinal`: Ajustado conforme o perfil da contraparte.

#### Exemplo Completo — NF-e de Entrada por Devolução de Consumidor / Compra:
```json
{
  "company_id": "85699d35-0515-423c-ad09-20b3eaeb4f73",
  "ref": "DEV-CLIENTE-9912",
  "kind": "nfe",
  "operation": "issue",
  "invoice": {
    "nature": "DEVOLUCAO DE VENDA",
    "direction": 0,
    "purpose": 4,
    "referenced_documents": [
      {
        "type": "nfe",
        "access_key": "29261163687210000110550010000000041236372404"
      }
    ],
    "recipient": {
      "tax_id": "01234567890",
      "name": "JOAO DA SILVA",
      "ie_indicator": 9,
      "address": {
        "street": "AVENIDA PAULISTA",
        "number": "1000",
        "district": "BELA VISTA",
        "city": "SAO PAULO",
        "state": "SP",
        "postal_code": "01310100",
        "city_code": "3550308"
      }
    },
    "items": [
      {
        "sku": "PROD-001",
        "description": "PRODUTO DEVOLVIDO EM GARANTIA",
        "ncm": "84713012",
        "cfop": "2202",
        "unit": "UN",
        "quantity": "1.0000",
        "unit_price": "1500.00",
        "total": "1500.00",
        "taxes": {
          "icms": {
            "origin": 0,
            "cst": "00",
            "base": "1500.00",
            "rate": "12.00",
            "amount": "180.00"
          },
          "pis": { "cst": "01", "base": "1500.00", "rate": "1.65", "amount": "24.75" },
          "cofins": { "cst": "01", "base": "1500.00", "rate": "7.60", "amount": "114.00" }
        }
      }
    ],
    "payments": [
      { "method": "90", "amount": "0.00" }
    ]
  }
}
```

---

### 2.3 Importação de Mercadorias (`import`, `taxes.ii`)
- **Grupo `DI/adi`:** O motor gera as tags `<DI>` e `<adi>` no item correspondente da NF-e.
- **Grupo `II` (Imposto de Importação):** Mapeado diretamente para `<II>` no XML.
- **PIS / COFINS Importação:** Aceita bases e alíquotas de importação.
- **Origem da Mercadoria (`origin`):** Aceita `1` (Estrangeira - Importação direta), `2` (Estrangeira - Adquirida no mercado interno), `3`, `6`, `7`, `8`.

#### Exemplo de Payload de Importação:
```json
"invoice": {
  "direction": 0,
  "nature": "IMPORTACAO DE MERCADORIAS",
  "import": {
    "declarations": [
      {
        "number": "26/0001234-5",
        "registered_at": "2026-11-01",
        "clearance_location": "Porto de Salvador",
        "clearance_state": "BA",
        "cleared_at": "2026-11-05",
        "transport_via": "01",
        "intermediation": "1",
        "exporter_code": "EXP-001",
        "acquirer_tax_id": "63687210000110",
        "acquirer_state": "BA"
      }
    ]
  },
  "items": [
    {
      "sku": "IMP-001",
      "description": "COMPONENTE ELETRONICO INDUSTRIAL",
      "ncm": "85423190",
      "cfop": "3102",
      "unit": "UN",
      "quantity": "100.0000",
      "unit_price": "50.00",
      "total": "5000.00",
      "import_additions": [
        {
          "number": 1,
          "sequence": 1,
          "manufacturer_code": "FAB-1",
          "discount": "0.00",
          "drawback": ""
        }
      ],
      "taxes": {
        "icms": { "origin": 1, "cst": "00", "base": "5620.00", "rate": "18.00", "amount": "1011.60" },
        "ii": {
          "base": "5000.00",
          "customs_expenses": "120.00",
          "amount": "500.00",
          "iof": "0.00"
        },
        "pis": { "cst": "01", "base": "5000.00", "rate": "2.10", "amount": "105.00" },
        "cofins": { "cst": "01", "base": "5000.00", "rate": "9.65", "amount": "482.50" }
      }
    }
  ]
}
```

---

### 2.4 Transporte (`transport`)
Suporte completo aos grupos `<transp>`, `<transporta>`, `<veicTransp>` e `<vol>`:
```json
"transport": {
  "mode": 0,
  "carrier": {
    "tax_id": "13937073000156",
    "name": "TRANSPORTADORA BRASIL LOGISTICA LTDA",
    "ie": "123456789",
    "address": "RODOVIA BR 324 KM 10",
    "city": "SALVADOR",
    "state": "BA"
  },
  "vehicle": {
    "plate": "ABC1D23",
    "state": "BA",
    "rntc": "12345678"
  },
  "volumes": [
    {
      "quantity": 3,
      "species": "CAIXA",
      "brand": "ALLSPARK",
      "number": "1-3",
      "net_weight": "10.500",
      "gross_weight": "11.200",
      "seals": ["LCR-9901", "LCR-9902"]
    }
  ]
}
```

---

### 2.5 Cobrança e Fatura (`billing`)
Suporte aos grupos `<cobr>`, `<fat>` e `<dup>`:
```json
"billing": {
  "invoice": {
    "number": "9001",
    "original": "1234.56",
    "discount": "0.00",
    "net": "1234.56"
  },
  "installments": [
    { "number": "001", "due_date": "2026-12-10", "amount": "617.28" },
    { "number": "002", "due_date": "2027-01-10", "amount": "617.28" }
  ]
}
```

---

### 2.6 Rastreabilidade de Lotes de Produtos (`lots`)
Gera o grupo `<rastro>` no item:
```json
"lots": [
  {
    "number": "L-2026-11",
    "quantity": "10.0000",
    "manufactured_on": "2026-10-01",
    "expires_on": "2028-10-01",
    "anvisa_code": ""
  }
]
```

---

### 2.7 Intermediador, Remessas e Pedidos do Cliente
- **Intermediador da Operação:** Mapeado em `<infIntermed>` e `ide.indIntermed`:
  ```json
  "intermediary": {
    "indicator": 1,
    "tax_id": "00000000000191",
    "name": "MARKETPLACE HUB"
  }
  ```
- **Remessa, Demonstração e Transferência:** Aceita pagamentos com método `90` ("Sem Pagamento") com valor `0.00`, CFOPs 5.9xx / 6.9xx e `purpose=1`.
- **Referência do Pedido de Compra (`xPed` / `nItemPed`):**
  ```json
  "order_reference": {
    "order_number": "PED-88120",
    "item_number": 1
  }
  ```
- **FCI (Ficha de Conteúdo de Importação):** Campo `"fci": "B01F22D3-3294-43C5-A050-E6CD0E36E8B1"` no item.

---

## 3. Pré-validação Sem Reservar Número (`POST /v1/documents/validate`)

O endpoint foi aprimorado para retornar o formato exato solicitado pelo DEPP, com o array de `issues` apontando o caminho exato do campo no JSON, o código e a ação corretiva:

```json
{
  "data": {
    "valid": false,
    "issues": [
      {
        "path": "invoice.items[0].ncm",
        "code": "NCM_INVALID",
        "message": "NCM deve conter 8 dígitos numéricos válidos.",
        "action": "Corrija o código NCM informado para 8 dígitos numéricos sem formatação."
      },
      {
        "path": "invoice.items[0].taxes.icms.origin",
        "code": "ORIGIN_INVALID",
        "message": "Origem da mercadoria deve estar entre 0 e 8.",
        "action": "Informe a origem conforme a Tabela A de CST do ICMS."
      }
    ]
  }
}
```

---

## 4. Webhooks e Eventos de Notificação

- **Headers enviados em cada webhook:**
  - `X-AllSpark-Signature`: Assinatura HMAC-SHA256 (`hex`) do corpo da requisição usando o secret configurado.
  - `X-AllSpark-Timestamp`: Timestamp UTC ISO-8601 do envio.
  - `X-AllSpark-Event`: Identificador do evento (`document.authorized`, `document.rejected`, `document.canceled`, `dfe.received`, `certificate.expiring`, `company.not_ready`).
  - `X-AllSpark-Event-ID`: UUID exclusivo do disparo.

- **Novos Eventos Cadastrados:**
  1. `dfe.received`: Disparado sempre que uma nova NF-e ou CT-e for identificada para o CNPJ no Ambiente Nacional.
     ```json
     {
       "event": "dfe.received",
       "company_id": "85699d35-0515-423c-ad09-20b3eaeb4f73",
       "data": {
         "access_key": "29261163687210000110550010000000041236372404",
         "nsu": "000000000000123",
         "model": "55",
         "issuer_tax_id": "13937073000156",
         "issuer_name": "DISTRIBUIDORA BAHIA LTDA",
         "total": "1234.56"
       }
     }
     ```
  2. `certificate.expiring`: Alerta preventivo com antecedência de 30, 15, 7 e 1 dia:
     ```json
     {
       "event": "certificate.expiring",
       "company_id": "85699d35-0515-423c-ad09-20b3eaeb4f73",
       "data": {
         "expires_at": "2027-05-21T18:41:40Z",
         "days_remaining": 15
       }
     }
     ```
  3. `company.not_ready`: Notificação quando uma empresa perder prontidão fiscal (ex: certificado expirou, série desabilitada).

---

## 5. Discovery e Versionamento em `GET /v1/capabilities`

O endpoint `GET /v1/capabilities` detalha as flags ativas na API para que o ERP do DEPP possa ativar cada módulo programaticamente:

```json
{
  "data": {
    "version": "1.2.0",
    "features": {
      "dfe": true,
      "dfe_cte": true,
      "import": true,
      "transport": true,
      "billing": true,
      "lots": true,
      "intermediary": true,
      "order_reference": true,
      "fci": true,
      "reforma_tributaria_rtc": true,
      "difal": true,
      "detailed_validation": true
    },
    "environments": ["homologation", "production"],
    "supported_models": [55, 65, 57]
  }
}
```

---

## 6. Acordo de Nível de Serviço (SLA) e Comunicação Técnica

1. **Garantia de Estabilidade:** Todas as alterações são retrocompatíveis. Nenhuma alteração incompatível (breaking change) será publicada sem aviso prévio mínimo de **30 dias** por escrito.
2. **Coleção Postman v0.5.0:** Atualizada com todas as novas pastas, requisições de exemplo e respostas reais de sucesso e erro.
3. **Canal Direto de Engenharia:** Canal aberto para alinhamento entre as equipes técnicas.

---

Equipe de Engenharia **AllSpark Fiscal**
