# Certificados A1

POST /v1/companies/{id}/certificates aceita PFX/P12 em base64 e senha, e CSC/CSCid
opcionais de NFC-e. Empresa deve pertencer ao tenant. O adaptador verifica abertura,
validade temporal, CNPJ, correspondência entre chave privada e certificado e cadeia
contra um bundle confiável. A API compara CNPJ exato com a empresa cadastrada.

O arquivo `var/secrets/icp_ca` começa vazio por decisão de falha fechada. Provisione
um bundle de raízes ICP-Brasil obtido e verificado pela operação, com intermediárias
quando necessário. Cadeias no PFX são material não confiável; não se tornam raízes
confiáveis automaticamente. Certificado sem confiança é recusado.

Armazenamento: payload PFX+senha+CSC cifrado por DEK aleatória de 32 bytes;
DEK cifrada com KEK local de 32 bytes; AES-256-GCM em ambas as camadas; AAD inclui
tenant, empresa e ID do certificado. Versão 2 do envelope é prefixada no blob.
Fingerprint, expiração, empresa, tenant, ativo e timestamps ficam como metadados.
Metadados adicionais (serial/issuer/subject/valid_from) estão no modelo de destino.

A KEK é um secret de arquivo fora do PostgreSQL e do Git. Não perdê-la: banco sem
chave não permite recuperar certificados. KMS/Vault gerenciado e rotação online
por key_id são próximos passos, não integrações prontas. O leitor mantém
compatibilidade com envelopes v1 desta base local; novas gravações usam v2.

Certificado substituído é desativado em transação com lock da empresa e índice de
um ativo. Certificados vencidos não são carregados pelo worker. Não há endpoint
para retornar PFX/senha. Material é decifrado apenas em memória; Go/PHP não garantem
apagamento físico imediato de todas as cópias de memória.

CRL/OCSP, comprovação de titularidade empresarial, alertas de expiração, revogação
administrativa e política de retenção precisam ser concluídos antes de produção.
`ALLOW_TEST_CERTIFICATES` é só para testes isolados; se ativo, o adaptador bloqueia
transmissão independentemente de ENABLE_ISSUANCE. No Compose normal permanece false.
