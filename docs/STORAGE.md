# Arquivos e armazenamento

Metadados no PostgreSQL: ID, tenant, documento, tipo, MIME, SHA-256 e tamanho.
Conteúdo cifrado fora do banco. Tipos atuais: signed_xml, preview_pdf, protocol_xml,
authorized_xml e danfe_pdf. `preview_pdf` não é comprovante de autorização.

Backend padrão local: `var/objects/UUID` com permissões restritas; todos os workers
no mesmo host montam o volume. API monta read-only e só retorna bytes após query
com tenant/documento/artefato. Conteúdo é envelope AES-256-GCM; AAD vincula contexto.

Backend opcional: `STORAGE_DRIVER=s3`, `S3_ENDPOINT` (host:porta), `S3_REGION`,
`S3_BUCKET`, `S3_ACCESS_KEY`, `S3_SECRET_KEY_FILE`, `S3_TLS=true` por padrão.
Use o overlay `compose.s3.yaml`, grave a credencial real em
`var/secrets/s3_secret_key` (chmod 600) e configure as variáveis S3 no `.env`.
Não basta definir STORAGE_DRIVER no shell sem passá-la ao container.
Bucket deve existir, privado; cliente de runtime precisa apenas GetObject/PutObject
no prefixo autorizado. Chave de objeto: tenant/document/id. Nunca publicar bucket.
A criptografia ocorre no app, podendo ser combinada com SSE-KMS do storage.

S3/MinIO deve ser compartilhado entre hosts antes de escalar workers para múltiplas
máquinas. Nesta entrega o backend S3 compila, mas não foi testado contra um serviço
real. As imagens MinIO consultadas não ficaram disponíveis no registry desta sessão;
não foi instalado um serviço substituto silenciosamente. Provisionar versão suportada
e auditada ou endpoint S3 antes da migração. O filesystem local permanece funcional.

Escrita de objeto precede transação de metadados. Queda pode deixar órfãos, não pode
expor arquivo sem metadado/autorização. Implementar coleta de órfãos com janela de
segurança e inventário, sem apagar histórico válido. Falha de persistência após envio
resulta em reconciliação; nunca reeditar XML já autorizado.

Retenção fiscal depende do documento e obrigações aplicáveis; definir política com
responsável fiscal, incluindo backup, legal hold e exclusão autorizada. Não inventar
prazo universal. Backup deve incluir banco, objetos e recuperação da KEK em canal
separado; testar restauração e integridade pelo hash.
