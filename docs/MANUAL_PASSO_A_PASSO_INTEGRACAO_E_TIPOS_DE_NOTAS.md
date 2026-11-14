# Manual Passo a Passo de Uso, Tipos de Notas Emitidas e Guia de Integração em Outros Sistemas

Este manual explica **como a AllSpark Fiscal funciona de ponta a ponta**, **quantos e quais tipos de notas e operações nosso sistema emite** e o **passo a passo prático para integrar em qualquer sistema** (E-commerce, PDV/Varejo, ERP, SaaS Multi-empresa ou Agentes de IA).

---

## 1. Quantos e Quais Tipos de Notas Nosso Sistema Emite?

Hoje, a AllSpark Fiscal emite **2 Modelos Oficiais Nacionais SEFAZ de Mercadorias/Varejo** em **todas as 27 Unidades Federativas do Brasil**, cobrindo **10 modalidades operacionais** e **5 eventos fiscais oficiais**, já **100% atualizados para a Reforma Tributária (`PL_010_V1.30` — IBS, CBS e Imposto Seletivo)**:

### 1.1. Modelos de Documentos Fiscais (`kind`)
| Modelo SEFAZ | Campo `kind` | Nome Oficial | Onde é utilizado | Status |
|---|---|---|---|---|
| **Modelo 55** | `"nfe"` | **NF-e (Nota Fiscal Eletrônica)** | E-commerce, Indústria, Atacado, Distribuidoras, Vendas B2B e B2C, Interestadual, Devoluções, Remessas e Exportação. | **100% Pronto (27 UFs)** |
| **Modelo 65** | `"nfce"` | **NFC-e (Nota Fiscal de Consumidor Eletrônica)** | Varejo presencial, Frente de Caixa (PDV), Restaurantes, Farmácias, Supermercados (com QR Code e CSC). | **100% Pronto (27 UFs)** |
| **Municipal / ADN** | `"nfse"` | **NFS-e (Nota Fiscal de Serviços Eletrônica)** | Prestação de serviços municipais (ISS). | *Mapeado no Roadmap* |

### 1.2. Modalidades e Finalidades de Notas Suportadas (`purpose`, `direction`, `destination`)
Dentro da **NF-e (Modelo 55)** e **NFC-e (Modelo 65)**, o sistema emite todas as variações exigidas pela operação comercial:

1. **Nota Fiscal Normal de Venda (`purpose: 1`, `direction: 1`)**:
   - Venda de produção própria (`CFOP 5101`/`6101`) ou revenda de mercadoria (`CFOP 5102`/`6102`).
2. **Nota Fiscal Interestadual com DIFAL (`destination: 2`, `icms_uf_dest`)**:
   - Venda para consumidor final em outro estado (`CFOP 6108`) com cálculo automático de partilha de ICMS e Fundo de Combate à Pobreza (`FCP`).
3. **Nota Fiscal com Reforma Tributária 2026 (`ibs_cbs` e `is`)**:
   - Geração e validação das novas tags `<IBSCBS>`, `<IBSCBSTot>`, `<IS>` e `<ISTot>` (IBS Estadual, IBS Municipal, CBS Federal e Imposto Seletivo).
4. **Nota Fiscal de Devolução de Mercadoria (`purpose: 4` + `referenced_keys`)**:
   - Devolução de compra ou venda (`CFOP 5202`, `6202`, `1202`, `2202`), referenciando a chave de 44 dígitos da nota original em `"referenced_keys"`.
5. **Nota Fiscal Complementar (`purpose: 2` + `referenced_keys`)**:
   - Complemento de preço, quantidade ou imposto (ICMS/IPI) vinculado à nota original.
6. **Nota Fiscal de Ajuste (`purpose: 3`)**:
   - Ajustes de escrituração fiscal e transferência de créditos.
7. **Nota Fiscal de Entrada (`direction: 0`)**:
   - Entrada de mercadoria no estoque (ex.: compra de produtor rural, devolução de cliente PF, importação).
8. **Nota Fiscal de Exportação / Comércio Exterior (`destination: 3`, UF `EX`)**:
   - Vendas para o exterior (`CFOP 7101`/`7102`).
9. **Nota Simplificada de Pedido (`POST /v1/orders/invoice`)**:
   - O sistema recebe apenas os itens comerciais e o cliente, e escolhe automaticamente `nfe` ou `nfce`, `CFOP`, `CSOSN`/`CST` (ou aplica o `tax_profile` da empresa) e calcula os totais.

### 1.3. Eventos Fiscais e Obrigações Acessórias Suportados
| Evento / Operação |Código SEFAZ | Endpoint | Descrição |
|---|---|---|---|
| **Cancelamento** | `110111` | `POST /v1/documents/{id}/cancel` (ou `/ref/{ref}/cancel`) | Cancela uma NF-e ou NFC-e autorizada dentro do prazo legal. |
| **Carta de Correção (CC-e)** | `110110` | `POST /v1/documents/{id}/correction-letter` (ou `/ref/{ref}/correction-letter`) | Corrige dados cadastrais/transporte de uma NF-e (até 20 cartas por nota). |
| **Inutilização de Numeração** | `sefazInutiliza` | `POST /v1/companies/{id}/void-range` | Inutiliza faixa de números que foram pulados na série. |
| **Manifestação do Destinatário (MDe)** | `210200` a `210240` | `POST /v1/companies/{id}/dfe/{access_key}/manifest` | Ciência da Operação, Confirmação, Desconhecimento ou Operação Não Realizada. |
| **Distribuição DF-e** | `sefazDistDFe` | `POST /v1/companies/{id}/dfe/sync` | Busca notas emitidas contra o CNPJ da empresa na SEFAZ Nacional. |

---

## 2. Passo a Passo: Como Usar a API (Do Zero à Primeira Nota)

### Passo 1 — Cadastrar a Empresa Emitente (`POST /v1/companies`)
Você cadastra o CNPJ, Inscrição Estadual (`ie`), Regime Tributário (`crt`: `1` = Simples Nacional, `3` = Regime Normal), endereço com código IBGE (`city_code`) e opcionalmente o logotipo em base64 (`logo_base64`) para sair impresso no DANFE PDF:

```bash
curl -X POST http://127.0.0.1:8088/v1/companies \
  -H "Authorization: Bearer af_sua_chave" \
  -H "Content-Type: application/json" \
  -d '{
    "cnpj": "11222333000181",
    "name": "MINHA EMPRESA COMERCIAL LTDA",
    "uf": "BA",
    "ie": "12345678",
    "crt": 1,
    "contingency_mode": "auto",
    "address": {
      "street": "Av Tancredo Neves",
      "number": "1000",
      "district": "Caminho das Arvores",
      "city_code": "2927408",
      "city": "Salvador",
      "state": "BA",
      "postal_code": "41820021"
    }
  }'
```
> Guarde o `data.id` retornado (`company_id`).

### Passo 2 — Configurar a Série e o Ambiente (`PUT /v1/companies/{id}/fiscal/nfe`)
Habilite a série (ex.: série `1`) em `homologation` (testes) ou `production`:

```bash
curl -X PUT http://127.0.0.1:8088/v1/companies/{company_id}/fiscal/nfe \
  -H "Authorization: Bearer af_sua_chave" \
  -H "Content-Type: application/json" \
  -d '{
    "environment": "homologation",
    "series": 1,
    "enabled": true
  }'
```

### Passo 3 — Enviar o Certificado Digital A1 (`POST /v1/companies/{id}/certificates`)
Converta o arquivo `.pfx` ou `.p12` da empresa para Base64 e envie junto com a senha. O certificado é validado (CNPJ, validade e cadeia ICP-Brasil) e salvo criptografado com AES-256-GCM:

```bash
curl -X POST http://127.0.0.1:8088/v1/companies/{company_id}/certificates \
  -H "Authorization: Bearer af_sua_chave" \
  -H "Content-Type: application/json" \
  -d '{
    "pfx_base64": "MII...",
    "password": "senha-do-certificado"
  }'
```

### Passo 4 — Verificar se a Empresa Está 100% Pronta (`GET /v1/companies/{id}/readiness`)
Antes de colocar em produção, chame o diagnóstico de prontidão:
```bash
curl -H "Authorization: Bearer af_sua_chave" \
  http://127.0.0.1:8088/v1/companies/{company_id}/readiness
```
Se `data.ready_for_nfe` for `true` e `pending_steps` estiver vazio (`null`), a empresa está 100% pronta!

### Passo 5 — Emitir a Nota Fiscal com `ref` e `webhook_url` (`POST /v1/orders/invoice`)
Envie os dados do pedido com o ID do pedido no seu sistema (`ref`) e a URL do seu sistema (`webhook_url`) para ser avisado quando a nota for aprovada:

```bash
curl -X POST http://127.0.0.1:8088/v1/orders/invoice \
  -H "Authorization: Bearer af_sua_chave" \
  -H "Content-Type: application/json" \
  -d '{
    "company_id": "{company_id}",
    "ref": "PEDIDO-1001",
    "operation": "issue",
    "webhook_url": "https://meusistema.com.br/webhooks/nfe",
    "webhook_secret": "meu-segredo-hmac",
    "customer": {
      "tax_id": "11144477735",
      "name": "JOAO DA SILVA",
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
        "sku": "CAMISA-01",
        "description": "CAMISA ALGODAO PREMIUM",
        "ncm": "61091000",
        "quantity": "2.0000",
        "price": "150.00"
      }
    ]
  }'
```

### Passo 6 — Receber o Webhook (ou Consultar por `ref`) e Baixar XML e PDF
Assim que o worker processar a nota, seu `webhook_url` receberá um `POST` assinado (`X-AllSpark-Signature`). Você também pode acessar diretamente usando o seu `ref`:
- **Status e Chave de 44 dígitos**: `GET /v1/documents/ref/PEDIDO-1001`
- **XML Assinado/Autorizado**: `GET /v1/documents/ref/PEDIDO-1001/xml`
- **DANFE em PDF (com Logo)**: `GET /v1/documents/ref/PEDIDO-1001/pdf`

---

## 3. Como Integrar em Outros Sistemas a Depender do Seu Caso de Uso

### Cenário A — E-commerce, Checkout ou Marketplace (Shopify, WooCommerce, VTEX, Próprio)
- **Qual modelo usar?** `kind: "nfe"` (Modelo 55).
- **Como integrar?**
  1. Cadastre um Perfil Tributário (`POST /v1/companies/{id}/tax-profiles`) com as regras padrão dos seus produtos (ex.: `ECOMMERCE_PADRAO`).
  2. Quando o pagamento do pedido `#9842` for aprovado no gateway (Stripe, Mercado Pago, Pagar.me, Asaas), seu backend chama `POST /v1/orders/invoice` passando `"ref": "PEDIDO-9842"`, `"tax_profile": "ECOMMERCE_PADRAO"` e `"webhook_url": "https://sualoja.com/api/webhooks/nfe"`.
  3. Quando seu endpoint `/api/webhooks/nfe` receber o evento `document.authorized`, salve a `access_key` no pedido e envie os links do DANFE PDF (`/v1/documents/ref/PEDIDO-9842/pdf`) e XML para o e-mail/WhatsApp do cliente.

### Cenário B — PDV, Frente de Caixa, Restaurantes e Supermercados
- **Qual modelo usar?** `kind: "nfce"` (Modelo 65).
- **Como integrar?**
  1. Habilite `nfce` na empresa (`PUT /v1/companies/{id}/fiscal/nfce`) e mantenha `contingency_mode: "auto"`.
  2. Na finalização da venda no caixa, chame `POST /v1/orders/invoice` com `"kind": "nfce"` (não precisa de endereço do consumidor; o CPF é opcional).
  3. Baixe o PDF térmico (`GET /v1/documents/ref/{ref}/pdf`) com QR Code pronto para impressão na impressora térmica 80mm.

### Cenário C — ERP, Indústria ou Distribuidora B2B
- **Qual modelo usar?** `kind: "nfe"` (Modelo 55) via `POST /v1/documents` (contrato completo) ou `POST /v1/documents/bulk` (lotes de até 50 notas por requisição).
- **Recursos recomendados para ERPs:**
  - **Validação Prévia no Tela de Faturamento**: chame `POST /v1/orders/preview` ou `POST /v1/documents/validate` enquanto o operador preenche o pedido para exibir os impostos calculados antes de emitir.
  - **Devoluções e Estornos**: envie `purpose: 4` e a chave de 44 dígitos original em `"referenced_keys": ["..."]`.
  - **Notas Emitidas Contra o CNPJ (Compras)**: chame `POST /v1/companies/{id}/dfe/sync` para importar XMLs de fornecedores e `POST /v1/companies/{id}/dfe/{access_key}/manifest` para dar **Ciência da Operação (`ciencia`)** ou **Confirmação (`confirmacao`)**.
  - **Fechamento Contábil no Dia 1º do Mês**: chame `GET /v1/companies/{id}/backups/2026-11.zip` e envie o arquivo `.zip` (contendo todos os XMLs, PDFs e `resumo-contador.json`) direto para o escritório de contabilidade.

### Cenário D — SaaS Multi-Empresa (ERP na Nuvem / Fintech / Plataforma para Múltiplos CNPJs)
- **Como funciona em alta escala (50.000+ notas/dia)?**
  1. Cada cliente novo do seu SaaS que cadastra o CNPJ na sua tela cria um registro em `POST /v1/companies` e envia o certificado `.pfx` em `POST /v1/companies/{id}/certificates`.
  2. Configure um job diário no seu SaaS que consulta `GET /v1/certificates/expiring?days=30` para avisar seus clientes no painel quando o certificado digital A1 deles estiver perto de vencer.
  3. Use a stack Docker Swarm ([infra/docker-stack.yml](file:///home/silane/Documentos/sistema/AllSpark%20Fiscal/infra/docker-stack.yml)) com múltiplos `workers` e o **Escudo Redis Anti-656** para processar dezenas de milhares de notas por dia com isolamento total entre empresas.

