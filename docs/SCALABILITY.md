# Escalabilidade e capacidade

Meta inicial confirmada na conversa: **mais de 50.000 documentos/dia**. Média de
0,579 documento/s. Cenário de expansão 500.000/dia: 5,787/s. Não dimensionar só
pela média: lotes, horário comercial, retries, XML/PDF e indisponibilidade dominam.

Separar três medidas: aceitação HTTP persistida; geração interna XML/PDF;
autorização fiscal externa. Um teste de POST/202 não prova emissões/dia.

Orçamento ilustrativo de concorrência: W ≈ taxa alvo × tempo médio do trabalho.
A 50 trabalhos/s e 2 s/trabalho, são cerca de 100 execuções em voo, antes de margem.
Isso NÃO autoriza 100 chamadas simultâneas à mesma SEFAZ. Dividir por empresa,
UF, modelo e provedor; respeitar limites e backoff. Workers atuais têm prefetch 1.

Escala local:

```sh
docker compose up -d --scale worker=8
```

API local tem porta fixa; não executar --scale api=N sem balanceador e remoção
do mapeamento fixo. Produção: replicar API atrás de LB, S3 compartilhado, pool de
conexões global dimensionado (réplicas × pool), PgBouncer se necessário, RabbitMQ HA,
Redis compartilhado, autoscaling por lag/idade de fila e não apenas CPU.

Exemplo de storage: 50 mil × 150 KB de XML/PDF/resultados ≈ 7,5 GB/dia antes de
réplicas/backups. 500 mil equivale a ≈75 GB/dia nessa hipótese. Medir tamanho real,
retenção e crescimento de índices; preservar capacidade/disco livre de emergência.

Teste disponibilizado: `scripts/load-test.py --requests 1000 --rate 50 --concurrency 20`.
Mede latências e códigos HTTP de aceitação, em tenant sintético sem certificado,
portanto jobs terminam como falha esperada. Não confundir com throughput do motor.
Relatório medido fica em `var/reports/load-test.json`; não extrapolar execução curta
para garantia de 24 horas. Antes de SLA: teste sustentado, picos, recuperação e
homologação com payloads representativos e limites dos autorizadores.
