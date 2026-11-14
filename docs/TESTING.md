# Testes e evidência

- `go test -race ./...`: validação de entradas, criptografia e isolamento de contexto.
- `go vet ./...`: análise estática Go.
- `sh scripts/test-engine.sh`: certificado sintético em container sem rede;
  extração CNPJ, assinatura/XML/XSD, PDF e bloqueios de transmissão/confiança.
- `python3 scripts/integration.py`: serviços reais Docker, dois tenants, IDOR,
  concorrência/idempotência, numeração não consumida por prévia, DLQ/falha permanente,
  certificado cifrado, geração via worker, arquivos cifrados e download com hash.
- `python3 scripts/test-recovery.py`: interrompe apenas os workers locais, injeta
  leases expirados e verifica requeue antes do envio e reconciliação depois dele;
  restaura a quantidade anterior de workers em finally.
- `python3 scripts/load-test.py`: cenário configurável de entrada; não emissão real.

O teste integrado exige trust store vazio e recusa substituir um existente. Insere
CA sintética apenas durante o teste local, restaura em finally, cria somente tenants
marcados integration. Não usar testes de integração/carga em instalação real.
Nenhum certificado real foi pedido ou utilizado. Nenhuma chamada SEFAZ foi feita.

Relatórios locais ficam em var/reports; o relatório versionável de validação
resume resultados sem tokens nem certificados. Validar também restore, falha real do
broker/banco, falhas de rede durante transmissão e retenção em uma próxima bateria.

Antes de produção: matriz por UF/modelo/regime, contingência, CSC/QR Code,
IBS/CBS/IS conforme leiaute vigente, eventos, testes de contratos/segurança,
revogação de certificado, concorrência de numeração e soak test sustentado.
