# API executável

Base local: `http://127.0.0.1:8088`. Contrato: [OpenAPI](../api/openapi.json).

```http
Authorization: Bearer API_KEY_DO_TENANT
Content-Type: application/json
Idempotency-Key: pedido-123-nfe
```

O `company_id` está no JSON da operação. `X-Company-ID` está no desenho futuro;
não é interpretado nesta versão. Não enviar `tenant_id`: ele vem da credencial.
IDs são UUID. `X-Request-ID` é gerado pelo servidor e devolvido no header e no JSON.

| Método | Rota | Operação |
|---|---|---|
| GET | `/healthz` | Saúde do processo |
| GET | `/readyz` | Prontidão PostgreSQL/Redis |
| GET | `/v1/capabilities` | Capacidades da base |
| POST | `/v1/companies` | Cadastrar empresa |
| GET | `/v1/companies` | Listar empresas do tenant |
| POST | `/v1/companies/{id}/certificates` | Cadastrar/substituir certificado A1 |
| POST | `/v1/documents` | Aceitar geração ou emissão assíncrona |
| GET | `/v1/documents` | Listar documentos do tenant |
| POST | `/v1/documents/bulk` | Aceitar até 50 documentos com resultado individual |
| GET | `/v1/documents/{id}` | Consultar documento e arquivos |
| POST | `/v1/documents/{id}/reconcile` | Consultar resultado fiscal desconhecido; nunca reenviar |
| GET | `/v1/documents/{id}/artifacts/{artifactId}` | Baixar artefato autorizado para o tenant |
| GET | `/v1/companies/{id}` | Consultar empresa |
| PATCH | `/v1/companies/{id}` | Editar razão social, IE e CRT |
| DELETE | `/v1/companies/{id}` | Desativar empresa preservando histórico |
| GET | `/v1/companies/{id}/certificate` | Consultar metadados do certificado ativo |
| PUT | `/v1/companies/{id}/certificate` | Cadastrar ou substituir certificado |
| DELETE | `/v1/companies/{id}/certificate` | Desativar certificado |
| GET | `/v1/companies/{id}/fiscal` | Consultar configurações fiscais |
| PUT | `/v1/companies/{id}/fiscal/nfe` | Configurar tipo, série e ambiente |
| PUT | `/v1/companies/{id}/fiscal/nfce` | Configurar tipo, série e ambiente |
| GET | `/v1/document-types` | Catálogo de documentos implementados e futuros |
| GET | `/v1/states` | Catálogo das 27 UFs brasileiras, códigos IBGE e autorizadores SEFAZ |
| POST | `/v1/documents/validate` | Validar entrada (completa ou simplificada) e retornar valores resolvidos |
| GET | `/v1/documents/{id}/xml` | Baixar diretamente o XML da nota (`authorized_xml` ou `signed_xml`) |
| GET | `/v1/documents/{id}/pdf` | Baixar diretamente o PDF da nota (`danfe_pdf` ou `preview_pdf`) |
| POST | `/v1/documents/{id}/query` | Consultar situação atual da chave de 44 dígitos na SEFAZ |
| POST | `/v1/documents/{id}/cancel` | Cancelar nota autorizada na SEFAZ (15 a 255 caracteres) |
| POST | `/v1/documents/{id}/correction-letter` | Emitir Carta de Correção Eletrônica (CC-e) para NF-e autorizada |
| GET | `/v1/companies/{id}/sefaz/status` | Consultar disponibilidade em tempo real da SEFAZ da UF da empresa |
| POST | `/v1/companies/{id}/void-range` | Inutilizar faixa de numeração fiscal na SEFAZ |
| POST | `/v1/companies/{id}/dfe/sync` | Consultar/baixar XMLs emitidos contra o CNPJ (Distribuição DF-e Nacional) |

A prontidão HTTP não garante SEFAZ disponível, certificado configurado ou fila vazia.
Rotas ainda não implementadas estão em [API_ROADMAP](API_ROADMAP.md).

Consulte também o [guia de campos, cliente destinatário, tipos e estados](REQUEST_FIELDS.md) e o [fluxo cadastral](COMPANY_API.md).

## Respostas

```json
{"data":{"id":"UUID","replayed":false,"status_url":"/v1/documents/UUID"},"error":null,"request_id":"UUID"}
```

```json
{"data":null,"error":{"code":"CONFLICT","message":"Chave reutilizada com outro conteúdo ou estado incompatível.","source":"allspark","retryable":false},"request_id":"UUID"}
```

201 cadastro; 202 trabalho persistido; 207 resultados individuais de bulk;
401 credencial inválida; 403 scope insuficiente; 404 objeto ausente ou de outro tenant;
409 conflito de idempotência/estado; 422 dados inválidos/não suportados;
429 limite/backpressure; 503 dependência indisponível. `Retry-After: 1` acompanha 429.
As mensagens internas de PHP/SQL não são retornadas ao cliente.

## Empresas e certificado

Use [empresa de exemplo](../examples/company.json). CNPJ sem máscara, inclusive
algoritmo de DV alfanumérico; UF, IE, CRT e address obrigatórios. Dados exemplificativos não
constituem configuração fiscal para uma empresa real.

Certificado: `{pfx_base64, password, csc?, csc_id?}`. A resposta contém somente ID,
fingerprint e vencimento. Não existe download da chave privada. O upload exige
`certificates:write` e tem limite específico de 5 tentativas por minuto por tenant; a troca desativa o anterior em transação.

## Documentos

Use [requisição completa](../examples/document.json). Campos externos:
`company_id`, `kind` (`nfe`/`nfce`), `operation` (`generate`/`issue`),
`environment` (`homologation`), `series` e `invoice`.

`invoice` contém natureza, direção, destino, cidade, consumidor final, presença,
endereço do emitente, destinatário, produtos, impostos e pagamentos. Quantidades,
preços, bases, alíquotas e valores são strings decimais; não usar ponto flutuante
para formar os valores financeiros no ERP. `items[].total` deve corresponder a
quantidade × preço com duas casas. O modelo ainda não cobre todos os cenários fiscais.

```sh
curl -X POST http://127.0.0.1:8088/v1/documents \
  -H "Authorization: Bearer $ALLSPARK_API_KEY" \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: pedido-123-nfe' \
  --data-binary @examples/document.json
```

Substitua o `company_id` do exemplo antes de executar. `issue` é recusado enquanto
`ENABLE_ISSUANCE=false`; produção é recusada mesmo se esse switch for ativado.

Bulk usa `{documents:[{idempotency_key, document:{...}}]}`. Cada item é sua própria
transação; falha em um não desfaz os outros. Reenviar somente falhas ou reutilizar as
mesmas chaves. Máximo 50 itens, corpo de 8 MiB e invoice de 1 MiB. Grandes imports
terão upload assíncrono futuro. Downloads requerem a mesma autorização do documento.
