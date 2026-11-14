# Motores e referências

| Repositório | Uso nesta base | Última verificação GitHub (14/11/2026) |
|---|---|---|
| https://github.com/nfephp-org/sped-nfe | Runtime: 5.2.8 via Composer | `v5.2.11` disponível (inclui `PL_010f`, NT 2025.002 v1.50 e NT 2026.008 v1.01 da Reforma Tributária) |
| https://github.com/nfephp-org/sped-da | Runtime: 1.1.6 via Composer | Tag `v1.1.6` + 110 commits na `master` (CNPJ alfanumérico, CRT 4 MEI, DANFSe v2) |
| https://github.com/Fm-s/open-nfse | Clone para avaliação de NFS-e nacional; não integrado | `v0.11.1` disponível (RTC v1.01, CNPJ alfanumérico, IBS/CBS e fix de dependências) |
| https://github.com/brasil-fiscal/nfe | Clone para comparação; não integrado | `52dc6df` (TypeScript, Leiaute 4.00 + IBS/CBS + IS) |
| https://github.com/mschunke/gonfe | Avaliado remotamente (Go nativo) | `v0.5.0` (6 stars; sem NTs recentes de 2026 da Reforma Tributária) |

Clones em third_party são referências, não versões vendorizadas do código AllSpark.
Commits exatos: engines/upstream.lock.json. Runtime PHP usa composer.lock e vendor.
`scripts/fetch-upstreams.py` reproduz os commits sem sobrescrever repositório alterado.
Não escolher branch flutuante na produção. Preservar LICENSE/NOTICE dos upstreams;
revisão de licença das versões e dependências é requisito de distribuição.

NFePHP foi escolhido pela separação biblioteca/API, construção XML, assinatura,
comunicação e ecossistema de documentos auxiliares. O adaptador não transfere ao
AllSpark a promessa de atender qualquer cenário sem configuração/testes.
Open-nfse declara operações nacionais e renderer; isso precisa ser verificado em
contratos, XSD e municípios alvo. O clone não prova integração nem cobertura nacional.
A alternativa TypeScript serve para comparar manutenção, extensibilidade, testes,
licença e complexidade operacional. ACBr pode ser avaliado antes de outra decisão;
não foi instalado ou apresentado como componente desta execução.

Referências comerciais para desenho (não dependências):
[Nuvem Fiscal: autenticação](https://dev.nuvemfiscal.com.br/docs/autenticacao/)
usa OAuth2 client_credentials;
[Nuvem Fiscal: NF-e](https://dev.nuvemfiscal.com.br/docs/nfe/)
documenta operações e scopes;
[Focus NFe: integração](https://www.focusnfe.com.br/guides/preparando_sistema/)
discute processamento assíncrono, notificações e limites;
[Focus: webhooks](https://doc.focusnfe.com.br/reference/criar_webhook)
descreve cadastro de destinos/eventos. A conclusão de usar aceitação assíncrona,
autorização por empresa e notificações é uma decisão de arquitetura AllSpark.

Confiabilidade da fila segue os conceitos oficiais de
[RabbitMQ confirms/ACK](https://www.rabbitmq.com/docs/confirms).
Nenhuma API comercial foi contratada nem chamada para emitir documentos.
