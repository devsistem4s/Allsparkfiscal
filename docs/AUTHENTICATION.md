# Autenticação e autorização

## Atual

Uma API key aleatória de 256 bits identifica o tenant. Só o digest SHA-256 fica no
banco; a chave de alta entropia não é senha humana. Criação por comando operacional
`ROLE=tenant-create`, sem endpoint de cadastro aberto. Scopes atuais:
`companies:write`, `certificates:write`, `documents:write`, `documents:read`.
Chaves revogadas deixam de autenticar imediatamente: não há cache de autenticação.

Revogar por `ROLE=key-revoke` com `KEY_ID`. Consultar o ID pelo banco administrativo;
não confundir ID de tenant com ID da chave. A criação inicial não é um sistema
completo de usuários/RBAC e a key atual não expira automaticamente.

## Fluxo especificado para evolução

KEY + username + password → POST /v1/auth/token → access token 15 minutos + refresh.
Senhas com Argon2id, parâmetros versionados e limite de tentativas por conta/IP/key.
Guardar refresh apenas como hash; rotação atômica em cada uso; detectar reutilização
e revogar a família. Revoke deve invalidar sessão/refresh e impedir reutilização.
Tokens devem vincular tenant, sujeito, scopes, audiência, expiração e identificador.
Usar biblioteca/OIDC estabelecidos para validação; rejeitar algoritmos não permitidos.

Separar usuários, papéis, permissões, credenciais de integração e sessões. Preparar
client_credentials para machine-to-machine. Não alegar OAuth2 enquanto esse fluxo
não existir. A comparação com APIs comerciais está em PROVIDERS.md.

Sem cookies na API atual: não há sessão de navegador; CORS não é habilitado por
padrão. Um futuro console exige decisões específicas de CSRF, sessão e MFA.
