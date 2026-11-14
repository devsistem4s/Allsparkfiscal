# Filas e processamento

Filas atuais: `fiscal.jobs` e `fiscal.dead`, ambas duráveis tipo quorum. Em nó único,
quorum não fornece tolerância à perda da máquina. Em produção usar cluster de pelo
menos três nós com discos e domínios de falha independentes.

Mensagem contém só `{job_id}`. Documento e certificado nunca entram no broker.
Dispatcher consulta outbox com FOR UPDATE SKIP LOCKED, publica mensagem persistente,
usa mandatory + publisher confirmation e então confirma published_at. Falha depois
do ACK do broker e antes do commit do banco pode duplicar publicação; o consumer
deve e consegue deduplicar pelo estado do job.

Worker usa prefetch 1, ACK manual só depois da persistência. Em falha de conexão,
reconecta. Falha antes da transmissão: retentativa 2^attempt segundos, até 5 tentativas;
erro permanente vai para estado dead e fila fiscal.dead. Falta de certificado,
dados não suportados ou erro XSD são permanentes. Jitter/circuit breaker por
UF/provedor e redrive administrativo auditado são próximos passos.

Lease de 5 minutos, timeout HTTP interno de 90 segundos. Dispatcher recupera leases
a cada 15 segundos. Trabalho expirado antes da transmissão pode voltar para fila;
na fase transmitting vira reconciliation_required. Nada retransmite automaticamente.
Serialização por empresa/modelo/ambiente usa advisory lock de sessão; operações de
prévia podem executar paralelamente. Ordem exata de entrega fiscal não é garantida.

Backpressure atual: limite de requisições e admissão quando jobs ativos atingem
100 mil. É uma proteção aproximada sob concorrência, não quota rígida; precisa
ser complementada por limites por tenant e medição de lag em produção.

Filas futuras: fiscal.nfe.issue, fiscal.nfce.issue, fiscal.nfse.issue,
fiscal.nfe.cancel, fiscal.nfe.correction, fiscal.pdf.generate e fiscal.webhook.dispatch,
com DLQs próprias. Migrar por versão e tipo de mensagem; nunca compartilhar concorrência
ilimitada com autorizadores lentos. Exactly-once não é uma garantia do RabbitMQ.
