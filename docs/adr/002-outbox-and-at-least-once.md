# ADR 002

Status: adotada na fundação de desenvolvimento.

Documento/job/outbox na mesma transação. RabbitMQ com confirms e ACK manual. Duplicação é esperada; claim com estado/lease deduplica. Timeout após transmissão exige reconciliação, não retry de emissão.
