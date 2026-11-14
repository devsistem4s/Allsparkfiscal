# Guia Completo do Servidor MCP Seguro — Integração com Claude, ChatGPT, Gemini, Cursor e Windsurf

A **AllSpark Fiscal** possui um servidor **Model Context Protocol (MCP)** nativo (`v0.4.0`, compatível com as especificações `2024-11-05` e `2025-03-26`) integrado diretamente na API (`/v1/mcp`), além de ponte local STDIO ([mcp-stdio-bridge.py](file:///home/silane/Documentos/sistema/AllSpark%20Fiscal/scripts/mcp-stdio-bridge.py)) e especificação OpenAPI 3.1 para Actions ([allspark-ai-actions.openapi.json](file:///home/silane/Documentos/sistema/AllSpark%20Fiscal/openapi/allspark-ai-actions.openapi.json)).

Com isso, se o cliente **já possui cadastro** (empresa, certificado A1 e série configurados), qualquer agente de IA autorizado consegue **consultar empresas, verificar prontidão, simular impostos, emitir NF-e/NFC-e, consultar status/XML/PDF, cancelar notas, emitir Carta de Correção e diagnosticar rejeições SEFAZ** por conversa natural.

---

## 1. Arquitetura de Segurança do MCP AllSpark Fiscal

O endpoint `/v1/mcp` foi blindado em 5 camadas para uso corporativo:

1. **Autenticação Obrigatória via Bearer Token (`SHA-256`)**:
   - Nenhuma chamada ao `/v1/mcp` (nem `GET`, nem `POST`) é aceita sem uma chave `Authorization: Bearer af_...` válida e ativa no banco.
2. **Isolamento Absoluto de Tenant (`Multi-Tenant`)**:
   - O `TenantID` nunca vem dos argumentos do prompt da IA: ele é derivado criptograficamente do token autenticado. Um agente de IA jamais consegue listar ou acessar empresas/notas de outro tenant.
3. **Controle Granular de Escopo por Tool (`RBAC por Ferramenta`)**:
   - Cada ferramenta do MCP exige um escopo específico (`companies:read`, `companies:write`, `documents:read`, `documents:write`, `certificates:read`):
     - **Chave Somente-Leitura (`*:read`)**: Ideal para agentes de suporte/atendimento/auditoria. Permite listar empresas, checar prontidão (`get_company_readiness`), simular impostos (`preview_order_invoice`), consultar notas (`get_document_status`, `list_documents`) e diagnosticar erros (`diagnose_fiscal_error`), mas **bloqueia com `FORBIDDEN`** qualquer tentativa de emitir ou cancelar notas!
     - **Chave Operacional Completa (`*:write`)**: Permite ao agente executar todo o ciclo fiscal (emitir notas, cadastrar perfis tributários, cancelar, enviar CC-e e reenviar webhooks).
4. **Sigilo de Segredos e Certificados A1**:
   - O MCP **jamais** expõe o binário `.pfx`, a senha do certificado A1 ou segredos HMAC de webhooks existentes.
5. **Proteção Automática contra Bloqueio SEFAZ (`Escudo Anti-656`)**:
   - Mesmo que um agente de IA tente consultar ou transmitir em loop, o governador Redis e os rate limiters por tenant/IP impedem qualquer "Consumo Indevido" (`cStat 656`) na SEFAZ.

---

## 2. O que o Agente de IA consegue fazer pelo MCP (16 Tools + 4 Resources + 2 Prompts)

### 2.1. Ferramentas (`tools/list` e `tools/call`)
| Tool MCP | Escopo Exigido | O que faz |
|---|---|---|
| `list_companies` | `companies:read` | Lista as empresas emitentes cadastradas no cliente (CNPJ, UF, IE, CRT, contingência). |
| `get_company_readiness` | `companies:read` | **Checklist de Prontidão**: verifica se a empresa tem endereço completo, Certificado A1 válido (e quantos dias faltam para vencer), séries `nfe`/`nfce` ativas, perfis tributários e se está `ready_for_nfe` / `ready_for_nfce`. |
| `create_company` | `companies:write` | Cadastra uma nova empresa emitente com validação de CNPJ, UF, IE e endereço IBGE. |
| `configure_company_fiscal` | `companies:write` | Habilita/atualiza série e ambiente (`homologation` / `production`) para `nfe` ou `nfce`. |
| `list_tax_profiles` | `companies:read` | Lista os perfis tributários (`tax_profile`) configurados na empresa. |
| `upsert_tax_profile` | `companies:write` | Cria ou atualiza um perfil tributário reutilizável (ex.: `REVENDA_PADRAO`, `INDUSTRIA_IBS_CBS`). |
| `preview_order_invoice` | `documents:read` | **Simulador Tributário (Dry-Run)**: calcula CFOP (`5102` vs `6102`), destino e todos os impostos (`ICMS`, `PIS`, `COFINS`, `DIFAL`, `IBS/CBS`) de um pedido sem gravar no banco e sem gastar numeração. |
| `emit_order_invoice` | `documents:write` | **Emissão Simplificada**: emite/gera NF-e (55) ou NFC-e (65) a partir de um pedido (cliente + itens + `tax_profile` + `ref` + `webhook_url`). |
| `emit_complete_document` | `documents:write` | **Emissão Completa Avançada**: emite NF-e/NFC-e com controle total de finalidade (Normal, Complementar, Ajuste, Devolução com `referenced_keys`), DIFAL e Reforma Tributária (`IBSCBS`, `IS`). |
| `get_document_status` | `documents:read` | Consulta status, chave de 44 dígitos, protocolo, URLs de XML/PDF, entregas de webhook e erros explicados por `id` ou `ref`. |
| `list_documents` | `documents:read` | Lista notas fiscais emitidas com filtros por `company_id`, `kind` (`nfe`/`nfce`) e `limit`. |
| `cancel_document` | `documents:write` | Cancela uma NF-e ou NFC-e autorizada (Evento `110111`) por `id` ou `ref`. |
| `send_correction_letter` | `documents:write` | Emite Carta de Correção Eletrônica — CC-e (Evento `110110`) por `id` ou `ref`. |
| `check_expiring_certificates` | `certificates:read` | Lista certificados A1 que vencem nos próximos `N` dias. |
| `manage_webhooks` | `documents:read`/`write` | Lista webhooks ativos (`action=list`), cria novo webhook (`action=create`) ou reenvia webhook (`action=resend`). |
| `diagnose_fiscal_error` | `documents:read` | Traduz qualquer código de erro da API ou `cStat` da SEFAZ informando **qual foi o erro** (`message`) e **o que fazer** (`action`). |
| `lookup_fiscal_table` | `documents:read` | Consulta tabelas de `cfop`, `ncm`, `states` (27 UFs), `document_types`, `cep` e `cnpj`. |

---

## 3. Como Configurar nas Principais IAs (Claude, ChatGPT, Gemini, Cursor e Windsurf)

### 3.1. Claude Desktop (`claude_desktop_config.json`)
O Claude Desktop utiliza o script de ponte STDIO [mcp-stdio-bridge.py](file:///home/silane/Documentos/sistema/AllSpark%20Fiscal/scripts/mcp-stdio-bridge.py) incluído no projeto:

Adicione ao seu arquivo `claude_desktop_config.json`:
```json
{
  "mcpServers": {
    "allspark-fiscal": {
      "command": "python3",
      "args": [
        "/home/silane/Documentos/sistema/AllSpark Fiscal/scripts/mcp-stdio-bridge.py"
      ],
      "env": {
        "ALLSPARK_API_URL": "http://127.0.0.1:8088",
        "ALLSPARK_API_KEY": "af_sua_chave_de_api_aqui"
      }
    }
  }
}
```

### 3.2. Claude Code CLI
No terminal com Claude Code, você pode conectar diretamente via HTTP ou via STDIO:
```bash
# Opção 1: Transporte HTTP Direto
claude mcp add --transport http allspark-fiscal http://127.0.0.1:8088/v1/mcp \
  --header "Authorization: Bearer af_sua_chave_de_api_aqui"

# Opção 2: Transporte STDIO via Bridge
claude mcp add allspark-fiscal \
  -e ALLSPARK_API_URL=http://127.0.0.1:8088 \
  -e ALLSPARK_API_KEY=af_sua_chave_de_api_aqui \
  -- python3 "/home/silane/Documentos/sistema/AllSpark Fiscal/scripts/mcp-stdio-bridge.py"
```

### 3.3. Google Gemini (Gemini CLI / Antigravity / Vertex AI)
No `~/.gemini/settings.json` (ou configuração de MCP do Gemini / Antigravity):
```json
{
  "mcpServers": {
    "allspark-fiscal": {
      "httpUrl": "http://127.0.0.1:8088/v1/mcp",
      "headers": {
        "Authorization": "Bearer af_sua_chave_de_api_aqui"
      }
    }
  }
}
```
*(Ou utilizando o `command: "python3"` com `scripts/mcp-stdio-bridge.py` como no exemplo do Claude Desktop).*

### 3.4. ChatGPT (Custom GPTs Actions & OpenAI Responses API MCP)
1. **Para OpenAI Responses API / Agents SDK (Remote MCP)**:
   ```json
   {
     "model": "gpt-4.1",
     "tools": [
       {
         "type": "mcp",
         "server_label": "allspark_fiscal",
         "server_url": "https://sua-api.com.br/v1/mcp",
         "headers": {
           "Authorization": "Bearer af_sua_chave_de_api_aqui"
         },
         "require_approval": "never"
       }
     ],
     "input": "Verifique se minha empresa está pronta e emita a nota do pedido PEDIDO-2050 para o cliente João."
   }
   ```
2. **Para ChatGPT Custom GPTs (Actions no Painel do ChatGPT)**:
   - Clique em **Create a GPT -> Configure -> Actions -> Import from file**.
   - Selecione o arquivo pronto [allspark-ai-actions.openapi.json](file:///home/silane/Documentos/sistema/AllSpark%20Fiscal/openapi/allspark-ai-actions.openapi.json).
   - Em **Authentication**, escolha **API Key -> Bearer** e cole sua chave `af_...`.

### 3.5. Cursor e Windsurf (`~/.cursor/mcp.json` ou `.cursor/mcp.json`)
```json
{
  "mcpServers": {
    "allspark-fiscal": {
      "command": "python3",
      "args": [
        "/home/silane/Documentos/sistema/AllSpark Fiscal/scripts/mcp-stdio-bridge.py"
      ],
      "env": {
        "ALLSPARK_API_URL": "http://127.0.0.1:8088",
        "ALLSPARK_API_KEY": "af_sua_chave_de_api_aqui"
      }
    }
  }
}
```

---

## 4. Exemplo de Fluxo Completo de Conversa com a IA (Cliente Já Cadastrado)

Quando o usuário pede à IA:
> *"Emita uma NF-e para minha empresa referente ao pedido #8840 de 2 Notebooks (R$ 4.500 cada) para o cliente CPF 111.444.777-35 em Salvador-BA e me avise no webhook."*

O agente de IA executa automaticamente via MCP:
1. **`list_companies`**: Identifica o `company_id` da empresa ativa do cliente.
2. **`get_company_readiness`**: Confirma que `ready_for_nfe == true` e que o certificado A1 está válido.
3. **`preview_order_invoice`**: Simula os impostos e valida que a nota soma `R$ 9.000,00` com `CFOP 5102` e tags da Reforma Tributária (`IBSCBS`).
4. **`emit_order_invoice`**: Enfileira a nota com `"ref": "PEDIDO-8840"` e `"webhook_url": "..."`.
5. **`get_document_status`**: Confirma o status `validated`/`authorized`, a chave de acesso de 44 dígitos e entrega ao usuário os links diretos de download do **XML** (`/v1/documents/ref/PEDIDO-8840/xml`) e do **DANFE PDF** (`/v1/documents/ref/PEDIDO-8840/pdf`).

