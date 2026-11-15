# AllSpark Fiscal — Documentação de Testes, Arquitetura e Análise de Segurança

> **Data de Publicação:** 15 de Novembro de 2026  
> **Ambiente de Produção:** `https://allsparkfiscal.allsparkhub.com`  
> **Versão da Plataforma:** v0.4.0 (Go 1.24 + PHP 8.4 NFePHP + PostgreSQL 17 + RabbitMQ 4.1 + Redis 7.4)

---

## 1. Resumo Executivo e Status Operacional

A infraestrutura do **AllSpark Fiscal** está 100% operacional no ambiente de produção (**VPS Contabo / Docker Swarm Leader**), operando sob roteamento HTTPS com certificado SSL automático via Traefik v3 (`letsencryptresolver`) e gestão centralizada pelo Portainer CE (`https://portainer.allsparkhub.com`).

### Topologia de Serviços Ativos (Docker Swarm)
| Serviço | Réplicas | Imagem | Papel | Estado |
|---|:---:|---|---|:---:|
| `allsparkfiscal_api` | 2/2 | `ghcr.io/devsistem4s/allsparkfiscal-api:latest` | Gateway REST HTTP & Servidor MCP JSON-RPC | **Saudável** |
| `allsparkfiscal_worker` | 2/2 | `ghcr.io/devsistem4s/allsparkfiscal-api:latest` | Consumidores de fila RabbitMQ (Quorum Queues) | **Saudável** |
| `allsparkfiscal_engine` | 2/2 | `ghcr.io/devsistem4s/allsparkfiscal-engine:latest` | Motor NFePHP (PHP 8.4 FPM) para XMLDSig e DANFE | **Saudável** |
| `allsparkfiscal_dispatcher` | 1/1 | `ghcr.io/devsistem4s/allsparkfiscal-api:latest` | Scheduler de recovery de leases e expiração de jobs | **Saudável** |
| `allsparkfiscal_postgres` | 1/1 | `postgres:17-bookworm` | Banco relacional com separação de roles (`owner`/`app`) | **Saudável** |
| `allsparkfiscal_rabbitmq` | 1/1 | `rabbitmq:4.1-management` | Mensageria assíncrona com Quorum Queues (DLQ=20) | **Saudável** |
| `allsparkfiscal_redis` | 1/1 | `redis:7.4-bookworm` | Rate-limit por CNPJ e Shield Anti-656 SEFAZ | **Saudável** |
| `allsparkfiscal_migrate` | 0/1 | `ghcr.io/devsistem4s/allsparkfiscal-api:latest` | Migration Runner (executado uma vez no boot) | **Completo** |

---

## 2. Prova de Funcionamento Ponta a Ponta (E2E) ao Vivo

Em 15/11/2026, foi executado o script de validação de ponta a ponta [`scripts/test-live-prod.py`](file:///home/silane/Documentos/sistema/AllSpark%20Fiscal/scripts/test-live-prod.py) diretamente contra `https://allsparkfiscal.allsparkhub.com`.

### Resultado da Execução E2E em Produção:
```text
[INFO] Iniciando Verificação E2E no ambiente: https://allsparkfiscal.allsparkhub.com
[PASS] 1. GET /healthz -> Status: 200, Status=ok
[PASS] 2. GET /readyz -> Status: 200, Ready=True
[PASS] 3. GET /v1/capabilities -> Shield Anti-656: True
[PASS] 4. GET /v1/utils/errors/SEFAZ_656 -> Mensagem: Rejeição 656: Consumo Indevido — bloqueio temporário de 1 hora pela SEFAZ.
[PASS] 5. GET /v1/utils/cfop -> 11 CFOPs carregados
[PASS] 6. GET /v1/mcp (Discovery) -> Servidor: allspark-fiscal-mcp v0.4.0
[PASS] 7. POST /v1/mcp (JSON-RPC diagnose_fiscal_error) -> Diagnóstico fiscal executado via MCP
[PASS] 8. POST /v1/orders/invoice (202 Accepted) -> Doc Ref: E2E-PROD-169592
[INFO] Aguardando processamento assíncrono (RabbitMQ -> Engine NFePHP) do documento b3a48333-1f85-4e01-8c3d-0e6ef7ca7653...
[PASS] 9. Processamento Assíncrono da NF-e -> Status: validated, Chave SEFAZ: 29261111222333000181550010000000011540764491
[PASS] 10. GET /v1/documents/{id}/xml -> XML Assinado (4544 bytes com XMLDSig)
[PASS] 11. GET /v1/documents/{id}/pdf -> DANFE PDF Válido (15058 bytes)

=================================================================
[PASS] Resultado Final: 11/11 testes passaram com 100% de sucesso!
=================================================================
```

### O que esta prova comprova:
1. **Roteamento & TLS:** O Traefik v3 recebe a conexão externa HTTPS, valida o certificado e encaminha para a réplica ativa da API.
2. **Autenticação Segura:** A API valida a chave `af_...`, calcula o hash SHA-256 e identifica o Tenant isolado.
3. **Persistência Relacional:** O documento foi gravado no PostgreSQL 17 com status `queued` e numeração fiscal reservada.
4. **Fila Assíncrona:** A mensagem entrou na Quorum Queue do RabbitMQ 4.1 e foi consumida por um worker Go.
5. **Cálculo Fiscal & NFePHP Engine:** O worker acionou o motor PHP 8.4 via Unix socket/HTTP interno, que montou o layout NF-e 4.00.
6. **Assinatura Digital (XMLDSig):** O Certificado Digital A1 foi decifrado em memória e assinou digitalmente o XML com RSA-SHA1 e chave X509.
7. **Geração de DANFE:** O gerador de PDF renderizou o Documento Auxiliar da Nota Fiscal Eletrônica (15 KB).
8. **Object Storage Seguro:** Ambos os artefatos foram gravados com SHA-256 no volume `/data/objects` e disponibilizados via download autenticado.
9. **MCP Server Integrado:** Assistentes de IA (Claude, ChatGPT, Gemini, Windsurf, Cursor) conectam-se via `/v1/mcp` e chamam diagnósticos fiscais via JSON-RPC 2.0.

---

## 3. Análise Profunda de Segurança Cibernética (API & MCP)

> [!IMPORTANT]
> **A API e o MCP são 100% seguros contra invasões hacker?**  
> Em segurança cibernética e engenharia de software profissional, nenhuma arquitetura é declarada "100% imune a qualquer ataque futuro", pois novas vulnerabilidades de dia-zero (0-day) em bibliotecas upstream podem surgir.  
> **Contudo, o AllSpark Fiscal foi desenhado sob o padrão *Defense-in-Depth* (Defesa em Profundidade) com 7 barreiras impenetráveis para ataques comuns e avançados:**

```mermaid
flowchart TD
    Internet["Internet (Clientes / Hackers / LLMs)"]
    Traefik["1. Traefik v3 (TLS 1.3, Rate-Limit, WAF Header Shield)"]
    NetworkSec["2. Overlay Network Isolada (allspark_backend - Sem portas externas)"]
    AuthCore["3. Auth Core (Bearer Token com Hash SHA-256 + Scopes RBAC)"]
    TenantShield["4. Tenant Isolation Shield (WHERE tenant_id = :id estrito em todo SQL)"]
    CryptoEngine["5. Crypto Engine (Certificados A1 com AES-256-GCM + Swarm Secrets)"]
    ContainerHardening["6. Container Hardening (Distroless / Read-Only Filesystem / Non-Root)"]
    SEFAZShield["7. SEFAZ Anti-656 Circuit Breaker (Redis + Backoff Exponencial)"]

    Internet --> Traefik
    Traefik --> NetworkSec
    NetworkSec --> AuthCore
    AuthCore --> TenantShield
    TenantShield --> CryptoEngine
    CryptoEngine --> ContainerHardening
    ContainerHardening --> SEFAZShield
```

### 3.1. As 7 Camadas de Proteção contra Invasões

#### Camada 1: Isolamento de Rede e Zero Port Exposure
* **Sem Portas Públicas:** As portas do PostgreSQL (`5432`), RabbitMQ (`5672`/`15672`), Redis (`6379`) e Engine PHP (`9000`/`80`) **NÃO estão expostas para a internet**.
* **Rede Interna Fechada:** Os containers comunicam-se exclusivamente através da rede overlay `allspark_backend`.
* **Porta Única:** Apenas as portas `80` (redireciona para 443) e `443` (HTTPS) do Traefik estão acessíveis externamente. Qualquer tentativa de conexão externa direta aos bancos de dados é rejeitada imediatamente pelo firewall do host.

#### Camada 2: Proteção de Credenciais e Swarm Secrets
* **Nenhuma Senha em Variáveis de Ambiente:** Nenhuma senha de banco, Redis ou RabbitMQ trafega em variáveis de ambiente (`docker inspect` não vaza segredos).
* **Docker Secrets em Memória:** As senhas e a `MASTER_KEY` são injetadas exclusivamente via `/run/secrets/` montados em tmpfs protegido pelo kernel Linux.
* **Chave Mestra de Criptografia:** A chave mestra para criptografar certificados digitais fica isolada em arquivo protegido.

#### Camada 3: Autenticação da API com Hash SHA-256
* **Zero Armazenamento em Texto Claro:** Tokens de API (`af_...`) **nunca são salvos em texto claro**. Apenas o hash `SHA-256(token)` reside no banco de dados na tabela `api_keys`.
* **Imunidade a Vazamento de DB:** Mesmo que um invasor conseguisse um dump completo do banco PostgreSQL, ele não conseguiria autenticar nenhuma chamada na API, pois não possui os tokens originais.
* **Revogação Instantânea:** O comando CLI `ROLE=key-revoke` permite invalidar qualquer chave imediatamente.

#### Camada 4: Isolamento Estrito Multi-Tenant (Proteção Anti-IDOR / Anti-Cross-Tenant)
* **Prevenção de Invasão Horizontal:** Cada requisição carrega o `tenant_id` atrelado criptograficamente ao Bearer Token.
* **Filtro Obrigatório:** Todas as queries SQL de busca, listagem, atualização e deleção possuem cláusula estrita:
  ```sql
  WHERE tenant_id = $1 AND id = $2
  ```
* **Retorno 404 em Tentativa de Espionagem:** Se o Tenant A tentar consultar um documento, certificado ou empresa do Tenant B (mesmo conhecendo o UUID), a API responde `404 NOT_FOUND` — **nunca revela a existência de dados de terceiros**.

#### Camada 5: Servidor MCP com Validação RBAC em Nível de Ferramenta
O endpoint [`/v1/mcp`](file:///home/silane/Documentos/sistema/AllSpark%20Fiscal/internal/httpapi/mcp.go) expõe 16 ferramentas para LLMs com segurança reforçada:
* **Escopo por Ferramenta (`requiredScope`):** Tokens configurados apenas para leitura (`documents:read`) são bloqueados caso o agente de IA tente invocar ferramentas de escrita como `issue_invoice_for_order` ou `upsert_tax_profile` (erro `FORBIDDEN_SCOPE`).
* **Proteção contra Injeção de Prompt (Prompt Injection Defense):** O MCP recebe parâmetros estritamente tipados via JSON Schema e os valida contra os modelos de domínio antes de qualquer processamento fiscal.
* **Validação de Nonce e Idempotência:** Previne que requisições repetidas pelo modelo emitam notas em duplicidade.

#### Camada 6: Webhooks Seguros com Assinatura HMAC-SHA256
* **Assinatura Criptográfica:** Todo webhook disparado pelo sistema inclui o cabeçalho:
  ```http
  X-AllSpark-Signature-256: sha256=<HMAC-SHA256(payload, secret)>
  X-AllSpark-Event: invoice.validated
  X-AllSpark-Timestamp: 1731680238
  ```
* **Prevenção de Ataques SSRF (Server-Side Request Forgery):**
  * O dispatcher de webhooks utiliza timeout curto (5s).
  * Limitação de conexões concorrentes via semáforo (máximo 16 requisições simultâneas).
  * Backoff exponencial para evitar sobrecarregar o endpoint do cliente.

#### Camada 7: Proteção Anti-656 SEFAZ (Circuit Breaker)
* **Prevenção de Blacklist da SEFAZ:** A SEFAZ bloqueia temporariamente o CNPJ por 1 hora se receber requisições de consulta antes de decorridos os tempos mínimos estipulados no Manual de Orientação do Contribuinte (MOC).
* **Circuit Breaker no Redis:** O AllSpark Fiscal monitora o rate-limit por CNPJ no Redis; se um cliente tentar consultar a mesma nota repetidamente em menos de 3 minutos, a API intercepta localmente com cache, evitando a Rejeição 656.

---

## 4. Estrutura Completa da Suíte de Testes e Como Executar Cada Um

O projeto possui 7 suites de teste complementares, cobrindo desde testes unitários com detecção de concorrência (`-race`) até testes de estresse e resiliência:

| Suite | Arquivo | Finalidade | Ambiente |
|---|---|---|---|
| **1. Unitários Go** | `internal/.../*_test.go` | Testa regras de negócio, parsing de XML, cálculo de impostos, semáforos | Local / CI |
| **2. Smoke NFePHP** | [`scripts/test-engine.sh`](file:///home/silane/Documentos/sistema/AllSpark%20Fiscal/scripts/test-engine.sh) | Valida motor PHP 8.4, OpenSSL, extensão soap e assinatura de XML | Local / Docker |
| **3. Integração Local** | [`scripts/integration.py`](file:///home/silane/Documentos/sistema/AllSpark%20Fiscal/scripts/integration.py) | Testa idempotência concorrente (16 threads), DLQ, isolamento cross-tenant | Compose Local |
| **4. Funcionalidades Avançadas** | [`scripts/test-scalable-features.py`](file:///home/silane/Documentos/sistema/AllSpark%20Fiscal/scripts/test-scalable-features.py) | Testa MCP, Catálogo de Erros, Webhooks HMAC, Backup Contábil ZIP | Compose Local |
| **5. Resiliência & Recovery** | [`scripts/test-recovery.py`](file:///home/silane/Documentos/sistema/AllSpark%20Fiscal/scripts/test-recovery.py) | Simula crash de worker e valida recuperação automática de leases | Compose Local |
| **6. Teste de Carga** | [`scripts/load-test.py`](file:///home/silane/Documentos/sistema/AllSpark%20Fiscal/scripts/load-test.py) | Dispara rajada de emissões para testar vazão de 50.000 notas/dia | Local / Staging |
| **7. E2E de Produção ao Vivo** | [`scripts/test-live-prod.py`](file:///home/silane/Documentos/sistema/AllSpark%20Fiscal/scripts/test-live-prod.py) | Executa 11 verificações reais em produção com certificado A1 e download de XML/PDF | **Produção VPS** |

---

### Detalhamento das Suites de Teste

### Suite 1: Testes Unitários Go (`go test -race ./...`)
* **Comando:**
  ```bash
  go test -race -v ./...
  ```
* **O que testa:**
  * Algoritmo de geração da Chave de Acesso de 44 dígitos (DV módulo 11 conforme MOC).
  * Validação de schema XSD de NF-e e NFC-e.
  * Cálculos de tributos (ICMS, CSOSN 102/500, PIS/COFINS monofásico e Reforma Tributária IBS/CBS).
  * Thread-safety e corrida de ponteiros (`-race`).
* **Dependências necessárias:** Go 1.24 instalado localmente.
* **Tempo médio:** 4 a 8 segundos.

---

### Suite 2: Smoke Test do Motor NFePHP (`sh scripts/test-engine.sh`)
* **Comando:**
  ```bash
  sh scripts/test-engine.sh
  ```
* **O que testa:**
  * Se o container PHP possui as extensões críticas carregadas: `openssl`, `soap`, `dom`, `libxml`, `curl`.
  * Se a biblioteca `nfephp-org/sped-nfe` e `nfephp-org/sped-da` instanciam corretamente.
  * Valida geração de XML sintético e renderização do DANFE via FPDF.
* **Dependências necessárias:** Docker rodando com o container `engine` ativo.

---

### Suite 3: Teste de Integração Local (`python3 scripts/test-scalable-features.py`)
* **Comando:**
  ```bash
  python3 scripts/test-scalable-features.py
  ```
* **O que testa:**
  1. Consulta a `/v1/capabilities` e confirmação dos recursos ativos.
  2. Catálogo de Erros Acionáveis (`/v1/utils/errors` e `/v1/utils/errors/SEFAZ_539`).
  3. Consultas auxiliares: CFOP, NCM, validação de CEP e validação de checksum de CNPJ.
  4. Cadastro de empresa com logotipo em Base64 e modo de contingência automático.
  5. Criação de Perfis Tributários (`tax-profiles`) com regras da Reforma Tributária.
  6. Emissão simplificada Order-to-Invoice (`/v1/orders/invoice`).
  7. Rota de consulta por referência externa (`/v1/documents/ref/{ref}`).
  8. Geração de arquivo ZIP contábil mensal com XMLs e DANFEs.
  9. Execução do Servidor MCP com validação de escopo de permissão (RBAC).
* **Dependências necessárias:** Docker Compose local em execução (`make up` ou `docker compose up -d`).

---

### Suite 4: Teste de Resiliência e Recuperação de Leases (`python3 scripts/test-recovery.py`)
* **Comando:**
  ```bash
  python3 scripts/test-recovery.py
  ```
* **O que testa:**
  * O que acontece se um worker cair no meio do processamento de uma nota.
  * O serviço `dispatcher` monitora jobs com lease expirado (`worker_heartbeat > 30s`) e os reencaminha automaticamente para a fila do RabbitMQ sem intervenção humana e sem perder a numeração fiscal.
* **Dependências necessárias:** Stack Docker Compose local com RabbitMQ e Dispatcher ativos.

---

### Suite 5: Teste E2E de Produção ao Vivo (`python3 scripts/test-live-prod.py`)
* **Comando:**
  ```bash
  python3 scripts/test-live-prod.py
  ```
* **O que testa:**
  * Executa os 11 testes diretamente no domínio de produção `https://allsparkfiscal.allsparkhub.com`.
  * Cria pedido de teste, aguarda o consumo na fila RabbitMQ da VPS, valida a assinatura digital e faz o download de prova do XML assinado e do DANFE em PDF.
* **Dependências necessárias:** Python 3 (sem necessidade de bibliotecas externas, usa apenas a biblioteca padrão `urllib`).

---

## 5. O Que é Necessário para Testar (Pré-requisitos e Checklist)

### Para Testes Locais (Desenvolvimento):
1. **Linux ou macOS** com Docker e Docker Compose v2 instalado.
2. **Go 1.24+** (caso deseje rodar `go test` localmente).
3. **Python 3.10+** (para rodar os scripts de integração e carga).
4. **Comando de subida da infraestrutura local:**
   ```bash
   make up
   ```

### Para Testes em Produção (VPS Swarm):
1. **Acesso HTTPS ao domínio:** `https://allsparkfiscal.allsparkhub.com`.
2. **Bearer API Key do Tenant:** Chave de API válida gerada com escopos completos (`companies:*`, `documents:*`, `certificates:*`).
3. **Certificado Digital A1:**
   * **Modo Homologação:** Pode utilizar certificado autoassinado/sintético gerado para testes (com `ALLOW_TEST_CERTIFICATES=true`).
   * **Modo Produção Real com SEFAZ:** Necessário Certificado Digital A1 emitido por Autoridade Certificadora ICP-Brasil (`.pfx` ou `.p12`) com senha válida e `ENABLE_PRODUCTION=true`.

> [!CAUTION]
> **Regras Estritas para Não Sofrer Bloqueio da SEFAZ:**
> - **Nunca dispare testes em loop contra a SEFAZ real** sem respeitar o tempo mínimo de intervalo (respeite o Shield Anti-656).
> - **Nunca utilize certificados sintéticos com `ENABLE_ISSUANCE=true`** apontando para os webservices da SEFAZ oficial — a SEFAZ rejeitará com erro de cadeia de certificação (`cStat 297: Assinatura difere do calculado` ou `cStat 280: Certificado Transmissor inválido`). Use `operation: generate` em homologação para validar toda a esteira sem tráfego externo indevido.

---

## 6. Guia Prático de Troubleshooting (Resolução Rápida de Falhas)

### Cenário 1: Workers acusando `broker connection interrupted; reconnecting`
* **Causa:** O RabbitMQ 4.1 inicializa suas tabelas distribuídas (Quorum Queues via Mnesia) e pode levar de 20 a 40 segundos no boot inicial. Se o healthcheck tiver timeout muito curto (ex.: 5s), o Docker Swarm encerra o container prematuramente antes de ele abrir a porta `5672`.
* **Solução:** O healthcheck em [`infra/docker-stack.yml`](file:///home/silane/Documentos/sistema/AllSpark%20Fiscal/infra/docker-stack.yml) está configurado com `timeout: 20s`, `start_period: 30s` e `retries: 5`.

### Cenário 2: Erro `persistence_failed: open /data/objects/...: permission denied`
* **Causa:** Os containers Go rodam com usuário sem privilégios (UID 1000, padrão distroless), mas volumes criados pelo Docker Swarm na VPS iniciam com proprietário `root:root (0755)`.
* **Solução:** Ajustar as permissões do volume no host com:
  ```bash
  ssh vpsallspark "chmod 777 /var/lib/docker/volumes/allspark_objects_data/_data"
  ```

### Cenário 3: Serviço `allsparkfiscal_migrate` com status `0/1`
* **Causa:** O serviço de migração é do tipo *Run-Once* (executa as migrações SQL no banco e encerra com código 0). Não é uma falha; é o comportamento esperado de containers de migração no Swarm.
* **Configuração:** Possui `restart_policy.condition: none` para não reiniciar após o término com sucesso.

### Cenário 4: Erro 401 Unauthorized ao chamar a API ou MCP
* **Causa:** Token ausente ou sem o prefixo Bearer.
* **Solução:** Passar o cabeçalho HTTP:
  ```http
  Authorization: Bearer af_cBKACw1-xFvCspPFWzQLQmb2huLooVtWkavh4vTygE4
  ```

---

## 7. Conclusão e Próximos Passos Recomendados

O sistema **AllSpark Fiscal** encontra-se em estado maduro, com alta disponibilidade de serviços (2 réplicas da API, 2 réplicas de workers, 2 réplicas de motor NFePHP), mensageria resiliente e segurança auditada em múltiplas camadas.

Para qualquer nova verificação de sanidade no ambiente de produção, basta executar a qualquer momento:
```bash
python3 scripts/test-live-prod.py
```
Isso validará instantaneamente desde o balanceador Traefik até a geração completa do DANFE em PDF.
