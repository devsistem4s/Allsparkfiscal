# Superfície futura da API

**Contrato de destino, não lista de rotas implementadas.** O OpenAPI executável
fica em `api/openapi.json`; a proposta está em `openapi/planned.json`.
Todos os endpoints fiscais futuros deverão manter tenant/scopes/idempotência.

| Grupo | Rotas previstas |
|---|---|
| Autenticação | POST /v1/auth/token, /refresh, /revoke |
| Empresas | Implementado; ver COMPANY_API.md |
| Certificado | Implementado; ver COMPANY_API.md |
| Configuração | NF-e/NFC-e implementadas; NFS-e futura |
| NF-e | POST/GET /v1/nfe; GET /v1/nfe/{id} |
| NF-e arquivos | GET /v1/nfe/{id}/xml, /pdf, /files, /events |
| NF-e validação | POST /v1/nfe/validate, sem transmissão e sem reservar número |
| NF-e eventos | POST /v1/nfe/{id}/cancel, /correction-letter; POST /v1/nfe/void-number-range |
| NFC-e | POST/GET /v1/nfce; GET /{id}, /{id}/xml, /{id}/pdf, /{id}/events |
| NFC-e operações | POST /v1/nfce/validate; POST /v1/nfce/{id}/cancel |
| NFS-e | POST/GET /v1/nfse; GET /{id}, /{id}/xml, /{id}/pdf, /{id}/events |
| NFS-e operações | POST /v1/nfse/validate; POST /v1/nfse/{id}/cancel, /replace |
| Genérico | POST/GET /v1/documents; GET /{id}, /{id}/events |
| Batch durável | POST /v1/batches; GET /v1/batches/{id}, /{id}/documents |
| Imports | POST /v1/imports; GET /v1/imports/{id} |
| Webhooks | POST/GET /v1/webhooks; GET/PATCH/DELETE /{id}; GET /{id}/deliveries |

Listagens terão cursor opaco e filtros por empresa, external_id, status, chave e
datas. Limite padrão 50, máximo 100. DELETE de empresa será lógico; histórico fiscal
não será eliminado. Eventos externos retornam 202 e IDs de operação.

Headers previstos: Bearer temporário, X-Company-ID, Idempotency-Key e X-Request-ID.
Se header e JSON indicarem empresas diferentes, rejeitar. Nenhum cliente escolhe
tenant pelo header. Escopos específicos (`nfe:write`, `nfce:read` etc.) serão mapeados
para permissões por recurso. Versionar mudanças incompatíveis e rejeitar campos
não suportados em vez de ignorá-los.

DTO completo deverá incorporar NCM, CEST, CFOP, GTIN, descontos, frete, seguro,
ICMS/ST/FCP/DIFAL, IPI, PIS, COFINS, IBS, CBS, IS, cobrança, transportadora,
intermediador e referências. A conversão ao leiaute/XSD pertence ao adaptador.
