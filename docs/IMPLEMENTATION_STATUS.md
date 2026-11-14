# Estado de implementação

| Capacidade | Estado |
|---|---|
| API Go, Docker Compose, quatro workers | Executável localmente |
| PostgreSQL, Redis, RabbitMQ quorum + outbox | Implementado e exercitado |
| Chaves de tenant com hash e scopes | Implementado; credencial de bootstrap |
| KEY + username + password, access/refresh token | Especificado, não implementado |
| Cadastro de empresa | POST/GET/PATCH/DELETE lógico, paginação e isolamento testados |
| Configuração fiscal por empresa | GET/PUT por NF-e/NFC-e; preflight antes de reservar número |
| Validação de entrada | POST /v1/documents/validate sem persistência; não equivale a autorização |
| Certificado A1 cifrado e validações | Implementado; testado com certificado sintético |
| Confiança ICP | Falha fechada sem bundle; bundle real deve ser provisionado |
| Revogação CRL/OCSP e KMS externo | Pendentes; bloquear produção até definir política |
| Idempotência, numeração, fila, bulk de até 50 | Implementado |
| Geração/assinatura/XSD e prévia DANFE NF-e | Teste local completo, sem efeito fiscal (`sped-nfe` v5.2.11 atualizado) |
| Modo Simplificado Nacional (Smart Defaults) | Implementado: auto-preenchimento de UF/destino, `city_code`, CFOP, impostos Simples/MEI e totais |
| Reforma Tributária (IBS, CBS e IS) e DIFAL | Implementado no DTO Go (`ibs_cbs`, `is`, `icms_uf_dest`) e no motor PHP (`v5.2.11`) |
| Download direto de XML e PDF | Implementado (`GET /v1/documents/{id}/xml` e `/pdf`) |
| Eventos, cancelamento, CC-e, inutilização e DF-e | Rotas e motor implementados (`/cancel`, `/correction-letter`, `/void-range`, `/dfe/sync`, `/sefaz/status`) |
| NFC-e/QR Code/DANFCE | Caminho implementado; exige CSC; não validado em homologação real |
| Transmissão e consulta NF-e/NFC-e | Adaptador implementado, desabilitado por padrão e não homologado |
| Reconciliação depois de resultado desconhecido | Estado e endpoint implementados; consulta nunca retransmite |
| XML, PDF e protocolo | Armazenamento/download implementados; protocolo real depende de autorização |
| S3 compatível | Backend implementado; serviço não exercitado nesta máquina |
| Volume local de objetos | Implementado e testado; compartilhado somente no mesmo host |
| Eventos, cancelamento, CC-e, inutilização | Planejados; nenhuma rota que simule sucesso |
| NFS-e nacional | Repositório baixado e fronteira definida; não integrado |
| CT-e, MDF-e, NFCom, DF-e | Roadmap |
| Webhooks, imports e batch durável | Especificados; ainda não implementados |
| Prometheus | Métricas básicas no processo; overlay opcional |
| OpenTelemetry/Grafana/Loki/Tempo | Plano, não stack completa configurada |
| WAF, TLS/mTLS, KMS gerenciado, HA | Requisitos de implantação; Compose local não entrega isso |

A API não afirma cobertura homologada de todas as UFs. A cobertura declarada por
uma biblioteca não substitui testes por UF, regime, modelo, ambiente e cenário.

DTO fiscal atual: venda normal, endereço/cliente/produtos/pagamentos,
ICMS CST 00 ou CSOSN 102/103/300/400, PIS/COFINS CST 01/02 e 04–09.
Cenários com IBS/CBS/IS e demais tributos precisam de novos campos, mapeamento e
validação. Não usar esta versão para produção na vigência de regras não mapeadas.
