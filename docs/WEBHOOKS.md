# Webhooks — desenho ainda não implementado

Eventos previstos: document.authorized, document.rejected, document.failed,
document.cancelled, document.validated. Uma transação de mudança de estado deve
registrar evento durável/outbox. Worker separado envia com entrega at-least-once.

```json
{"id":"UUID_EVENTO","type":"document.authorized","occurred_at":"2026-09-28T12:00:00Z","data":{"document_id":"UUID","company_id":"UUID","status":"authorized"}}
```

Headers: X-AllSpark-Event-ID, X-AllSpark-Timestamp e
X-AllSpark-Signature: sha256=HMAC-SHA256(secret, timestamp + "." + raw_body).
Segredo por endpoint, mostrado uma vez e cifrado. Destinatário valida timestamp,
assinatura em tempo constante e deduplica event_id; retorna 2xx rápido.

Não incluir certificado, senha, XML/PDF completo nem link permanente público.
Cliente consulta API autenticada após o evento. Filtrar endpoint e empresa pelo tenant.

Somente HTTPS com certificado válido; bloquear loopback, redes privadas, link-local,
multicast, metadata cloud e IPs reservados em IPv4/IPv6. Resolver DNS e validar todos
os IPs; fixar IP validado no dial para impedir rebinding; não seguir redirects.
Aplicar política de egress e limites de porta/tamanho/tempo. URL fornecida pelo cliente
jamais deve ser usada diretamente por um cliente HTTP genérico.

Retentativas propostas: backoff exponencial com jitter, orçamento máximo e DLQ;
429/5xx temporários, 410 desativa endpoint mediante política; histórico sanitizado,
redrive auditado, secret rotation com janela de coexistência. Rejeição fiscal não
é falha de entrega do webhook.
