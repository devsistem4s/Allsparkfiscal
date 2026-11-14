# Segurança

Controles implementados: autorização por tenant e scope; FKs compostas; hash de
API key; rate limit compartilhado com Redis e falha fechada se indisponível;
limites de tamanho/tempo; SQL parametrizado; idempotência; cifragem autenticada por
recurso; upload A1 validado; secrets fora do repositório; containers de aplicação
sem root, filesystem read-only e sem capabilities; redes internas; download
privado; logs sem payload e sem exceções fiscais; auditoria de mutações principais.

Na aplicação, banco usa papel sem superuser/DDL. Migração usa papel separado;
a senha do app não é a senha do proprietário. Auditoria permite INSERT/SELECT,
sem UPDATE/DELETE ao papel da aplicação. Isso não protege contra o administrador
do banco; retenção imutável externa ainda é necessária.

## Fronteira de desenvolvimento

A porta pública do Compose escuta somente em 127.0.0.1. HTTP, AMQP e comunicação
interna sem TLS destinam-se a desenvolvimento na mesma máquina. Secrets do Compose
são bind mounts, não um KMS nem armazenamento criptografado do Docker. Não publicar
essa topologia diretamente na internet. MinIO/S3, quando usado, deve permanecer privado.

## Critérios de liberação para produção

TLS externo e mTLS/rede autenticada entre serviços; WAF/limite de conexões; RBAC
com tokens temporários; KMS/Vault e rotação; CRL/OCSP e trust store mantido;
scans de dependências/imagens, SAST/DAST e teste de invasão; backup cifrado restaurado
em teste; revisão tributária por cenário; homologação por UF; segmentação da
saída SEFAZ; políticas de retenção/LGPD aprovadas; HA e alarmes exercitados.

A ausência desses itens é uma limitação real desta fase. Não chamar a base de
“extremamente segura” ou certificada sem auditoria e validação independentes.

CORS está fechado; não confiar em X-Forwarded-For sem proxy confiável. Limite IP
usa RemoteAddr e pode agrupar clientes sob um mesmo proxy: configurar gateway antes
de escalar externamente. Webhooks futuros exigem proteção SSRF no destino e no dial.
