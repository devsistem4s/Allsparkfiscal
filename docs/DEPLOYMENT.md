# Docker e operação

Requisitos: Docker Engine/Compose e Python 3 para bootstrap/testes. Go 1.26+ somente
para desenvolvimento fora dos containers. PHP/Composer são instalados na imagem. O build Docker usa Go 1.27.

```sh
python3 scripts/init-local.py
docker compose up -d --build --scale worker=4
docker compose ps
```

Bootstrap cria segredos aleatórios somente se inexistentes, com umask 077, preserva
`.env` e arquivos existentes. Nunca apagar/recriar master_key em instalação com dados.
Migração configura senha do papel de aplicação usando credencial administrativa
separada, cria esquema e grants. App não é superuser. Secrets não entram em imagens.

Serviços: PostgreSQL 17, RabbitMQ 4.1, Redis 7.4, engine PHP 8.3, api, dispatcher,
worker e migração efêmera. Composer lock fixa bibliotecas; go.sum fixa módulos.
Imagens de infraestrutura ainda devem ser travadas por digest e auditadas na release.

Exposto no host: API em 127.0.0.1:8088. Banco, broker, Redis, engine e métricas não
publicam portas. Rede backend é interna; engine possui rede de saída para futura
SEFAZ; API tem rede de borda local. Nenhum envio fiscal está habilitado por padrão.

## Habilitação de homologação

Primeiro configurar empresas reais, cadeia ICP, A1/CSC, revisar DTO/impostos e
executar cenários de teste fiscal. Só então configurar ENABLE_ISSUANCE=true em API
e engine simultaneamente. `environment=production` continua proibido por código.
Não habilitar transmissão enquanto houver certificados sintéticos configurados.

## Observabilidade opcional

```sh
docker compose -f compose.yaml -f compose.observability.yaml up -d
```

Overlay Prometheus deve ser validado/atualizado pela operação. Ele consulta api:9090;
interface local em 127.0.0.1:9091. Não inclui Grafana/Loki/Tempo.

## Parar

`docker compose stop` preserva dados. `docker compose down` remove containers/redes
mas preserva volumes nomeados. Não usar `down -v` em ambiente com documentos.
Para logs: `docker compose logs --tail=100 api worker dispatcher`; não habilitar
logs de payload. S3, TLS/mTLS, KMS, WAF e cluster HA são etapas de produção.
