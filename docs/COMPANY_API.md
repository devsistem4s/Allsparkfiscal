# Cadastro, requisições e validações

Todas as rotas `/v1` exigem `Authorization: Bearer <api_key>`. A chave define o tenant; enviar um `company_id` não concede acesso a outra organização. Empresas e documentos são sempre filtrados pelo tenant autenticado. O ID da empresa vai no caminho das operações cadastrais e no JSON das requisições de documentos.

## Fluxo executável

1. `POST /v1/companies` com `examples/company.json`: CNPJ sem pontuação, razão social, UF, IE, CRT e address. Retorna 201 com UUID. CNPJ tem dígitos verificadores validados; IE tem validação de formato, não consulta cadastral à SEFAZ. Campos desconhecidos são recusados.
2. `PUT /v1/companies/{id}/certificate`: `pfx_base64`, `password` e, para NFC-e, `csc` e `csc_id`. O motor verifica certificado, titularidade, validade e cadeia confiável. PFX, senha e CSC ficam criptografados; a chave de criptografia fica fora do banco.
3. `PUT /v1/companies/{id}/fiscal/nfe` (ou `nfce`): `{"environment":"homologation","series":1,"enabled":true}`. Isso habilita o tipo na empresa; não substitui credenciamento na SEFAZ. Produção permanece bloqueada.
4. `POST /v1/documents/validate` com o formato de `examples/document.json`: verifica entrada e regras suportadas sem criar documento, fila ou numeração. Retorno `valid:true` refere-se somente à entrada; não significa XSD válido, cálculo fiscal completo ou autorização.
5. `POST /v1/documents`, com `Idempotency-Key` de 8 a 128 caracteres e o mesmo JSON. `kind`: `nfe` (55) ou `nfce` (65). `operation`: `generate` para prévia ou `issue` para transmitir. Use o UUID cadastrado em `company_id`. Emissão exige configuração habilitada, série correspondente e certificado ativo não expirado antes de reservar número. Prévia não reserva numeração fiscal, mas precisa de certificado para o motor gerar XML assinado.
6. Retorno 202 fornece `id` e `status_url`. Consultar `GET /v1/documents/{id}`; arquivos disponíveis em `GET /v1/documents/{id}/artifacts/{artifactId}`. O processamento assíncrono faz geração, assinatura, validação XSD e, quando solicitado/habilitado, transmissão. 202 não significa nota autorizada.

## Administração

| Método e rota | Resultado |
|---|---|
| GET /v1/companies | Lista empresas, inclusive desativadas |
| GET /v1/companies/{id} | Cadastro e datas |
| PATCH /v1/companies/{id} | Edita `name`, `ie`, `crt`, `address`; CNPJ e UF imutáveis |
| DELETE /v1/companies/{id} | Desativação lógica; preserva documentos |
| GET /v1/companies/{id}/fiscal | Configurações por tipo |
| GET /v1/companies/{id}/certificate | Metadados do certificado ativo; nunca PFX/senha |
| DELETE /v1/companies/{id}/certificate | Desativa certificado |
| POST /v1/companies/{id}/certificates | Alias compatível de cadastro/substituição |
| GET /v1/document-types | NF-e/NFC-e implementadas; NFS-e futura |
| GET /v1/documents | Lista com filtros opcionais `company_id` e `kind` |
| POST /v1/documents/bulk | Até 50 documentos, resultado individual 207 |

Listagens aceitam `limit` de 1 a 100 (padrão 50) e `cursor` UUID. Use `next_cursor` quando `has_more` for true. A ordem é por UUID, não cronológica; páginas não são um snapshot contra inserções concorrentes.

Escopos: `companies:read/write`, `certificates:read/write`, `documents:read/write`. Escrita também permite leitura do mesmo recurso para compatibilidade com chaves anteriores. Nenhum escopo de leitura permite mutações. Cadastro/alterações são auditados. Alterações de empresa, configuração e desativação de certificado são recusadas com `COMPANY_BUSY` enquanto houver documentos pendentes ou exigindo reconciliação. A substituição do certificado é bloqueada durante trabalho ativo, mas permitida quando restam apenas documentos aguardando reconciliação: isso permite renovar um certificado expirado para consultar o resultado fiscal.

## Erros e limites

Envelope padrão: `data`, `error` (`code`, `message`, `source`, `retryable`) e `request_id`. 401 credencial; 403 escopo; 404 ausente/outra organização; 409 conflito, empresa desativada ou ocupada; 422 entrada/configuração/certificado inválido; 429 limite; 503 dependência.

Validações atuais incluem estrutura JSON estrita, limites de tamanho, tipos, ambiente, série, formatos de campos e consistência de quantidades/valores/pagamentos nos cenários tributários suportados. Cancelamento (evento 110111), Carta de Correção Eletrônica (CC-e evento 110110), Inutilização de faixa de numeração, Distribuição DF-e (sefazDistDFe), Manifestação do Destinatário (MDe) e Webhooks com assinatura HMAC-SHA256 estão 100% implementados e validados em produção. NFS-e (ADN nacional) está no roadmap futuro.

Contrato executável: `openapi/openapi.yaml`. Teste seguro do cadastro e preflight: `python3 scripts/test-company-api.py`, com emissão de homologação habilitada e sem envio de certificados ou documentos à SEFAZ. Migrações têm checksum e trava transacional; não editar migração já aplicada.

## Fluxo multi-tenant atualizado

Consulte [identidade, cadastro completo e credenciais somente por código](TENANT_FLOW.md). O cadastro de empresa agora exige `address`; o endereço do emitente é carregado desse cadastro. `invoice.issuer_address` é legado, opcional e ignorado na geração.
