# Modelo de ameaças

| Ameaça | Controle atual | Trabalho adicional |
|---|---|---|
| Trocar company_id para acessar outra empresa | Tenant em queries/FKs; testes IDOR | RLS e testes contínuos |
| Vazamento do banco | PFX/senha e objetos com envelope; API key por hash | KMS, backup seguro e rotação |
| Banco + KEK comprometidos | Separação de secrets/containers | KMS externo com política por serviço |
| Certificado de outro CNPJ | Conferência de titularidade/cadeia/validade | Onboarding empresarial e revogação |
| Reenvio HTTP concorrente | Idempotência transacional por tenant/hash | Política de retenção de chaves |
| Queda após envio à SEFAZ | Checkpoint + reconciliation_required | Homologar falhas reais por autorizador |
| XML externo malicioso | API aceita DTO; adaptador rejeita DTD/ENTITY, NONET | Fuzzing dos builders e bibliotecas |
| Vazamento em logs | Erros controlados, sem payload/secret | Redaction central e scans em CI |
| Saturação de entrada | Rate limit, tamanho máximo e limite de backlog | Quotas por tenant, WAF e fairness |
| Abuso do motor interno | Rede privada e token próprio | mTLS, políticas de egress |
| SSRF de webhook | Nenhum envio implementado | DNS/IP pinning, bloqueio de redes privadas, sem redirects |
| Dependência comprometida | Composer lock, go.sum, commits registrados | Assinaturas/SBOM/scanning e revisão de upgrades |
| Administrador malicioso | Auditoria aplicacional | Trilha imutável externa e segregação de funções |

Assets: certificados, senhas, dados fiscais, credenciais, números e protocolos.
Limites de confiança: internet → API; API → dados; fila → worker; worker → motor;
motor → autorizador; serviço → object storage. UUIDs são identificadores, não autorização.
