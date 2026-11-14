# Multi-tenancy

Tenant representa ERP/integração e contém diversas empresas. ID de empresa não é
credencial. A cadeia de autorização é key → tenant → scope → empresa/documento.

Todos os acessos HTTP a empresas, certificado, documentos e artefatos filtram
`tenant_id`. FKs compostas impedem gravar documento/certificado para empresa de
outro tenant. Recursos de outro tenant retornam 404 para não divulgar existência;
escopo insuficiente retorna 403. Esta é uma escolha explícita frente ao exemplo
403 para empresa de outra integração apresentado na conversa.

A chave de idempotência atual é única **por tenant**; reutilizá-la em outra empresa
com conteúdo diferente gera 409. Isso é mais restritivo que a chave de destino
(tenant + empresa + operação + chave) e deve ser preservado ou migrado de forma
versionada. Não permitir migração silenciosa que duplique operações antigas.

Workers possuem acesso global operacional, pois precisam processar tenants;
carregam tenant/empresa a partir de jobs persistidos, não da mensagem externa.
RabbitMQ contém apenas job_id. Objetos têm AAD criptográfica tenant/documento/id:
trocar um ciphertext de contexto impede decifragem mesmo com acesso ao armazenamento.

Limite atual por tenant e IP. Fair scheduling, quotas de backlog por tenant,
conexões segregadas e RLS PostgreSQL ainda não estão implementados. RLS é uma
camada adicional planejada; não assumir que já existe. Testes negativos de IDOR
cobrem empresa, certificado, documento e arquivo.
