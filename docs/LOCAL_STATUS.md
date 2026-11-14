# Estado local — 30/09/2026

Transmissão habilitada por solicitação do usuário **somente em homologação**:
ENABLE_ISSUANCE=true no .env local. O padrão distribuído continua false.
Produção permanece bloqueada por código. ALLOW_TEST_CERTIFICATES permanece false.

No momento da habilitação não havia trabalhos fiscais pendentes nem certificados
ativos válidos. O trust store ICP está vazio. Portanto a flag está habilitada,
mas ainda não é possível emitir sem configurar a cadeia confiável, a empresa e
um certificado A1 válido (e CSC para NFC-e). Nenhuma transmissão foi realizada.

Repositórios baixados: sped-nfe e sped-da integrados via Composer; open-nfse e
brasil-fiscal/nfe disponíveis para avaliação, sem integração ao fluxo atual.
NFS-e, webhooks e eventos fiscais continuam conforme o roadmap.

A suíte scripts/integration.py exige ENABLE_ISSUANCE=false para seus testes
sintéticos. Não executar essa suíte em um ambiente com emissão real habilitada.
