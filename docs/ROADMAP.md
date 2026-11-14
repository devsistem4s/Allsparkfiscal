# Etapas

1. Fundação atual: Docker, API multi-tenant, PostgreSQL/outbox, RabbitMQ, limites,
   envelope, adaptador NFePHP e testes locais.
2. Completar identidade: KEY/usuário/senha, Argon2id, access/refresh rotativos,
   RBAC, CRUD empresas/configurações, auditoria de acessos e revogação.
3. Expandir DTO fiscal para cenários reais, Reforma Tributária, validações e fixtures;
   homologar NF-e em uma UF/empresa com responsável fiscal.
4. Certificados em KMS/Vault, CRL/OCSP, TLS/mTLS, object storage compartilhado,
   restauração testada e observabilidade completa.
5. Eventos/cancelamento/CC-e/inutilização, PDFs independentes e reconciliação automática
   com orçamento/limites por autorizador; não retransmissão cega.
6. Webhooks com HMAC/SSRF, batches duráveis, imports, quotas/fairness e circuit breakers.
7. NFC-e validada com CSC/QR Code/contingência, ampliar matriz de UFs.
8. NFS-e nacional via provider avaliado; municipais conforme cobertura necessária.
9. CT-e, MDF-e, NFCom e DF-e por adapters independentes.
10. Ensaios de carga, falha e segurança para estabelecer SLA/capacidade, HA e operação.

Esta sequência estrutura a implementação completa discutida. Um conjunto de pastas
ou um clone Git não transforma todos esses recursos em funcionalidades prontas.
