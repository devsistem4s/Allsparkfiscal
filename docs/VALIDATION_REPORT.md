# Relatório de validação — 29/09/2026

## Ambiente e escopo

Ambiente Docker local, um PostgreSQL, um RabbitMQ, um Redis, uma API, um dispatcher,
um motor PHP e quatro workers. Somente dados/certificados sintéticos. Nenhuma
chamada foi realizada à SEFAZ e nenhum documento foi autorizado fiscalmente.

## Resultados

| Verificação | Resultado |
|---|---|
| Go test com detector de corridas | Passou |
| Go vet | Passou |
| Geração PHP isolada, sem rede | Certificado/CNPJ, XML assinado/XSD e PDF passaram |
| Integração Docker | Passou: isolamento, idempotência, bulk, certificado, fila e arquivos |
| Concorrência idempotente | 16 solicitações simultâneas resultaram em um documento/job |
| Prévia | Não consumiu sequência fiscal |
| Recuperação | Lease expirado antes do envio volta à fila; depois exige reconciliação |
| Composer audit | Nenhum advisory de segurança/abandono reportado |
| Govulncheck | Nenhuma vulnerabilidade alcançável no código/pacotes importados |
| Compose principal e overlays | Configurações sintaticamente válidas |
| Links locais da documentação | Verificados |

## Carga de entrada

1000 POSTs únicos, alvo 50/s, concorrência máxima
20, duração 19.987 s.

- Taxa observada: **50.03 requisições/s**.
- HTTP 202: **1000/1000**.
- Documentos persistidos: **1000**.
- p50: **5.47 ms**; p95: **7.26 ms**; p99: **8.71 ms**.

O tenant de carga não possui certificado: mede admissão durável e tratamento de
falha local, não geração/assinatura/transmissão em massa. Teste curto não comprova
SLA, capacidade contínua de 24 horas ou capacidade da SEFAZ. Não extrapolar estes
números como garantia de emissão de 50 mil/500 mil notas por dia.

## Segurança e limitações da varredura

Atualizada a dependência golang.org/x/crypto para v0.56.0 após identificar dois
avisos em SSH. O scan restante aponta GO-2026-5932 no subpacote openpgp do módulo;
**esse pacote não é importado nem chamado** pelo projeto, conforme análise de
alcance. Não existe correção indicada para esse pacote descontinuado. Monitorar
upgrades. Isso não equivale a auditoria completa, scan de imagens, SAST/DAST ou pentest.

O build Docker foi alinhado ao Go 1.27; go.mod exige 1.26 devido à dependência
corrigida. Bibliotecas PHP estão fixadas em composer.lock.

## Não validado nesta entrega

Emissão real/homologação externa, cobertura por UF, CRL/OCSP, KMS gerenciado,
S3 real, eventos fiscais, webhooks, NFS-e, observabilidade completa, restore/HA e
cenários tributários fora do subconjunto documentado. Produção continua bloqueada.

Logs e JSONs brutos sem credenciais ficam em var/reports (ignorados pelo Git).
Reprodução: docs/TESTING.md e scripts/.

Verificação final em 30/09/2026: serviços Docker ativos, quatro workers e healthz/readyz respondendo 200. O trust store sintético foi removido após os testes.

## Endpoints cadastrais — 2026-10-02

Testes Go com race detector e vet passaram. Docker reconstruído, migrações 001/002 aplicadas com ledger de checksums, API e quatro workers atualizados. `scripts/test-company-api.py` passou: cadastro, edição, filtros/paginação, isolamento entre tenants, scopes somente leitura, configuração, validação sem persistir, rejeição antes de reservar numeração e desativação. Teste isolado do motor passou para certificado, assinatura, XSD, PDF e bloqueios de transmissão/confiança. Nenhuma chamada fiscal externa realizada. Repositórios existentes conferidos contra upstream.lock.json e preservados.

## Cadastro multi-tenant completo — 2026-10-05

Migração 003 adiciona endereço ao cadastro sem modificar empresas existentes. Novas empresas exigem endereço e novos envios de empresas antigas exigem preenchimento por PATCH. O worker usa o endereço cadastrado; o teste integrado enviou um endereço divergente na nota e verificou que ele não apareceu no XML assinado. Teste integrado de XML/XSD/PDF e isolamento aprovado, sem chamadas SEFAZ. OpenAPI e exemplos atualizados para omitir endereço do emitente na requisição da nota. Credenciais permanecem provisionadas somente pelo comando administrativo, sem endpoint público de cadastro.
