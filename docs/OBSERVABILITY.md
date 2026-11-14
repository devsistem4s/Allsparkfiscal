# Observabilidade

Métricas básicas em `api:9090/metrics`, porta interna sem publicação no host:
allspark_http_requests_total, allspark_http_duration_seconds_sum/count,
allspark_jobs{state}, allspark_metrics_database_up. A duração cobre o handler HTTP;
é agregado local do processo, não percentil ou duração do autorizador.

Prometheus opcional em `compose.observability.yaml`. Não colocar tenant_id, CNPJ,
document_id ou request_id em labels: cardinalidade e dados sensíveis. Logs incluem
identificadores técnicos apenas quando necessários; biblioteca fiscal não expõe
exception/stack ao cliente. Requests recebem X-Request-ID.

Próximos instrumentos: histogramas de HTTP/worker/SEFAZ, lag/idade da outbox, DLQ,
certificados próximos do vencimento, sucesso/retry de webhook e traces OpenTelemetry
com contexto propagado pela outbox. Grafana/Loki/Tempo não estão configurados nesta fase.

Alertas planejados: outbox sem drenar; lease expirado recorrente; aumento de
reconciliation_required; DLQ crescendo; falha de certificado/armazenamento;
disco/backup/replicação; taxa de rejeição por código; indisponibilidade por UF.
Saúde do processo, prontidão de entrada e prontidão fiscal são medidas diferentes.
