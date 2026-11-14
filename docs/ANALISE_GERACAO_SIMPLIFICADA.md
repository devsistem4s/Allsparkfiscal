# Análise Completa do Sistema: Feito, Pronto, Pendente, Auditoria do Motor Fiscal e Plano de Execução

Data da atualização: 14/11/2026.

Este documento consolida o diagnóstico técnico do **AllSpark Fiscal**, incluindo:
1. Como o sistema funciona hoje.
2. O que está **Feito/Pronto** vs **Pendente**.
3. **Auditoria de Segurança, Atualização Git e Reforma Tributária** do motor PHP (`sped-nfe`) comparando com alternativas em outras linguagens (Go, TypeScript, Python, Rust).
4. Capacidades de **gestão completa da nota** (pegar XML, PDF, consultar status na SEFAZ, cancelar, CC-e, inutilizar, DistDFe).
5. **Plano de Execução Estruturado (Master Plan)** para deixar a API pronta, simplificada e nacional.

---

## 1. Visão Geral de Como o Sistema Funciona Hoje

O AllSpark Fiscal é uma API fiscal multi-tenant e assíncrona dividida em duas camadas principais:

1. **Orquestrador Core em Go (`cmd/api`, `internal/`)**:
   - **Multi-tenancy e Segurança**: Autenticação via `Authorization: Bearer <api_key>` com hash SHA-256 no banco e escopos granulares (`companies:read/write`, `certificates:read/write`, `documents:read/write`). Isolamento estrito por `tenant_id`.
   - **Cofre Criptográfico (`internal/vault`)**: Certificados A1 (`.pfx`, senha, `csc` e `csc_id`) e artefatos fiscais (XML e PDF) são cifrados com **AES-256-GCM** (envelope encryption com DEK exclusiva por objeto).
   - **Persistência e Mensageria (`internal/store`, `internal/queue`, `internal/worker`)**:
     - PostgreSQL garante idempotência (`Idempotency-Key`), reserva transacional de numeração fiscal por `tenant + company + kind + environment + series` e gravação atômica de `jobs` + `outbox`.
     - Dispatcher lê a tabela `outbox` e publica no RabbitMQ (Quorum Queue com confirmação).
     - Workers replicáveis processam os jobs com controle de lease e *advisory lock* por empresa/modelo/ambiente para serializar transmissões e evitar duplicidade na SEFAZ.
2. **Motor Fiscal Isolado (`engines/nfephp`)**:
   - Comunica-se apenas internamente com os workers Go via HTTP autenticado (`internal/engine.FiscalProvider`).
   - Utiliza `nfephp-org/sped-nfe` e `nfephp-org/sped-da`.
   - O Go converte o contrato JSON de negócio (`internal/fiscal/invoice.go`) em tags estruturadas, injeta os dados cadastrais e endereço da empresa emitente (`companies`) e chama o motor para:
     - `certificate`: validar PFX, chave privada, CNPJ, validade e cadeia ICP-Brasil.
     - `prepare`: montar XML 4.00, assinar digitalmente, validar contra os schemas XSD (`PL_010_V1.30`) e renderizar prévia PDF (DANFE / DANFCE).
     - `transmit`: enviar lote síncrono (`sefazEnviaLote`) para a SEFAZ da UF do emitente, anexar protocolo (`Complements::toAuthorize`) e gerar o PDF final.
     - `reconcile`: consultar chave na SEFAZ (`sefazConsultaChave`) quando uma transmissão sofre timeout ou resposta ambígua, sem risco de reenviar ou duplicar nota.

---

## 2. Auditoria do Motor Fiscal: Segurança, Versão no Git, Reforma Tributária e Comparativo de Linguagens

### 2.1. O Motor PHP Atual é Seguro? E as Vulnerabilidades do PHP?

**Sim, na arquitetura atual ele está isolado e seguro**, pelos seguintes motivos técnicos verificados no código (`compose.yaml`, `engines/nfephp/Dockerfile` e `engines/nfephp/public/index.php`):

1. **Invisível na Internet (Zero Exposição Externa)**:
   - O container `engine` **não publica nenhuma porta no host** (`compose.yaml`). Ele roda preso na rede interna `backend`, acessível exclusivamente pelos processos Go (`api` e `worker`).
   - As vulnerabilidades críticas recentes do ecossistema PHP (como `CVE-2024-4577` em PHP-CGI no Windows ou exploits em PHP-FPM exposto diretamente) **não se aplicam** porque usamos `php:8.3-cli-bookworm` em Linux Debian, sem CGI e sem exposição pública.
2. **Autenticação Interna e Hardening de Container**:
   - Toda chamada Go -> PHP exige `Authorization: Bearer <engine_token>` de 32+ bytes, validado com `hash_equals` (comparação em tempo constante contra timing attacks).
   - O container roda sem root (`USER 1000:1000`), com filesystem **somente leitura (`read_only: true`)**, todas as capabilities Linux removidas (`cap_drop: [ALL]`) e `no-new-privileges: true`.
   - O parser XML bloqueia ataques de **XXE (XML External Entity)** usando `LIBXML_NONET` e rejeitando `<!DOCTYPE` e `<!ENTITY`.
   - Exceções internas nunca são serializadas no retorno HTTP (`display_errors=0`), impedindo vazamento de PFX, senhas ou XML em logs/respostas.
   - `composer audit` (`var/reports/composer-audit.json`) reporta **0 vulnerabilidades (`advisories: []`)**.
3. **Pontos de Melhoria no Runtime do Motor**:
   - **Atualizar PHP CLI Server para Produção**: Hoje o `Dockerfile` usa `php -S 0.0.0.0:9000` com `PHP_CLI_SERVER_WORKERS=4`. Para alta concorrência em produção, devemos empacotar com **FrankenPHP** (servidor de aplicação PHP moderno escrito em Go) ou **PHP-FPM + Nginx/Caddy**, além de manter a imagem base sempre na última release de segurança (`php:8.3-cli-bookworm` / `php:8.4-cli-bookworm`).

### 2.2. Está Atualizado com a Última Versão do Git?

Verificação em tempo real contra o GitHub em **14/11/2026**:

| Biblioteca | Versão Local Atual (`composer.json` / `upstream.lock.json`) | Última Versão no GitHub | O que saiu de novo no GitHub que precisamos atualizar |
|---|---|---|---|
| **`nfephp-org/sped-nfe`** | `v5.2.8` (`759fb23`) | **`v5.2.11` (`4596b89`, 13/11/2026)** | **66 commits novos!** Inclui **Pacote de Liberação `PL_010f` (NT 2025.002 v1.50)** de IBS/CBS/IS, **NT 2026.006, 2026.007 (IE opcional no emitente) e NT 2026.008 v1.01** (`vUnComLiq`, `vProdLiq`, `vProdLiqTot` e soma `vIBS+vCBS+vIS` em `vItem`), além dos novos eventos de vínculo de pagamento (`110300`, `110001`) e crédito presumido (`211110`). |
| **`nfephp-org/sped-da`** | `v1.1.6` (`93ae550`) | `v1.1.6` (tag) / **`master` +110 commits (`105e9c1`)** | Na branch `master` já entraram suporte a **CNPJ Alfanumérico no DANFE**, **CRT 4 (MEI)**, novos meios de pagamento da **NT 2024.002**, DANFE Simplificado Varejo e DANFSe v2 (NT 008). |
| **`Fm-s/open-nfse`** | `v0.11.0` (`0b70af4`) | **`v0.11.1` (`18d70dd`, 03/11/2026)** | Atualização de segurança nas dependências `undici 8.12.3` e `node-forge 1.4.2`. |
| **`brasil-fiscal/nfe`** | `374f9c7` | **`52dc6df` (29/10/2026)** | Correção de tag `<ICMS40>` para CST 40/41/50 e default `indIntermed=0`. |

### 2.3. E a Reforma Tributária (IBS, CBS e Imposto Seletivo)?

- **No motor `sped-nfe`**: **JÁ ESTÁ IMPLEMENTADO!** A biblioteca já possui `TraitTagDetIBSCBS.php` (`tagIBSCBS`, `gIBSUF`, `gIBSMun`, `gCBS`, monofasia, diferimento, redução de alíquota, devolução, crédito presumido, ZFM e compras governamentais), `TraitTagDetIS.php` (`tagIS`), os totalizadores `IBSCBSTot` e `ISTot` em `TraitTagTotal.php` e os eventos da Reforma Tributária em `TraitEventsRTC.php`. Atualizando do `5.2.8` para o `5.2.11`, ficamos com a **NT 2025.002 v1.50 (`PL_010f`) e NT 2026.008 v1.01** 100% em dia.
- **No nosso código Go (`internal/fiscal/invoice.go`)**: **PENDENTE EXPOR NO JSON!** Embora o `Engine.php` já aceite as tags `'IBSCBS'`, `'IBSCBSTribRegular'`, `'IBSCBSMono'`, `'IBSCBSTot'`, `'IS'`, `'ISTot'`, a struct `Taxes` no `internal/fiscal/invoice.go` hoje só possui `ICMS`, `PIS` e `COFINS`. Precisamos adicionar os blocos `IBSCBS` e `IS` no `invoice.go` para que a sua API consiga receber (ou calcular no modo simplificado) os novos impostos da Reforma Tributária.

### 2.4. Comparativo com Outros Projetos no GitHub (Vale a Pena Refazer em Outra Linguagem?)

| Projeto e Linguagem | Comunidade / Maturidade | Reforma Tributária (IBS/CBS/IS) | Recursos Fiscais (NF-e/NFC-e/Eventos/DANFE) | Análise: Vale a pena trocar ou reescrever? |
|---|---|---|---|---|
| **`nfephp-org/sped-nfe` (PHP)** | **1.624 stars / 662 forks** (Padrão de mercado open-source no Brasil há +15 anos) | **Completa** (`PL_010f`, NT 2025.002 v1.50, NT 2026.008 v1.01) | **100%**: 27 UFs, contingência, QR Code, todos os eventos, DistDFe, DANFE/DANFCE | **Melhor escolha para NF-e/NFC-e hoje.** Sempre que uma SEFAZ estadual muda URL, certificado ou regra de Nota Técnica, a comunidade atualiza em horas/dias. |
| **`@brasil-fiscal/nfe` (TypeScript)** | 34 stars / 11 forks (Recente, código muito limpo) | **Sim** (Leiaute 4.00 + IBS/CBS + Split Payment + IS) | NF-e, NFC-e, Cancelamento, CC-e, Inutilização, DistDFe, Manifestação, DANFE, 5.571 municípios IBGE | Excelente arquitetura em TS, porém com comunidade ainda pequena. Ótima referência para tabelas IBGE e regras de validação ou como motor secundário. |
| **`Fm-s/open-nfse` (TypeScript)** | 30 stars / 9 forks (Focado 100% em NFS-e Nacional) | **Sim** (RTC v1.01, CNPJ alfanumérico, IBS/CBS) | Emissão DPS, consulta chave/NSU, cancelamento, substituição, parâmetros municipais e DANFSe local | **Melhor escolha para o motor de NFS-e Nacional** (`nfse.gov.br`), onde o ecossistema PHP é menos unificado. |
| **`mschunke/gonfe` (Go / Golang)** | 6 stars / 3 forks (Autor único) | **Parcial / Desatualizado para NTs de 2026** | NF-e, NFC-e, CT-e, MDF-e, Eventos e DANFE em Go puro | Apesar de ser em Go, tem apenas 6 stars e não acompanha a velocidade das NTs da Reforma Tributária de 2026. |
| **Reescrever do zero em Go nativo** | Interna (100% nossa) | Teríamos que codificar cada NT manualmente | Exige manter +150 tags XSD, particularidades SOAP de 14 autorizadores e geradores de PDF A4/80mm | Fazer tudo do zero agora atrasaria a entrega nacional e criaria um fardo enorme de manutenção a cada Nota Técnica da SEFAZ. |

**Decisão Recomendada**:
1. **Manter a arquitetura híbrida desacoplada (`FiscalProvider`)**: A API pública, segurança, banco, fila, criptografia, cálculos simplificados e regras de negócio ficam **100% em Go**.
2. **Para NF-e / NFC-e**: Atualizar imediatamente o motor `sped-nfe` para **`v5.2.11`** (e `sped-da` atualizado), aproveitando toda a maturidade e as Notas Técnicas de 2026 já testadas pela comunidade.
3. **Para NFS-e (Serviços)**: Integrar o **`open-nfse` (`v0.11.1`)** para o Padrão Nacional (`nfse.gov.br`) + adaptadores municipais.

---

## 3. Gerenciamento Completo da Nota (XML, PDF, Status, Eventos)

Você perguntou: *"Consigo pegar o XML, trazer ela certinho, verificar status, na verdade tudo que precisa para gerenciar ela?"*

Veja o que **já existe hoje** e o que a biblioteca `sped-nfe` (`Tools.php`) já tem pronto no motor, bastando expor na API Go:

### O que você JÁ consegue fazer hoje:
1. **Validar os dados antes de emitir**: `POST /v1/documents/validate`.
2. **Gerar o XML assinado e PDF de prévia**: `POST /v1/documents` (`"operation": "generate"`).
3. **Emitir na SEFAZ**: `POST /v1/documents` (`"operation": "issue"`).
4. **Verificar o Status da Nota no Sistema, Chave de 44 dígitos e Protocolo**: `GET /v1/documents/{id}` retorna `status`, `access_key`, `result.protocol` (`number`, `status_code`, `reason`) e a lista de `artifacts`.
5. **Pegar o XML e o PDF**: `GET /v1/documents/{id}/artifacts/{artifactId}` baixa o `signed_xml` (XML assinado), `authorized_xml` (XML completo com protocolo `procNFe`), `protocol_xml` e `danfe_pdf` / `preview_pdf`.
6. **Reconciliar Status na SEFAZ**: `POST /v1/documents/{id}/reconcile` consulta a chave na SEFAZ quando a nota ficou com resultado pendente.

### O que falta expor na API para você gerenciar 100% do ciclo de vida da Nota:
1. **Atalhos Diretos para Pegar XML e PDF (`GET /v1/documents/{id}/xml` e `GET /v1/documents/{id}/pdf`)**:
   - Em vez de obrigar o cliente a ler o array `artifacts[]` e fazer uma segunda busca pelo `artifactId`, devolver diretamente o XML autorizado (ou assinado) e o DANFE em PDF com 1 chamada!
2. **Consulta de Status na SEFAZ a Qualquer Momento (`POST /v1/documents/{id}/query` ou `GET /v1/sefaz/status?uf=SP`)**:
   - Hoje o `/reconcile` só roda se a nota estiver em `reconciliation_required`. Precisamos permitir consultar a situação atual da chave na SEFAZ a qualquer momento (para saber se foi cancelada externamente, etc.) e consultar se a SEFAZ da UF está online (`sefazStatus`).
3. **Cancelamento de Nota (`POST /v1/documents/{id}/cancel`)**:
   - Chama `$tools->sefazCancela($chave, $justificativa, $nProt)`, anexa o protocolo com `Complements::toAuthorize`, salva o `cancel_event_xml` nos artefatos e muda o status do documento para `canceled`.
4. **Carta de Correção - CC-e (`POST /v1/documents/{id}/correction-letter`)**:
   - Chama `$tools->sefazCCe($chave, $textoCorrecao, $nSeqEvento)`, salva o XML protocolado (`cce_xml`) e gera o PDF da CC-e (`Dacce`).
5. **Inutilização de Numeração (`POST /v1/companies/{id}/void-range`)**:
   - Chama `$tools->sefazInutiliza($nSerie, $nIni, $nFin, $xJust, $tpAmb, $ano)` quando uma faixa de numeração não foi usada, gravando o XML de inutilização.
6. **Distribuição DF-e e Manifestação (`POST /v1/companies/{id}/dfe/sync` e `POST /v1/documents/{id}/manifest`)**:
   - Chama `$tools->sefazDistDFe` e `$tools->sefazManifesta` para buscar e baixar o XML de todas as notas emitidas **contra** o CNPJ da empresa no Brasil inteiro.

---

## 4. Catálogo Completo dos Endpoints Existentes Hoje

| Grupo | Método | Endpoint | Escopo Exigido | Descrição |
|---|---|---|---|---|
| **Saúde** | `GET` | `/healthz` | Público | Verifica se o processo HTTP está vivo |
| **Saúde** | `GET` | `/readyz` | Público | Verifica conectividade com PostgreSQL e Redis |
| **Catálogo** | `GET` | `/v1/capabilities` | Qualquer válido | Retorna capacidades ativas, tipos e se emissão está ligada |
| **Catálogo** | `GET` | `/v1/document-types` | `documents:read` | Lista modelos (`nfe` 55, `nfce` 65, `nfse` futuro) |
| **Empresas** | `POST` | `/v1/companies` | `companies:write` | Cadastra empresa emitente (CNPJ, UF, IE, CRT e `address`) |
| **Empresas** | `GET` | `/v1/companies` | `companies:read` | Lista empresas do tenant (paginação `limit` e `cursor`) |
| **Empresas** | `GET` | `/v1/companies/{id}` | `companies:read` | Consulta dados da empresa emitente |
| **Empresas** | `PATCH` | `/v1/companies/{id}` | `companies:write` | Atualiza `name`, `ie`, `crt` ou `address` |
| **Empresas** | `DELETE` | `/v1/companies/{id}` | `companies:write` | Arquiva (desativa logicamente) a empresa |
| **Config Fiscal** | `GET` | `/v1/companies/{id}/fiscal` | `companies:read` | Lista configurações fiscais (`nfe`, `nfce`) da empresa |
| **Config Fiscal** | `PUT` | `/v1/companies/{id}/fiscal/nfe` | `companies:write` | Configura ambiente, série e habilitação de NF-e (mod. 55) |
| **Config Fiscal** | `PUT` | `/v1/companies/{id}/fiscal/nfce` | `companies:write` | Configura ambiente, série e habilitação de NFC-e (mod. 65) |
| **Certificado** | `PUT` | `/v1/companies/{id}/certificate` | `certificates:write` | Envia/substitui certificado A1 (`pfx_base64`, `password`, `csc`, `csc_id`) |
| **Certificado** | `POST` | `/v1/companies/{id}/certificates` | `certificates:write` | Alias compatível para cadastro/substituição de certificado A1 |
| **Certificado** | `GET` | `/v1/companies/{id}/certificate` | `certificates:read` | Consulta metadados (`id`, `fingerprint`, `expires_at`, `expired`) |
| **Certificado** | `DELETE` | `/v1/companies/{id}/certificate` | `certificates:write` | Desativa o certificado ativo da empresa |
| **Estados (UFs)** | `GET` | `/v1/states` | `documents:read` | Catálogo das 27 UFs brasileiras com código IBGE e autorizador SEFAZ (`SVRS`, `SVAN`, próprio) |
| **Documentos** | `POST` | `/v1/documents/validate` | `documents:write` | Valida o payload da nota (com Smart Defaults, DIFAL e Reforma Tributária) sem gravar no banco |
| **Documentos** | `POST` | `/v1/documents` | `documents:write` | Enfileira geração (`generate`) ou emissão (`issue`) assíncrona (requer `Idempotency-Key`) |
| **Documentos** | `POST` | `/v1/documents/bulk` | `documents:write` | Enfileira lote de 1 a 50 notas em uma única chamada (retorna `207`) |
| **Documentos** | `GET` | `/v1/documents` | `documents:read` | Lista documentos do tenant (filtros `company_id`, `kind`, `cursor`, `limit`) |
| **Documentos** | `GET` | `/v1/documents/{id}` | `documents:read` | Consulta status, chave de 44 dígitos, protocolo, erros e links de artefatos |
| **Documentos** | `GET` | `/v1/documents/{id}/xml` | `documents:read` | Download direto do XML principal (`authorized_xml` ou `signed_xml`) |
| **Documentos** | `GET` | `/v1/documents/{id}/pdf` | `documents:read` | Download direto do PDF principal (`danfe_pdf` ou `preview_pdf`) |
| **Documentos** | `POST` | `/v1/documents/{id}/reconcile` | `documents:write` | Agenda consulta por chave na SEFAZ para documento em `reconciliation_required` |
| **Gestão SEFAZ** | `POST` | `/v1/documents/{id}/query` | `documents:write` | Consulta ao vivo a chave de 44 dígitos na SEFAZ (`sefazConsultaChave`) |
| **Gestão SEFAZ** | `POST` | `/v1/documents/{id}/cancel` | `documents:write` | Cancelamento fiscal (Evento `110111`) na SEFAZ com geração de `cancel_xml` |
| **Gestão SEFAZ** | `POST` | `/v1/documents/{id}/correction-letter` | `documents:write` | Carta de Correção Eletrônica (CC-e, Evento `110110`) com geração de `cce_xml` e `cce_pdf` |
| **Gestão SEFAZ** | `GET` | `/v1/companies/{id}/sefaz/status` | `companies:read` | Consulta em tempo real a disponibilidade da SEFAZ da UF da empresa (`sefazStatus`) |
| **Gestão SEFAZ** | `POST` | `/v1/companies/{id}/void-range` | `companies:write` | Inutilização de faixa de numeração fiscal (`sefazInutiliza`) |
| **Gestão SEFAZ** | `POST` | `/v1/companies/{id}/dfe/sync` | `companies:write` | Sincronização de DF-e / notas emitidas contra o CNPJ no Ambiente Nacional (`sefazDistDFe`) |
| **Artefatos** | `GET` | `/v1/documents/{id}/artifacts/{artifactId}` | `documents:read` | Faz download de qualquer artefato individual (`signed_xml`, `preview_pdf`, `authorized_xml`, `danfe_pdf`, `cancel_xml`, `cce_xml`, `cce_pdf`) |

---

## 5. PLANO DE AÇÃO MESTRE — STATUS DE EXECUÇÃO (FEITO/PRONTO vs PENDENTE)

### Etapa 1 — Atualização e Blindagem do Motor Fiscal (`engines/nfephp`) — **CONCLUÍDA E VERIFICADA**
- [x] **1.1.** Atualizado `nfephp-org/sped-nfe` de `5.2.8` para **`5.2.11`** (trazendo `PL_010f`, NT 2025.002 v1.50 e NT 2026.006/007/008 v1.01 da Reforma Tributária) e atualizado `engines/upstream.lock.json` (`4596b89e457d27ad2c114dda2b2ed04043f6327c`).
- [x] **1.2.** Atualizados `third_party/open-nfse` (`v0.11.1` / `18d70dd2acac6fcc5c4a918a451c61c548b20596`) e `third_party/brasil-fiscal-nfe` (`52dc6df2fd02c7a96ba8b37b071c702291da2fae`), além de `composer audit` com **0 vulnerabilidades** em `var/reports/composer-audit.json`.
- [x] **1.3.** Revisado `engines/nfephp/src/Engine.php` e `public/index.php` para suportar todas as tags da Reforma Tributária (`IBSCBS`, `IBSCBSTot`, `IS`, `ISTot`), DIFAL (`ICMSUFDest`), `IPI`, `CEST`, `refNFe`, e os endpoints `status`, `cancel`, `correction_letter`, `void_range` e `dist_dfe`.

### Etapa 2 — Geração de Nota Fiscal Simplificada ("Smart Defaults" para Qualquer Lugar do Brasil) — **CONCLUÍDA E VERIFICADA**
- [x] **2.1. Auto-preenchimento inteligente de cabeçalho (`internal/fiscal/invoice.go`)**:
  - `nature`: padrão `"VENDA DE MERCADORIA"` se omitido.
  - `direction`: padrão `1` (Saída) se omitido.
  - `city_code`: herdado automaticamente de `company.address.city_code` se omitido na nota.
  - `destination` (`idDest`): calculado automaticamente comparando `company.uf` com `recipient.address.state` (`1` se mesma UF, `2` se UF diferente, `3` se exterior `EX`; ou `1` para NFC-e presencial).
  - `final_consumer`: calculado automaticamente quando consumidor final ou NFC-e.
  - `presence`: padrão `1` (operação presencial) se omitido.
  - `freight_mode`: padrão `9` (sem ocorrência de transporte) se omitido.
- [x] **2.2. Destinatário Opcional na NFC-e (Modelo 65)**:
  - Permite emitir NFC-e sem `recipient` (consumidor final não identificado).
- [x] **2.3. Auto-cálculo Financeiro (`items` e `payments`)**:
  - Se `item.total` for omitido, calcula automaticamente `quantity * unit_price` com arredondamento de 2 casas decimais.
  - Se `payments` for omitido, gera automaticamente 1 pagamento à vista (`method: "01"`) com o valor líquido total da nota (`vNF`).
- [x] **2.4. Tributação Simplificada Automática por Regime (`CRT`)**:
  - Quando o integrador não envia `taxes` ou `cfop` no item:
    - Define `cfop` automaticamente: `5102` (operação interna `destination=1`) ou `6102` (operação interestadual `destination=2`).
    - Define `icms` (`CSOSN 102` para Simples/MEI ou `CST 41` para Regime Normal) e `pis`/`cofins` (`CST 07`).

### Etapa 3 — Cobertura Tributária Completa + Reforma Tributária (IBS, CBS e IS) — **CONCLUÍDA E VERIFICADA**
- [x] **3.1. Reforma Tributária no DTO Go (`internal/fiscal/invoice.go`)**:
  - Suporte completo aos grupos `ibs_cbs` (`CST`, `cClassTrib`, `vBC`, `ibs_uf_rate`, `ibs_uf_amount`, `ibs_mun_rate`, `ibs_mun_amount`, `cbs_rate`, `cbs_amount` com cálculo automático dos valores) e `is` (Imposto Seletivo) por item e na totalização (`IBSCBSTot`, `ISTot`), validado contra o XSD oficial `PL_010_V1.30`.
- [x] **3.2. Expansão de ICMS, IPI, PIS/COFINS e DIFAL Interestadual**:
  - Suportados todos os CSTs de ICMS (`00, 10, 20, 30, 40, 41, 50, 51, 60, 70, 90`) e CSOSNs (`101, 102, 103, 201, 202, 203, 300, 400, 500, 900`).
  - Suportado **DIFAL (`icms_uf_dest` -> `tagICMSUFDest`)** e **FCP** para vendas interestaduais a consumidor final em qualquer UF, com cálculo automático de `vICMSUFDest` e `vFCPUFDest` e soma automática no `ICMSTot`.
  - Suportados `discount` (`vDesc`), `freight` (`vFrete`), `insurance` (`vSeg`), `other_expenses` (`vOutro`), `cest`, `referenced_keys` (`refNFe`) e `ipi` por item e na soma do `vNF`.

### Etapa 4 — Gerenciamento Completo da Nota (XML, PDF, Status SEFAZ e Eventos) — **CONCLUÍDA E VERIFICADA**
- [x] **4.1. Atalhos Diretos de Download e Visualização**:
  - `GET /v1/documents/{id}/xml` — Retorna diretamente o XML (`authorized_xml` se autorizada, ou `signed_xml` se prévia/pendente).
  - `GET /v1/documents/{id}/pdf` — Retorna diretamente o PDF (`danfe_pdf` ou `preview_pdf`).
- [x] **4.2. Consulta de Status na SEFAZ**:
  - `GET /v1/companies/{id}/sefaz/status` — Consulta em tempo real se a SEFAZ da UF da empresa está em operação (`sefazStatus`).
  - `POST /v1/documents/{id}/query` — Consulta a situação atual da nota na SEFAZ pela chave de acesso (`sefazConsultaChave`).
- [x] **4.3. Eventos Pós-Emissão (Cancelamento, Carta de Correção, Inutilização e DistDFe)**:
  - `POST /v1/documents/{id}/cancel` — Cancela a nota na SEFAZ (exige `reason` de 15 a 255 caracteres), grava o XML de evento autorizado (`cancel_xml`) e atualiza status para `canceled`.
  - `POST /v1/documents/{id}/correction-letter` — Envia Carta de Correção (CC-e, `correction` de 15 a 1000 caracteres, `sequence` 1..20), guarda XML (`cce_xml`) e PDF (`cce_pdf` via `Dacce`).
  - `POST /v1/companies/{id}/void-range` — Inutiliza faixa de numeração (`kind`, `series`, `start_number`, `end_number`, `reason`).
  - `POST /v1/companies/{id}/dfe/sync` — Sincroniza documentos fiscais destinados via `sefazDistDFe` nacional.

### Etapa 5 — Catálogo de Estados e Ambiente de Produção Controlado — **CONCLUÍDA (Homologação Real e Webhooks Pendentes)**
- [x] **5.1.** Suporte a `ENABLE_PRODUCTION` no motor PHP (`engines/nfephp/src/Engine.php`) bloqueado por padrão por segurança até homologação real.
- [x] **5.2.** Endpoint `GET /v1/states` listando todas as 27 UFs brasileiras, códigos IBGE e autorizador SEFAZ correspondente.
- [ ] **5.3. (Pendente para fase operacional)** Webhooks assinados com HMAC-SHA256 para notificar o ERP integrador automaticamente e homologação real com certificado ICP-Brasil A1 em cada UF.

### Etapa 6 — Integração de NFS-e (Serviços para Qualquer Município) — **PENDENTE (Fase Futura)**
- [ ] **6.1.** Criar worker/adaptador usando `open-nfse` (`v0.11.1`, já atualizado em `third_party/open-nfse`) para emitir, consultar, cancelar e gerar DANFSe no **Padrão Nacional de NFS-e (`nfse.gov.br`)**.
- [ ] **6.2.** Estrutura de provedores municipais (`fiscal-providers/municipal`) para prefeituras fora do convênio nacional.
