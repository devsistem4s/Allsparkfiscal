# Fluxo multi-tenant e credenciais administrativas

## Identidade e autorização

1. Um operador com acesso ao código/ambiente cria o tenant e sua credencial pelo comando administrativo abaixo.
2. O integrador usa `Authorization: Bearer <api_key>` para cadastrar empresas.
3. Cada empresa recebe um UUID. Nas notas, enviar `company_id`, tipo, operação, ambiente, série e dados da venda.
4. A API confere se a empresa pertence ao tenant da credencial. Conhecer o UUID de outra empresa não concede acesso: retorna 404.
5. O worker busca CNPJ, razão social, IE, CRT, UF e endereço no cadastro da empresa; busca o certificado ativo cifrado e o abre internamente. O integrador não envia novamente esses dados na nota.
6. Resultado e arquivos continuam protegidos pelo tenant autenticado e pelos scopes.

Não há cadastro público de usuários, endpoint de emissão de chave ou login/senha nesta versão. `/v1/users`, `/v1/auth/register` e `/v1/auth/token` não são funcionalidades disponíveis. Usuários humanos e sessões não estão modelados: a identidade atual de integração é uma API key vinculada a um tenant. Uma chave de tenant pode acessar as empresas desse tenant conforme seus scopes; não existem permissões individuais por empresa nesta versão.

## Criar acesso somente pelo código administrativo

Na raiz do projeto, com Docker configurado:

```sh
docker compose run --rm --no-deps \
  -e ROLE=tenant-create \
  -e TENANT_NAME='Nome da organização' api
```

O comando devolve `tenant_id` e `api_key`. Guarde a chave em gerenciador de segredos; o banco guarda somente o hash, sem recuperação do original. A saída contém segredo e não deve ser enviada a logs, documentação ou Git. Esse comando cria um tenant novo; não é cadastro de empresa nem cadastro de usuário humano.

Revogação administrativa de uma chave existente, usando seu UUID interno:

```sh
docker compose run --rm --no-deps \
  -e ROLE=key-revoke \
  -e KEY_ID='UUID_INTERNO_DA_CHAVE' api
```

Não publicar esses comandos como serviço HTTP. Restringir acesso ao host, Docker, banco e arquivos de segredos aos operadores autorizados. Rotação autônoma de credenciais por endpoint não está implementada.

## Cadastro completo da empresa

Use `examples/company.json`: `cnpj`, `name`, `uf`, `ie`, `crt` e `address`. Endereço contém `street`, `number`, `district`, `city_code` (7 dígitos), `city`, `state` igual à UF da empresa e `postal_code` (8 dígitos). A validação é estrutural; não consulta Receita, município ou SEFAZ.

Empresas anteriores à migração 003 são preservadas com endereço nulo. Complete com `PATCH /v1/companies/{id}` e `{"address":{...}}` antes de novos documentos. Documentos históricos continuam acessíveis. Reconciliação usa o XML já assinado e não exige recadastrar o endereço histórico.

`invoice.issuer_address` não é mais obrigatório. Se enviado por clientes antigos, é ignorado na geração: prevalece o endereço cadastrado. Dados da operação, destinatário, produtos e tributos continuam sendo enviados a cada nota. A API não calcula automaticamente todos os tributos.

Certificado é cadastrado separadamente em `PUT /v1/companies/{id}/certificate`, validado e armazenado cifrado com senha e CSC. A configuração de modelo/série/ambiente fica em `/v1/companies/{id}/fiscal/nfe` ou `/nfce`. Durante trabalhos ativos, alterações cadastrais/certificado são bloqueadas para preservar consistência.

## Segurança e limites verificados

Bearer obrigatório, scopes por recurso, filtros por tenant, JSON com campos desconhecidos recusados, limite de corpo, limitação de requisições, idempotência, certificado cifrado e arquivos privados. Testes incluem UUID de empresa de outro tenant, chaves somente leitura e tentativa de sobrescrever endereço na nota.

Esses controles não substituem implantação com TLS, proteção do host, backup, rotação das chaves de criptografia e homologação fiscal. Acesso ao banco/host administrativo não é isolado pelas mesmas barreiras HTTP. Produção permanece bloqueada; não há promessa de segurança absoluta ou cobertura fiscal completa.
