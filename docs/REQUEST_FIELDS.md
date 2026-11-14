# Guia de requisições, destinatário, tipos e estados

Este guia descreve a API implementada. Referências: [todas as rotas](API.md), [cadastro e configuração](COMPANY_API.md), [OpenAPI](../openapi/openapi.yaml) e [JSON completo de nota](../examples/document.json).

## Empresa emitente e cliente destinatário

`POST /v1/companies` cadastra a **empresa que emite** a nota. Não cadastra o comprador. Cada empresa pertence ao tenant identificado pelo Bearer token.

**Ainda não existe endpoint `/v1/customers` nem cadastro independente de clientes.** Os dados do comprador são enviados em `invoice.recipient` em cada nota e ficam no conteúdo do documento. Não existe `customer_id` neste contrato. O ERP pode manter seu cadastro de clientes e preencher esse objeto.

Exemplo do cadastro da empresa:

```json
{"cnpj":"11222333000181","name":"EMPRESA DE TESTE","uf":"BA","ie":"12345678","crt":1,"address":{"street":"RUA TESTE","number":"100","district":"CENTRO","city_code":"2927408","city":"SALVADOR","state":"BA","postal_code":"40020000"}}
```

| Campo | Contrato atual |
|---|---|
| cnpj | 14 caracteres sem máscara; dígitos verificadores conferidos; aceita formato alfanumérico maiúsculo |
| name | Razão social, 2 a 120 bytes; espaços externos removidos |
| uf | Uma das 27 siglas abaixo; maiúscula |
| ie | 2 a 14 dígitos ou `ISENTO`; somente formato, sem validação estadual completa |
| crt | Inteiro entre 1 e 4; deve corresponder ao regime da empresa; aceitar o número não comprova cobertura tributária desse regime |

A resposta devolve `id`, dados cadastrais, `created_at`, `updated_at` e `archived_at`. Use `id` como `company_id`. CNPJ e UF não podem ser alterados por PATCH.

## Tipos de documento e ambiente

| kind | Modelo | Estado da implementação |
|---|---|---|
| nfe | 55 | Geração/assinatura/XSD/PDF e adaptador de transmissão; homologação real pendente |
| nfce | 65 | Caminho implementado; exige CSC e identificador CSC; homologação real pendente |
| nfse | Serviço | Futuro; requisição de documento é recusada |

Consultar `GET /v1/document-types` e `GET /v1/capabilities`. A presença no catálogo não comprova credenciamento ou disponibilidade do autorizador.

`environment` aceita somente `homologation`. Produção é recusada. `operation=generate` cria uma prévia assíncrona e não reserva número fiscal. `operation=issue` solicita transmissão e exige habilitação global, configuração fiscal da empresa e certificado válido. `202` confirma aceitação do trabalho, não autorização da nota.

## Estados brasileiros

O cadastro aceita as 27 UFs: AC, AL, AP, AM, BA, CE, DF, ES, GO, MA, MT, MS, MG, PA, PB, PR, PE, PI, RJ, RN, RS, RO, RR, SC, SP, SE e TO.

- `GET /v1/states` retorna o catálogo completo das 27 Unidades Federativas e o suporte nacional do motor NFePHP (`v5.2.11`).
- Empresa: `uf`, utilizada pelo motor para selecionar o autorizador estadual ou virtual (`SVAN`, `SVRS`) da UF do emitente.
- Endereço: `state`; no XML do emitente, o motor usa a UF cadastrada da empresa.
- Município: `city_code`, como string do código IBGE de 7 dígitos; exemplo de Salvador: `2927408`.
- CEP: `postal_code`, como string de 8 dígitos sem máscara.

## Modo Simplificado Nacional ("Smart Defaults")

Para emitir ou gerar notas para **qualquer estado do Brasil** enviando apenas os dados essenciais da venda, a API suporta preenchimento inteligente automático baseado no cadastro da empresa emitente (`company_id`):

| Campo em `invoice` | Se omitido pelo integrador |
|---|---|
| `nature` | Preenchido com `"VENDA DE MERCADORIA"` |
| `direction` | `1` (Saída) |
| `destination` | Calculado automaticamente comparando `company.uf` com `recipient.address.state` (`1` = Operação Interna, `2` = Interestadual, `3` = Exterior se `EX`) |
| `city_code` | Herdado automaticamente de `company.address.city_code` |
| `presence` | `1` (Operação presencial) |
| `freight_mode` | `9` (Sem ocorrência de transporte) |
| `recipient` | Opcional em `nfce` (consumidor anônimo); obrigatório em `nfe` |
| `items[i].sku` | Gerado automaticamente (`ITEM-1`, `ITEM-2`, ...) |
| `items[i].unit` | `"UN"` |
| `items[i].cfop` | `"5102"` em operação interna (`destination=1`) ou `"6102"` em operação interestadual (`destination=2`) |
| `items[i].total` | Calculado automaticamente como `quantity × unit_price` |
| `items[i].taxes` | Preenchido automaticamente conforme `company.crt`: Simples Nacional/MEI (`CSOSN 102`, `PIS 07`, `COFINS 07`) ou Regime Normal (`ICMS CST 41`, `PIS 07`, `COFINS 07`) |
| `payments` | Preenchido automaticamente com `[{"method":"01","amount":"<total líquido da nota>"}]` |

Veja os exemplos prontos em [examples/document-simplified.json](../examples/document-simplified.json) e [examples/document-rtc-difal.json](../examples/document-rtc-difal.json).

## Corpo da requisição de documento

| Campo | Conteúdo |
|---|---|
| company_id | UUID da empresa pertencente ao tenant autenticado |
| kind | `nfe` ou `nfce` |
| operation | `generate` ou `issue` |
| environment | `homologation` |
| series | Inteiro de 0 a 889; em emissão deve coincidir com a configuração fiscal |
| invoice | Objeto descrito abaixo; até 1 MiB |

Enviar explicitamente todos os campos do contrato. O servidor não interpreta `X-Company-ID`. `POST /v1/documents` exige `Idempotency-Key` de 8 a 128 caracteres. Mesma chave e conteúdo retornam o documento existente; mesma chave e conteúdo diferente retornam 409.

### invoice

| Campo | Uso no contrato |
|---|---|
| nature | Natureza da operação, como `VENDA` |
| direction | Inteiro 0/1, mapeado para `tpNF` |
| destination | Inteiro 1/2/3, mapeado para `idDest` |
| city_code | Município do fato gerador, mapeado para `cMunFG` |
| final_consumer | Booleano, mapeado para `indFinal` |
| presence | Inteiro, mapeado para `indPres`; exemplo usa 1 |
| issuer_address | Legado/opcional; ignorado na geração, que usa company.address |
| recipient | Dados do destinatário da nota |
| items | De 1 a 990 produtos |
| payments | De 1 a 100 pagamentos |
| freight_mode | Inteiro mapeado para `modFrete`; exemplo usa 9 |
| additional_info | Texto opcional de informações complementares |

Os códigos fiscais são enviados pelo integrador. Nem todas as combinações são validadas na entrada; o motor e o autorizador aplicam validações adicionais. Use os exemplos como teste, não como definição tributária para operações reais.

### Endereços e destinatário

Endereço contém `street`, `number`, `district`, `city_code`, `city`, `state` e `postal_code`, todos strings. O mapeamento atual usa país Brasil e não implementa destinatário estrangeiro completo.

`recipient` contém:

- `tax_id`: CPF/CNPJ sem máscara. O mapeamento usa CPF quando há 11 caracteres e CNPJ nos demais casos; a validação de entrada ainda não faz conferência completa do identificador do destinatário.
- `name`: nome/razão social; o exemplo de homologação usa texto sem valor fiscal.
- `ie_indicator`: inteiro enviado como `indIEDest`.
- `ie`: inscrição estadual opcional, conforme o cenário.
- `address`: endereço do destinatário.

### Produtos, impostos, DIFAL e Reforma Tributária (IBS, CBS e IS)

Cada produto contém `sku`, `description`, `ncm` (8 dígitos), `cfop` (4 dígitos), `unit`, `quantity`, `unit_price`, `total`, opcionais de valores acessórios (`discount`, `freight`, `insurance`, `other_expenses`, `cest`) e `taxes`. `gtin` é opcional; quando ausente o motor utiliza `SEM GTIN`.

Valores monetários e quantidades são **strings decimais**, com ponto e sem separador de milhares. Quantidade deve ser positiva. `total` permite até duas casas decimais e deve coincidir com quantidade × preço arredondado a duas casas (ou pode ser omitido para cálculo automático). A soma dos pagamentos deve coincidir com o total líquido da nota (`vNF = vProd - vDesc + vFrete + vSeg + vOutro + vIPI`).

`taxes` suporta cobertura nacional completa e a Reforma Tributária do Consumo (NT 2025.002):
- `icms`: Regime Normal (`cst` `00`, `10`, `20`, `30`, `40`, `41`, `50`, `51`, `60`, `70`, `90`) ou Simples Nacional/MEI (`csosn` `101`, `102`, `103`, `201`, `202`, `203`, `300`, `400`, `500`, `900`), com `origin`, `base`, `rate`, `amount`, `red_bc`, `st_base`, `st_rate`, `st_amount`, `st_mva` e `credit_sn`.
- `icms_uf_dest` (**DIFAL interestadual**): `base`, `fcp_base`, `fcp_rate`, `fcp_amount`, `dest_rate`, `inter_rate` (`4.00`, `7.00` ou `12.00`), `share_rate` (default `100.00`), `dest_amount` (calculado automaticamente se omitido) e `origin_amount`.
- `ipi`: `cst` (`00`, `49`, `50`, `51`, `52`, `53`, `54`, `55`, `99`), `code` (default `999`), `base`, `rate`, `amount`.
- `pis` e `cofins`: `cst` (`01`, `02`, `04` a `09`, `49`, `99`), `base`, `rate`, `amount`.
- `ibs_cbs` (**Reforma Tributária — IBS e CBS**, grupo `IBSCBS` + totalizador `IBSCBSTot`): `cst`, `class_trib` (6 dígitos), `base`, `ibs_uf_rate`, `ibs_uf_amount`, `ibs_mun_rate`, `ibs_mun_amount`, `cbs_rate`, `cbs_amount` (com cálculo automático de valores se informados `base` e alíquotas).
- `is` (**Imposto Seletivo**, grupo `IS` + totalizador `ISTot`): `cst`, `class_trib`, `base`, `rate`, `amount`.

Cada pagamento contém `method` (string do código enviado como `tPag`) e `amount` (string decimal). O exemplo usa `01` e `10.00`.

### Gestão completa pós-emissão

- Download direto: `GET /v1/documents/{id}/xml` e `GET /v1/documents/{id}/pdf`
- Status SEFAZ da UF: `GET /v1/companies/{id}/sefaz/status?kind=nfe`
- Consulta de chave na SEFAZ: `POST /v1/documents/{id}/query`
- Cancelamento (Evento `110111`): `POST /v1/documents/{id}/cancel`
- Carta de Correção Eletrônica (Evento `110110`): `POST /v1/documents/{id}/correction-letter`
- Inutilização de numeração: `POST /v1/companies/{id}/void-range`
- Distribuição DF-e Nacional: `POST /v1/companies/{id}/dfe/sync`

## Validar, gerar e acompanhar

1. Cadastrar empresa e guardar o UUID.
2. Cadastrar A1 em `PUT /v1/companies/{id}/certificate`; nunca colocar certificado/senha na requisição da nota.
3. Para emissão, configurar `PUT /v1/companies/{id}/fiscal/nfe` ou `/nfce` com `{"environment":"homologation","series":1,"enabled":true}`.
4. Copiar `examples/document.json`, substituir `company_id` e preencher os dados reais do cenário de teste.
5. Enviar para `POST /v1/documents/validate`: valida entrada sem criar documento ou reservar número. Usa o mesmo corpo; se `operation=issue`, também exige habilitação global de emissão. Não verifica toda a configuração/certificado nem executa XSD.
6. Enviar para `POST /v1/documents` com chave de idempotência. Consultar o `status_url` recebido.
7. Baixar pelos `download_url` dos artefatos disponíveis, usando Bearer token.

Exemplo de validação, a partir da raiz do projeto:

```sh
curl http://127.0.0.1:8088/v1/documents/validate \
  -H "Authorization: Bearer $ALLSPARK_API_KEY" \
  -H 'Content-Type: application/json' \
  --data-binary @examples/document.json
```

Substituir o UUID antes de executar. O mesmo arquivo pode ser enviado para `/v1/documents`, acrescentando o header `Idempotency-Key`.

## Estados de processamento

| status | Significado |
|---|---|
| queued | Persistido e aguardando processamento |
| processing | Worker preparando ou consultando |
| validated | Prévia gerada pelo motor; não autorizada |
| transmitting | Transmissão iniciada com XML previamente persistido |
| authorized | Autorização fiscal reconhecida pelo adaptador |
| rejected | Rejeição fiscal reconhecida pelo adaptador |
| reconciliation_required | Resultado incerto; usar POST /v1/documents/{id}/reconcile para consulta |
| failed | Falha local terminal; consultar erros |

Nem todo documento terá todos os arquivos. Artefatos possíveis: `signed_xml`, `preview_pdf`, `protocol_xml`, `authorized_xml`, `danfe_pdf`. Falha de geração do PDF pode ser registrada mesmo com XML disponível. Não reenviar uma emissão incerta com outra chave: use reconciliação.

Respostas e erros seguem o envelope descrito em [API.md](API.md). Listagens e cadastro do certificado, escopos, paginação, desativação e bloqueios de empresa estão detalhados em [COMPANY_API.md](COMPANY_API.md).

## Fluxo multi-tenant atualizado

Consulte [identidade, cadastro completo e credenciais somente por código](TENANT_FLOW.md). O cadastro de empresa agora exige `address`; o endereço do emitente é carregado desse cadastro. `invoice.issuer_address` é legado, opcional e ignorado na geração.
