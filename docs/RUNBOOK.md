# Runbook

## Outbox ou fila crescendo

Verificar saúde PostgreSQL/RabbitMQ, logs de dispatcher e workers, idade da fila,
consumo e disco. Não apagar fila/outbox. Confirmação do broker precede published_at;
reiniciar dispatcher é seguro contra perda, podendo gerar duplicatas deduplicáveis.

## Worker interrompido

Lease expira em 5 minutos. Recuperação roda a cada 15 segundos. Antes de transmitir,
job pode ser reencaminhado. Depois do checkpoint transmitting, consultar documento:
reconciliation_required → POST /v1/documents/{id}/reconcile. Não transformar esse
estado em queued manualmente para reenviar. Chave, XML e protocolo são essenciais.

## DLQ

Inspecionar last_error e corrigir causa. Não consumir/republicar tudo automaticamente.
Não há endpoint de redrive nesta fase. Registrar intervenção e nunca reutilizar número
para outro payload. Falhas permanentes exigem correção de dados/configuração.

## Certificados

Conferir CNPJ/validade/cadeia/CSC. Nunca solicitar PFX/senha por log ou ticket.
Troca de A1 usa endpoint autenticado e desativa anterior. Alertas e revogação
administrativa ainda precisam de implementação. KMS externo é necessário para
rotação operacional sem distribuir KEK em arquivos.

## Backup/restore

Backup consistente PostgreSQL + objetos + referência de versão/KEK. Guardar KEK em
canal separado e acesso restrito. Restauração precisa confirmar metadados/hash,
decifragem e que jobs transmitting não sejam reenviados. RPO/RTO só podem ser
publicados após ensaio. Não há garantia de backup somente por usar volume Docker.

## Dependências

Atualizar em branch, revisar notas técnicas/schemas e licenças, regenerar lock,
rodar testes e homologar. Não editar third_party esperando alterar runtime Composer.
Não trocar imagens em produção sem revisão de digest e vulnerabilidades.
