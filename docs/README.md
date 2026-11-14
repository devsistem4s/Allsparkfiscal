# Documentação AllSpark Fiscal

Documentação consolidada em 28/09/2026. Leia o estado antes de tratar qualquer
endpoint do desenho futuro como funcional.

| Documento | Conteúdo |
|---|---|
| [MANUAL_PASSO_A_PASSO_INTEGRACAO_E_TIPOS_DE_NOTAS](MANUAL_PASSO_A_PASSO_INTEGRACAO_E_TIPOS_DE_NOTAS.md) | **Manual Passo a Passo (Do Zero à 1ª Nota), Todos os Tipos e Finalidades de Notas Emitidas e Guia de Integração (E-commerce, PDV, ERP, SaaS)** |
| [GUIA_MCP_CLAUDE_CHATGPT_GEMINI](GUIA_MCP_CLAUDE_CHATGPT_GEMINI.md) | **Guia Completo e Seguro do Servidor MCP (`/v1/mcp` + Bridge STDIO) para Claude, ChatGPT, Gemini, Cursor e Windsurf** |
| [GUIA_COMPLETO_API_E_ERROS](GUIA_COMPLETO_API_E_ERROS.md) | **Documentação estilo Focus NFe: Introdução, Todos os Endpoints, Webhooks (`webhook_url`), MCP (`/v1/mcp`) e Tabela Completa de Erros ("Qual foi o erro" e "O que fazer")** |
| [../openapi/allspark-fiscal.postman_collection.json](../openapi/allspark-fiscal.postman_collection.json) | **Postman Collection oficial v2.1 com todas as pastas, payloads prontos e exemplos de resposta** |
| [../openapi/allspark-ai-actions.openapi.json](../openapi/allspark-ai-actions.openapi.json) | **OpenAPI 3.1 para ChatGPT Custom GPTs (Actions), OpenAI Assistants e Google Gemini Extensions** |
| [ANALISE_CONCORRENTES_ESCALABILIDADE_SWARM](ANALISE_CONCORRENTES_ESCALABILIDADE_SWARM.md) | **Benchmark (Focus NFe, NotaaS, Digisan, Spedy, PlugNotas), Erros Acionáveis, Limites SEFAZ (NT 2018.002), Filas 50k+ notas/dia e Docker Swarm** |
| [ANALISE_GERACAO_SIMPLIFICADA](ANALISE_GERACAO_SIMPLIFICADA.md) | **Análise completa: Feito, Pronto, Pendente, Endpoints e Geração Simplificada Nacional** |
| [../infra/README](../infra/README.md) | **Infraestrutura Docker Swarm (`docker-stack.yml`, `swarm-deploy.sh`) e Dimensionamento de Alta Escala** |
| [LOCAL_STATUS](LOCAL_STATUS.md) | Configuração atual desta instalação |
| [IMPLEMENTATION_STATUS](IMPLEMENTATION_STATUS.md) | Implementado, testado, condicionado e futuro |
| [ARCHITECTURE](ARCHITECTURE.md) | Componentes e fronteiras |
| [FISCAL_FLOW](FISCAL_FLOW.md) | Ciclo completo, estados e recuperação |
| [API](API.md) | Contrato atual e exemplos |
| [API_ROADMAP](API_ROADMAP.md) | Todos os endpoints previstos na conversa |
| [AUTHENTICATION](AUTHENTICATION.md) | Chaves atuais e plano KEY/usuário/senha/token |
| [MULTI_TENANCY](MULTI_TENANCY.md) | Isolamento e autorização |
| [CERTIFICATES](CERTIFICATES.md) | A1, confiança e criptografia |
| [SECURITY](SECURITY.md) | Controles e requisitos para produção |
| [THREAT_MODEL](THREAT_MODEL.md) | Ameaças e controles |
| [DATABASE](DATABASE.md) | ERD e transações |
| [QUEUES](QUEUES.md) | Outbox, ACK, retries e DLQ |
| [WEBHOOKS](WEBHOOKS.md) | Contrato planejado e proteção contra SSRF |
| [STORAGE](STORAGE.md) | Arquivos, S3 e retenção |
| [SCALABILITY](SCALABILITY.md) | Dimensionamento, picos e carga |
| [DEPLOYMENT](DEPLOYMENT.md) | Docker e operação local |
| [OBSERVABILITY](OBSERVABILITY.md) | Métricas, logs e plano de traces |
| [VALIDATION_REPORT](VALIDATION_REPORT.md) | Resultados medidos e limites |
| [TESTING](TESTING.md) | Testes reais e limites da evidência |
| [PROVIDERS](PROVIDERS.md) | Repositórios, licenças e critérios de seleção |
| [RUNBOOK](RUNBOOK.md) | Incidentes, recuperação, backup e rotação |
| [ROADMAP](ROADMAP.md) | Sequência de implementação |
| [adr/](adr/) | Decisões arquiteturais |
| [reference/](reference/) | Referência parcial da conversa original |

A referência de conversa é truncada pela ferramenta de leitura a 20 mil caracteres.
Ela é material histórico. Estes documentos consolidam os requisitos recuperados e
registram explicitamente diferenças entre o desenho final e a implementação atual.
