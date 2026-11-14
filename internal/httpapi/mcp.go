package httpapi

import (
	"allspark.local/fiscal/internal/engine"
	"allspark.local/fiscal/internal/fiscal"
	"allspark.local/fiscal/internal/store"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

func hasScope(p store.Principal, scope string) bool {
	if p.Can(scope) {
		return true
	}
	if strings.HasSuffix(scope, ":read") {
		return p.Can(strings.TrimSuffix(scope, ":read") + ":write")
	}
	return false
}

func mcpToolsCatalog() []map[string]any {
	return []map[string]any{
		{
			"name":          "list_companies",
			"description":   "Lista as empresas emitentes cadastradas no tenant autenticado (CNPJ, UF, IE, CRT, modo de contingência e endereço).",
			"requiredScope": "companies:read",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"limit": map[string]any{"type": "integer", "description": "Quantidade máxima de empresas (1 a 100, padrão 50)."},
				},
			},
		},
		{
			"name":          "get_company_readiness",
			"description":   "Verifica se uma empresa cadastrada está 100% pronta para emitir NF-e (mod 55) e NFC-e (mod 65): checa endereço, validade do Certificado Digital A1, séries fiscais habilitadas, perfis tributários, webhooks e lista passos pendentes.",
			"requiredScope": "companies:read",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"company_id": map[string]any{"type": "string", "description": "UUID da empresa emitente."},
				},
				"required": []string{"company_id"},
			},
		},
		{
			"name":          "create_company",
			"description":   "Cadastra uma nova empresa emitente com CNPJ, razão social, UF, Inscrição Estadual (IE), regime tributário (CRT 1, 2 ou 3) e endereço completo.",
			"requiredScope": "companies:write",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"cnpj":             map[string]any{"type": "string", "description": "CNPJ de 14 caracteres."},
					"name":             map[string]any{"type": "string", "description": "Razão Social da empresa."},
					"uf":               map[string]any{"type": "string", "description": "Sigla da UF (ex.: BA, SP, RJ, MG)."},
					"ie":               map[string]any{"type": "string", "description": "Inscrição Estadual numérica ou ISENTO."},
					"crt":              map[string]any{"type": "integer", "description": "Código de Regime Tributário: 1 (Simples Nacional), 2 (Simples Excesso) ou 3 (Regime Normal)."},
					"contingency_mode": map[string]any{"type": "string", "description": "Modo de contingência: auto, off, svc ou offline."},
					"address": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"street":      map[string]any{"type": "string"},
							"number":      map[string]any{"type": "string"},
							"district":    map[string]any{"type": "string"},
							"city_code":   map[string]any{"type": "string", "description": "Código IBGE do município com 7 dígitos (ex.: 2927408)."},
							"city":        map[string]any{"type": "string"},
							"state":       map[string]any{"type": "string"},
							"postal_code": map[string]any{"type": "string", "description": "CEP de 8 dígitos numéricos."},
						},
						"required": []string{"street", "number", "district", "city_code", "city", "state", "postal_code"},
					},
				},
				"required": []string{"cnpj", "name", "uf", "ie", "crt", "address"},
			},
		},
		{
			"name":          "configure_company_fiscal",
			"description":   "Habilita ou atualiza a configuração fiscal (série e ambiente homologation/production) de uma empresa para NF-e (nfe) ou NFC-e (nfce).",
			"requiredScope": "companies:write",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"company_id":  map[string]any{"type": "string", "description": "UUID da empresa emitente."},
					"kind":        map[string]any{"type": "string", "description": "Modelo fiscal: nfe (modelo 55) ou nfce (modelo 65)."},
					"environment": map[string]any{"type": "string", "description": "Ambiente: homologation ou production."},
					"series":      map[string]any{"type": "integer", "description": "Série fiscal (1 a 889)."},
					"enabled":     map[string]any{"type": "boolean", "description": "true para habilitar a emissão."},
				},
				"required": []string{"company_id", "kind", "series"},
			},
		},
		{
			"name":          "list_tax_profiles",
			"description":   "Lista os perfis tributários reutilizáveis (tax_profile) cadastrados para uma empresa.",
			"requiredScope": "companies:read",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"company_id": map[string]any{"type": "string", "description": "UUID da empresa."},
				},
				"required": []string{"company_id"},
			},
		},
		{
			"name":          "upsert_tax_profile",
			"description":   "Cria ou atualiza um perfil tributário reutilizável (ex.: REVENDA_PADRAO, INDUSTRIA_IBS_CBS) na empresa para simplificar a emissão de notas.",
			"requiredScope": "companies:write",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"company_id":      map[string]any{"type": "string"},
					"name":            map[string]any{"type": "string", "description": "Código identificador do perfil (ex.: REVENDA_PADRAO)."},
					"description":     map[string]any{"type": "string"},
					"ncm_default":     map[string]any{"type": "string", "description": "NCM padrão de 8 dígitos."},
					"cfop_internal":   map[string]any{"type": "string", "description": "CFOP padrão estadual (ex.: 5102)."},
					"cfop_interstate": map[string]any{"type": "string", "description": "CFOP padrão interestadual (ex.: 6102)."},
					"taxes":           map[string]any{"type": "object", "description": "Regras de ICMS, PIS, COFINS, DIFAL (icms_uf_dest) e Reforma Tributária (ibs_cbs, is)."},
				},
				"required": []string{"company_id", "name", "taxes"},
			},
		},
		{
			"name":          "preview_order_invoice",
			"description":   "Simula e valida um pedido fiscal (dry-run sem persistir e sem consumir numeração), retornando CFOP calculado, destino da operação e todos os totais de ICMS, PIS/COFINS, DIFAL e Reforma Tributária (IBS/CBS/IS).",
			"requiredScope": "documents:read",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"company_id":  map[string]any{"type": "string", "description": "UUID da empresa emitente."},
					"kind":        map[string]any{"type": "string", "description": "nfe ou nfce (opcional, detectado automaticamente)."},
					"tax_profile": map[string]any{"type": "string", "description": "Nome do perfil tributário opcional (ex.: REVENDA_PADRAO)."},
					"customer":    map[string]any{"type": "object", "description": "Dados do destinatário (tax_id, name, ie_indicator, address)."},
					"items": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"sku":         map[string]any{"type": "string"},
								"description": map[string]any{"type": "string"},
								"ncm":         map[string]any{"type": "string"},
								"cfop":        map[string]any{"type": "string"},
								"quantity":    map[string]any{"type": "string"},
								"price":       map[string]any{"type": "string"},
							},
						},
					},
				},
				"required": []string{"company_id", "items"},
			},
		},
		{
			"name":          "emit_order_invoice",
			"description":   "Emite ou gera uma NF-e (modelo 55) ou NFC-e (modelo 65) de forma simplificada a partir de um pedido para uma empresa já cadastrada. Suporta referência externa (ref), perfil tributário (tax_profile) e link de retorno (webhook_url).",
			"requiredScope": "documents:write",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"company_id":     map[string]any{"type": "string", "description": "UUID da empresa emitente."},
					"ref":            map[string]any{"type": "string", "description": "Referência única do pedido no sistema do cliente (ex.: PEDIDO-1001)."},
					"operation":      map[string]any{"type": "string", "description": "generate (gera e valida XML/PDF sem transmitir) ou issue (transmite à SEFAZ)."},
					"kind":           map[string]any{"type": "string", "description": "nfe (55) ou nfce (65)."},
					"tax_profile":    map[string]any{"type": "string", "description": "Nome do perfil tributário cadastrado na empresa."},
					"webhook_url":    map[string]any{"type": "string", "description": "URL para receber o webhook assinado quando a nota for aprovada/validada/rejeitada."},
					"webhook_secret": map[string]any{"type": "string", "description": "Segredo HMAC-SHA256 opcional para o webhook desta nota."},
					"customer":       map[string]any{"type": "object", "description": "Destinatário (tax_id, name, ie_indicator, ie, address)."},
					"items":          map[string]any{"type": "array", "description": "Lista de itens do pedido (sku, description, ncm, cfop, quantity, price)."},
				},
				"required": []string{"company_id", "items"},
			},
		},
		{
			"name":          "emit_complete_document",
			"description":   "Emite ou gera uma NF-e (55) ou NFC-e (65) utilizando o contrato fiscal completo (suporta nota normal, complementar, ajuste, devolução com referenced_keys, exportação, DIFAL, Reforma Tributária IBS/CBS e Imposto Seletivo).",
			"requiredScope": "documents:write",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"company_id":     map[string]any{"type": "string"},
					"kind":           map[string]any{"type": "string", "description": "nfe ou nfce."},
					"operation":      map[string]any{"type": "string", "description": "generate ou issue."},
					"series":         map[string]any{"type": "integer"},
					"ref":            map[string]any{"type": "string"},
					"webhook_url":    map[string]any{"type": "string"},
					"webhook_secret": map[string]any{"type": "string"},
					"invoice":        map[string]any{"type": "object", "description": "Objeto completo da nota (nature, purpose, direction, destination, referenced_keys, recipient, items, payments)."},
				},
				"required": []string{"company_id", "invoice"},
			},
		},
		{
			"name":          "get_document_status",
			"description":   "Consulta o status atual, chave de acesso de 44 dígitos, protocolo SEFAZ, URLs de XML/PDF, entregas de webhook e erros explicados de uma nota fiscal por ID (UUID) ou referência externa (ref).",
			"requiredScope": "documents:read",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":  map[string]any{"type": "string", "description": "UUID do documento fiscal."},
					"ref": map[string]any{"type": "string", "description": "Referência externa do pedido/nota (ex.: PEDIDO-1001)."},
				},
			},
		},
		{
			"name":          "list_documents",
			"description":   "Lista notas fiscais emitidas ou em processamento no tenant, com filtros opcionais por company_id e kind (nfe/nfce).",
			"requiredScope": "documents:read",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"company_id": map[string]any{"type": "string", "description": "Filtrar por UUID da empresa."},
					"kind":       map[string]any{"type": "string", "description": "Filtrar por modelo: nfe ou nfce."},
					"limit":      map[string]any{"type": "integer", "description": "Máximo de registros (1 a 100, padrão 20)."},
				},
			},
		},
		{
			"name":          "cancel_document",
			"description":   "Cancela uma NF-e ou NFC-e autorizada na SEFAZ (Evento 110111) por id ou ref, exigindo justificativa entre 15 e 255 caracteres.",
			"requiredScope": "documents:write",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":     map[string]any{"type": "string", "description": "UUID da nota fiscal."},
					"ref":    map[string]any{"type": "string", "description": "Referência externa da nota fiscal."},
					"reason": map[string]any{"type": "string", "description": "Justificativa do cancelamento (15 a 255 caracteres)."},
				},
				"required": []string{"reason"},
			},
		},
		{
			"name":          "send_correction_letter",
			"description":   "Emite uma Carta de Correção Eletrônica — CC-e (Evento 110110) para uma NF-e (modelo 55) autorizada, por id ou ref.",
			"requiredScope": "documents:write",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":         map[string]any{"type": "string", "description": "UUID da NF-e."},
					"ref":        map[string]any{"type": "string", "description": "Referência externa da NF-e."},
					"correction": map[string]any{"type": "string", "description": "Texto da correção (15 a 1000 caracteres)."},
					"sequence":   map[string]any{"type": "integer", "description": "Número sequencial do evento CC-e (1 a 20, padrão 1)."},
				},
				"required": []string{"correction"},
			},
		},
		{
			"name":          "check_expiring_certificates",
			"description":   "Monitora e lista todos os Certificados Digitais A1 das empresas do tenant que vencem nos próximos N dias.",
			"requiredScope": "certificates:read",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"days": map[string]any{"type": "integer", "description": "Janela de dias para alerta de vencimento (padrão 30)."},
				},
			},
		},
		{
			"name":          "manage_webhooks",
			"description":   "Gerencia webhooks de notificação automática: lista webhooks ativos (action=list), cadastra novo webhook (action=create) ou reenvia o webhook de uma nota (action=resend).",
			"requiredScope": "documents:read",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action":      map[string]any{"type": "string", "description": "list, create ou resend."},
					"company_id":  map[string]any{"type": "string", "description": "UUID opcional da empresa (para create)."},
					"url":         map[string]any{"type": "string", "description": "URL https:// ou http:// de destino (para create)."},
					"secret":      map[string]any{"type": "string", "description": "Segredo HMAC-SHA256 opcional (para create)."},
					"document_id": map[string]any{"type": "string", "description": "UUID da nota fiscal (para resend)."},
					"ref":         map[string]any{"type": "string", "description": "Referência externa da nota (para resend)."},
				},
				"required": []string{"action"},
			},
		},
		{
			"name":          "resend_document_webhook",
			"description":   "Reenvia imediatamente a notificação de Webhook (assinada com HMAC-SHA256) para uma nota fiscal por id ou ref.",
			"requiredScope": "documents:write",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":  map[string]any{"type": "string", "description": "UUID do documento."},
					"ref": map[string]any{"type": "string", "description": "Referência externa do documento."},
				},
			},
		},
		{
			"name":          "diagnose_fiscal_error",
			"description":   "Traduz qualquer código de erro da API ou código cStat de rejeição da SEFAZ explicando qual foi o erro (message) e o passo a passo do que fazer para corrigir (action).",
			"requiredScope": "documents:read",
			"inputSchema": map[string]any{
				"type":       "object",
				"properties": map[string]any{"code": map[string]any{"type": "string", "description": "Código do erro (ex.: SEFAZ_539, SEFAZ_656, CERTIFICATE_REQUIRED)"}},
				"required":   []string{"code"},
			},
		},
		{
			"name":          "list_brazilian_states",
			"description":   "Lista as 27 UFs brasileiras com código IBGE e autorizador SEFAZ.",
			"requiredScope": "documents:read",
			"inputSchema":   map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			"name":          "lookup_fiscal_table",
			"description":   "Consulta tabelas auxiliares fiscais: cfop (CFOPs de venda/devolução/exportação), ncm (códigos NCM), states (27 UFs), document_types (modelos de notas e finalidades suportadas), cep (validação de CEP) ou cnpj (validação de CNPJ).",
			"requiredScope": "documents:read",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"table": map[string]any{"type": "string", "description": "cfop, ncm, states, document_types, errors, cep ou cnpj."},
					"value": map[string]any{"type": "string", "description": "Valor para consulta de cep, cnpj ou código de erro."},
				},
				"required": []string{"table"},
			},
		},
	}
}

func mcpResourcesCatalog() []map[string]any {
	return []map[string]any{
		{
			"uri":         "fiscal://document-types",
			"name":        "Tipos e Modalidades de Notas Fiscais Suportadas",
			"description": "Matriz completa de modelos (NF-e 55, NFC-e 65), finalidades (Normal, Complementar, Ajuste, Devolução), Reforma Tributária (IBS/CBS/IS) e Eventos.",
			"mimeType":    "application/json",
		},
		{
			"uri":         "fiscal://states",
			"name":        "27 UFs Brasileiras e Autorizadores SEFAZ",
			"description": "Tabela oficial IBGE e SEFAZ Autorizadora para cada estado do Brasil.",
			"mimeType":    "application/json",
		},
		{
			"uri":         "fiscal://errors",
			"name":        "Catálogo Completo de Erros e Rejeições SEFAZ",
			"description": "Todos os códigos de erro da API e rejeições SEFAZ com explicação de qual foi o erro e o que fazer.",
			"mimeType":    "application/json",
		},
		{
			"uri":         "fiscal://companies",
			"name":        "Empresas Emitentes do Cliente",
			"description": "Lista de empresas emitentes ativas do tenant autenticado.",
			"mimeType":    "application/json",
		},
	}
}

func mcpPromptsCatalog() []map[string]any {
	return []map[string]any{
		{
			"name":        "emitir-nota-cliente",
			"description": "Fluxo seguro passo a passo para verificar a prontidão da empresa, simular impostos e emitir uma NF-e ou NFC-e.",
			"arguments": []map[string]any{
				{"name": "company_id", "description": "UUID da empresa emitente (opcional; se omitido, lista as empresas).", "required": false},
			},
		},
		{
			"name":        "diagnosticar-rejeicao-sefaz",
			"description": "Analisa uma nota fiscal rejeitada ou código cStat da SEFAZ e orienta exatamente como corrigir.",
			"arguments": []map[string]any{
				{"name": "code_or_ref", "description": "Código do erro (ex.: SEFAZ_539) ou referência do pedido (ref).", "required": true},
			},
		},
	}
}

func (s *Server) handleMCP(w http.ResponseWriter, r *http.Request, p store.Principal) {
	tools := mcpToolsCatalog()
	resources := mcpResourcesCatalog()
	prompts := mcpPromptsCatalog()
	reqID := w.Header().Get("X-Request-ID")

	if r.Method == "GET" {
		reply(w, r, 200, map[string]any{
			"protocol":          "mcp/1.0",
			"protocol_versions": []string{"2025-03-26", "2024-11-05"},
			"server": map[string]string{
				"name":        "allspark-fiscal-mcp",
				"version":     "0.4.0",
				"description": "Servidor MCP Oficial da AllSpark Fiscal compatível com Claude, ChatGPT, Gemini, Cursor e Windsurf",
			},
			"security": map[string]any{
				"tenant_isolated":   true,
				"scope_enforced":    true,
				"active_scopes":     p.Scopes,
				"anti_656_shielded": true,
			},
			"tools":     tools,
			"resources": resources,
			"prompts":   prompts,
		}, "", "")
		return
	}
	if r.Method != "POST" {
		reply(w, r, 405, nil, "NOT_FOUND", "Use GET ou POST em /v1/mcp.")
		return
	}

	var rpc struct {
		JSONRPC string         `json:"jsonrpc"`
		ID      any            `json:"id"`
		Method  string         `json:"method"`
		Params  map[string]any `json:"params"`
	}
	if decode(w, r, &rpc) != nil {
		reply(w, r, 422, nil, "INVALID_JSON", "Requisição JSON-RPC MCP inválida.")
		return
	}

	writeRPC := func(res any) {
		w.WriteHeader(200)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      rpc.ID,
			"result":  res,
		})
	}

	switch rpc.Method {
	case "initialize":
		writeRPC(map[string]any{
			"protocolVersion": "2024-11-05",
			"serverInfo":      map[string]string{"name": "allspark-fiscal-mcp", "version": "0.4.0"},
			"capabilities": map[string]any{
				"tools":     map[string]any{"listChanged": false},
				"resources": map[string]any{"listChanged": false},
				"prompts":   map[string]any{"listChanged": false},
			},
		})
	case "notifications/initialized", "ping":
		writeRPC(map[string]any{})
	case "tools/list":
		writeRPC(map[string]any{"tools": tools})
	case "resources/list":
		writeRPC(map[string]any{"resources": resources})
	case "resources/read":
		uri, _ := rpc.Params["uri"].(string)
		ctx := r.Context()
		var data any
		switch uri {
		case "fiscal://states":
			data = brazilianStates
		case "fiscal://errors":
			data = fiscal.AllErrorGuides()
		case "fiscal://companies":
			data, _ = s.Store.Companies(ctx, p.TenantID, "", 50)
		default:
			data = map[string]any{
				"models":    []string{"nfe (55)", "nfce (65)"},
				"purposes":  []string{"1: Normal", "2: Complementar", "3: Ajuste", "4: Devolução"},
				"rtc_ready": true,
				"difal":     true,
			}
		}
		raw, _ := json.MarshalIndent(data, "", "  ")
		writeRPC(map[string]any{
			"contents": []map[string]any{
				{"uri": uri, "mimeType": "application/json", "text": string(raw)},
			},
		})
	case "prompts/list":
		writeRPC(map[string]any{"prompts": prompts})
	case "prompts/get":
		pName, _ := rpc.Params["name"].(string)
		text := "Use as ferramentas list_companies -> get_company_readiness -> preview_order_invoice -> emit_order_invoice para emitir com segurança total."
		if pName == "diagnosticar-rejeicao-sefaz" {
			text = "Consulte get_document_status e diagnose_fiscal_error para identificar qual foi o erro (message) e o passo a passo do que fazer (action)."
		}
		writeRPC(map[string]any{
			"description": "Guia operacional AllSpark Fiscal MCP",
			"messages": []map[string]any{
				{"role": "user", "content": map[string]string{"type": "text", "text": text}},
			},
		})
	case "tools/call":
		name, _ := rpc.Params["name"].(string)
		args, _ := rpc.Params["arguments"].(map[string]any)
		if args == nil {
			args = map[string]any{}
		}
		output, isErr := s.executeMCPTool(r, p, name, args, reqID)
		txt, _ := json.MarshalIndent(output, "", "  ")
		writeRPC(map[string]any{
			"isError": isErr,
			"content": []map[string]string{{"type": "text", "text": string(txt)}},
		})
	default:
		writeRPC(map[string]any{"tools": tools})
	}
}

func (s *Server) executeMCPTool(r *http.Request, p store.Principal, name string, args map[string]any, reqID string) (any, bool) {
	ctx := r.Context()

	// Find required scope for tool
	reqScope := "documents:read"
	for _, t := range mcpToolsCatalog() {
		if t["name"] == name {
			if sc, ok := t["requiredScope"].(string); ok {
				reqScope = sc
			}
			break
		}
	}
	if name == "manage_webhooks" {
		act, _ := args["action"].(string)
		if act == "create" || act == "resend" {
			reqScope = "documents:write"
		}
	}
	if !hasScope(p, reqScope) {
		return fiscal.ExplainError("FORBIDDEN", "A chave de API atual não possui o escopo necessário ("+reqScope+") para executar a tool '"+name+"'.", false), true
	}

	switch name {
	case "list_companies":
		limit := 50
		if n, ok := args["limit"].(float64); ok && int(n) > 0 && int(n) <= 100 {
			limit = int(n)
		}
		items, err := s.Store.Companies(ctx, p.TenantID, "", limit)
		if err != nil {
			return fiscal.ExplainError("SERVICE_UNAVAILABLE", err.Error(), true), true
		}
		return map[string]any{"items": items, "count": len(items)}, false

	case "get_company_readiness":
		companyID, _ := args["company_id"].(string)
		if !validID(companyID) {
			return fiscal.ExplainError("INVALID_COMPANY", "Informe um company_id (UUID) válido.", false), true
		}
		out, err := s.companyReadiness(ctx, p.TenantID, companyID)
		if err != nil {
			return fiscal.ExplainError("NOT_FOUND", "Empresa não encontrada no seu tenant.", false), true
		}
		return out, false

	case "create_company":
		raw, _ := json.Marshal(args)
		var in store.Company
		if json.Unmarshal(raw, &in) != nil {
			return fiscal.ExplainError("INVALID_COMPANY", "Parâmetros inválidos para criação de empresa.", false), true
		}
		in.CNPJ = strings.ToUpper(strings.TrimSpace(in.CNPJ))
		in.UF = strings.ToUpper(strings.TrimSpace(in.UF))
		in.IE = strings.ToUpper(strings.TrimSpace(in.IE))
		in.Name = strings.TrimSpace(in.Name)
		if in.ContingencyMode == "" {
			in.ContingencyMode = "auto"
		}
		if in.Address == nil || !validAddress(in.Address) || in.Address.State != in.UF || !validCNPJ(in.CNPJ) || len(in.Name) < 2 || len(in.Name) > 120 || !strings.Contains("|AC|AL|AP|AM|BA|CE|DF|ES|GO|MA|MT|MS|MG|PA|PB|PR|PE|PI|RJ|RN|RS|RO|RR|SC|SP|SE|TO|", "|"+in.UF+"|") || len(in.UF) != 2 || !validIE(in.IE) || in.CRT < 1 || in.CRT > 4 {
			return fiscal.ExplainError("INVALID_COMPANY", "Informe CNPJ válido, razão social, UF, IE, CRT (1..3) e endereço completo da mesma UF.", false), true
		}
		c, err := s.Store.CreateCompany(ctx, p, in, reqID)
		if err != nil {
			return fiscal.ExplainError("CONFLICT", err.Error(), false), true
		}
		return c, false

	case "configure_company_fiscal":
		companyID, _ := args["company_id"].(string)
		kind, _ := args["kind"].(string)
		env, _ := args["environment"].(string)
		if env == "" {
			env = "homologation"
		}
		series := 1
		if n, ok := args["series"].(float64); ok && int(n) > 0 {
			series = int(n)
		}
		enabled := true
		if b, ok := args["enabled"].(bool); ok {
			enabled = b
		}
		if !validID(companyID) || (kind != "nfe" && kind != "nfce") || (env != "homologation" && env != "production") || series < 1 || series > 889 {
			return fiscal.ExplainError("INVALID_FISCAL_CONFIG", "Informe company_id válido, kind (nfe ou nfce), environment (homologation) e series (1..889).", false), true
		}
		cfg, err := s.Store.PutSettings(ctx, p, companyID, store.FiscalSettings{
			Kind:        kind,
			Environment: env,
			Series:      series,
			Enabled:     enabled,
		}, reqID)
		if err != nil {
			return fiscal.ExplainError("INVALID_FISCAL_CONFIG", err.Error(), false), true
		}
		return cfg, false

	case "list_tax_profiles":
		companyID, _ := args["company_id"].(string)
		if !validID(companyID) {
			return fiscal.ExplainError("INVALID_COMPANY", "Informe um company_id (UUID) válido.", false), true
		}
		items, err := s.Store.ListTaxProfiles(ctx, p.TenantID, companyID)
		if err != nil {
			return fiscal.ExplainError("SERVICE_UNAVAILABLE", err.Error(), true), true
		}
		return map[string]any{"items": items, "count": len(items)}, false

	case "upsert_tax_profile":
		raw, _ := json.Marshal(args)
		var tp store.TaxProfile
		if json.Unmarshal(raw, &tp) != nil || !validID(tp.CompanyID) || strings.TrimSpace(tp.Name) == "" || len(tp.Taxes) == 0 {
			return fiscal.ExplainError("INVALID_JSON", "Informe company_id, name e taxes para o perfil tributário.", false), true
		}
		saved, err := s.Store.UpsertTaxProfile(ctx, p, tp, reqID)
		if err != nil {
			return fiscal.ExplainError("INVALID_JSON", err.Error(), false), true
		}
		return saved, false

	case "preview_order_invoice":
		raw, _ := json.Marshal(args)
		var order orderInvoiceRequest
		if json.Unmarshal(raw, &order) != nil || !validID(order.CompanyID) || len(order.Items) == 0 {
			return fiscal.ExplainError("INVALID_DOCUMENT", "Informe company_id válido e ao menos 1 item em items.", false), true
		}
		out, code, why, err := s.simulateOrder(ctx, p.TenantID, order)
		if err != nil {
			return fiscal.ExplainError("NOT_FOUND", err.Error(), false), true
		}
		if code != "" {
			return fiscal.ExplainError(code, why, false), true
		}
		return out, false

	case "emit_order_invoice":
		raw, _ := json.Marshal(args)
		var order orderInvoiceRequest
		if json.Unmarshal(raw, &order) != nil || !validID(order.CompanyID) || len(order.Items) == 0 {
			return fiscal.ExplainError("INVALID_DOCUMENT", "Informe company_id válido e ao menos 1 item em items.", false), true
		}
		in := s.buildOrderInput(ctx, p.TenantID, order)
		key := "mcp-order-" + uuid.NewString()
		if in.Ref != "" {
			key = "order-ref-" + in.Ref
		}
		if why := s.validate(in, key); why != "" {
			return fiscal.ExplainError("INVALID_DOCUMENT", why, false), true
		}
		id, replay, err := s.Store.Submit(ctx, p, in, key, reqID)
		if err != nil {
			return fiscal.ExplainError("INVALID_DOCUMENT", err.Error(), false), true
		}
		return map[string]any{
			"id":          id,
			"replayed":    replay,
			"kind":        in.Kind,
			"ref":         in.Ref,
			"webhook_url": in.WebhookURL,
			"status_url":  "/v1/documents/" + id,
			"xml_url":     "/v1/documents/" + id + "/xml",
			"pdf_url":     "/v1/documents/" + id + "/pdf",
		}, false

	case "emit_complete_document":
		raw, _ := json.Marshal(args)
		var in store.Input
		if json.Unmarshal(raw, &in) != nil || !validID(in.CompanyID) {
			return fiscal.ExplainError("INVALID_DOCUMENT", "Informe company_id e invoice válidos.", false), true
		}
		normalizeInput(&in)
		key := "mcp-doc-" + uuid.NewString()
		if in.Ref != "" {
			key = "doc-ref-" + in.Ref
		}
		if why := s.validate(in, key); why != "" {
			return fiscal.ExplainError("INVALID_DOCUMENT", why, false), true
		}
		id, replay, err := s.Store.Submit(ctx, p, in, key, reqID)
		if err != nil {
			return fiscal.ExplainError("INVALID_DOCUMENT", err.Error(), false), true
		}
		return map[string]any{
			"id":          id,
			"replayed":    replay,
			"kind":        in.Kind,
			"ref":         in.Ref,
			"webhook_url": in.WebhookURL,
			"status_url":  "/v1/documents/" + id,
			"xml_url":     "/v1/documents/" + id + "/xml",
			"pdf_url":     "/v1/documents/" + id + "/pdf",
		}, false

	case "get_document_status":
		id, _ := args["id"].(string)
		ref, _ := args["ref"].(string)
		if ref != "" {
			doc, err := s.Store.DocumentByRef(ctx, p.TenantID, ref)
			if err != nil {
				return fiscal.ExplainError("NOT_FOUND", "Documento não encontrado para a referência: "+ref, false), true
			}
			return doc, false
		}
		if validID(id) {
			doc, err := s.Store.Document(ctx, p.TenantID, id)
			if err != nil {
				return fiscal.ExplainError("NOT_FOUND", "Documento não encontrado para o id: "+id, false), true
			}
			return doc, false
		}
		return fiscal.ExplainError("NOT_FOUND", "Informe id (UUID) ou ref válido.", false), true

	case "list_documents":
		companyID, _ := args["company_id"].(string)
		kind, _ := args["kind"].(string)
		limit := 20
		if n, ok := args["limit"].(float64); ok && int(n) > 0 && int(n) <= 100 {
			limit = int(n)
		}
		if companyID != "" && !validID(companyID) {
			return fiscal.ExplainError("INVALID_COMPANY", "company_id inválido.", false), true
		}
		items, err := s.Store.Documents(ctx, p.TenantID, companyID, kind, "", limit)
		if err != nil {
			return fiscal.ExplainError("SERVICE_UNAVAILABLE", err.Error(), true), true
		}
		return map[string]any{"items": items, "count": len(items)}, false

	case "cancel_document":
		docID, _ := args["id"].(string)
		ref, _ := args["ref"].(string)
		reason, _ := args["reason"].(string)
		reason = strings.TrimSpace(reason)
		if len(reason) < 15 || len(reason) > 255 {
			return fiscal.ExplainError("INVALID_CANCEL_REASON", "A justificativa de cancelamento deve ter entre 15 e 255 caracteres.", false), true
		}
		if docID == "" && ref != "" {
			if d, err := s.Store.DocumentByRef(ctx, p.TenantID, ref); err == nil {
				docID = d.ID
			}
		}
		if !validID(docID) {
			return fiscal.ExplainError("NOT_FOUND", "Documento não encontrado por id ou ref.", false), true
		}
		d, err := s.Store.Document(ctx, p.TenantID, docID)
		if err != nil {
			return fiscal.ExplainError("NOT_FOUND", "Documento não encontrado.", false), true
		}
		if !s.Issuance {
			return fiscal.ExplainError("TRANSMISSION_DISABLED", "Transmissão fiscal SEFAZ desabilitada neste ambiente.", false), true
		}
		if d.Status != "authorized" || d.AccessKey == nil || *d.AccessKey == "" {
			return fiscal.ExplainError("INVALID_DOCUMENT_STATE", "Somente notas autorizadas com chave de acesso podem ser canceladas.", false), true
		}
		var prevResult struct {
			Protocol struct {
				Number string `json:"number"`
			} `json:"protocol"`
		}
		_ = json.Unmarshal(d.Result, &prevResult)
		c, _ := s.Store.Company(ctx, p.TenantID, d.CompanyID)
		cert, err := s.Store.ActiveCertificate(ctx, s.Vault, p.TenantID, d.CompanyID)
		if err != nil {
			return fiscal.ExplainError("CERTIFICATE_REQUIRED", err.Error(), false), true
		}
		var res engine.Result
		if err := s.Engine.Call(ctx, "cancel", map[string]any{
			"company":         c,
			"certificate":     cert,
			"document":        map[string]any{"kind": d.Kind, "environment": d.Environment},
			"access_key":      *d.AccessKey,
			"reason":          reason,
			"protocol_number": prevResult.Protocol.Number,
		}, &res); err != nil {
			return fiscal.ExplainError("SEFAZ_REJECTED", err.Error(), false), true
		}
		var saved []store.Artifact
		for _, a := range res.Artifacts {
			if rawBytes, decErr := base64.StdEncoding.DecodeString(a.Base64); decErr == nil {
				if art, putErr := s.Objects.Put(p.TenantID, docID, a.Kind, a.MediaType, rawBytes); putErr == nil {
					saved = append(saved, art)
				}
			}
		}
		res.Artifacts = nil
		newStatus := ""
		if res.Status == "canceled" {
			newStatus = "canceled"
		}
		_ = s.Store.RecordDocumentEvent(ctx, p, docID, "document.canceled", newStatus, res, saved, reqID)
		return map[string]any{"id": docID, "status": res.Status, "protocol": res.Protocol}, false

	case "send_correction_letter":
		docID, _ := args["id"].(string)
		ref, _ := args["ref"].(string)
		correction, _ := args["correction"].(string)
		correction = strings.TrimSpace(correction)
		seq := 1
		if n, ok := args["sequence"].(float64); ok && int(n) >= 1 && int(n) <= 20 {
			seq = int(n)
		}
		if len(correction) < 15 || len(correction) > 1000 {
			return fiscal.ExplainError("INVALID_CCE_INPUT", "O texto da Carta de Correção deve ter entre 15 e 1000 caracteres.", false), true
		}
		if docID == "" && ref != "" {
			if d, err := s.Store.DocumentByRef(ctx, p.TenantID, ref); err == nil {
				docID = d.ID
			}
		}
		if !validID(docID) {
			return fiscal.ExplainError("NOT_FOUND", "Documento não encontrado por id ou ref.", false), true
		}
		if !s.Issuance {
			return fiscal.ExplainError("TRANSMISSION_DISABLED", "Transmissão fiscal SEFAZ desabilitada neste ambiente.", false), true
		}
		return map[string]any{"id": docID, "sequence": seq, "status": "ready"}, false

	case "check_expiring_certificates":
		days := 30
		if n, ok := args["days"].(float64); ok && int(n) > 0 && int(n) <= 3650 {
			days = int(n)
		}
		items, err := s.Store.ExpiringCertificates(ctx, p.TenantID, days)
		if err != nil {
			return fiscal.ExplainError("SERVICE_UNAVAILABLE", err.Error(), true), true
		}
		return map[string]any{"within_days": days, "items": items, "count": len(items)}, false

	case "manage_webhooks":
		action, _ := args["action"].(string)
		switch action {
		case "list":
			items, err := s.Store.ListWebhooks(ctx, p.TenantID)
			if err != nil {
				return fiscal.ExplainError("SERVICE_UNAVAILABLE", err.Error(), true), true
			}
			return map[string]any{"items": items, "count": len(items)}, false
		case "create":
			urlStr, _ := args["url"].(string)
			secret, _ := args["secret"].(string)
			compID, _ := args["company_id"].(string)
			if !strings.HasPrefix(urlStr, "http://") && !strings.HasPrefix(urlStr, "https://") {
				return fiscal.ExplainError("INVALID_JSON", "Informe uma url válida iniciando com http:// ou https://.", false), true
			}
			var compPtr *string
			if validID(compID) {
				compPtr = &compID
			}
			wh, err := s.Store.CreateWebhook(ctx, p, compPtr, urlStr, secret, nil, reqID)
			if err != nil {
				return fiscal.ExplainError("INVALID_JSON", err.Error(), false), true
			}
			return wh, false
		case "resend":
			docID, _ := args["document_id"].(string)
			ref, _ := args["ref"].(string)
			if docID == "" && ref != "" {
				if d, err := s.Store.DocumentByRef(ctx, p.TenantID, ref); err == nil {
					docID = d.ID
				}
			}
			if !validID(docID) {
				return fiscal.ExplainError("NOT_FOUND", "Informe document_id ou ref válido para reenviar o webhook.", false), true
			}
			dels, err := s.Store.DispatchDocumentWebhooks(ctx, p.TenantID, docID)
			if err != nil {
				return fiscal.ExplainError("NOT_FOUND", err.Error(), false), true
			}
			return map[string]any{"document_id": docID, "deliveries": dels}, false
		default:
			return fiscal.ExplainError("INVALID_JSON", "action deve ser list, create ou resend.", false), true
		}

	case "resend_document_webhook":
		id, _ := args["id"].(string)
		ref, _ := args["ref"].(string)
		if id == "" && ref != "" {
			if d, err := s.Store.DocumentByRef(ctx, p.TenantID, ref); err == nil {
				id = d.ID
			}
		}
		if !validID(id) {
			return fiscal.ExplainError("NOT_FOUND", "Informe id (UUID) ou ref válido.", false), true
		}
		dels, err := s.Store.DispatchDocumentWebhooks(ctx, p.TenantID, id)
		if err != nil {
			return fiscal.ExplainError("NOT_FOUND", err.Error(), false), true
		}
		return map[string]any{"document_id": id, "deliveries": dels}, false

	case "diagnose_fiscal_error":
		code, _ := args["code"].(string)
		return fiscal.ExplainError(code, "", false), false

	case "list_brazilian_states":
		return brazilianStates, false

	case "lookup_fiscal_table":
		table, _ := args["table"].(string)
		val, _ := args["value"].(string)
		switch strings.ToLower(strings.TrimSpace(table)) {
		case "cfop":
			return map[string]any{"items": cfopCatalog}, false
		case "ncm":
			return map[string]any{"items": ncmCatalog}, false
		case "states", "ufs":
			return map[string]any{"items": brazilianStates}, false
		case "errors":
			if val != "" {
				return fiscal.ExplainError(val, "", false), false
			}
			return map[string]any{"items": fiscal.AllErrorGuides()}, false
		case "cep":
			cep := regexp.MustCompile(`[^0-9]`).ReplaceAllString(val, "")
			return map[string]any{"postal_code": cep, "valid_format": len(cep) == 8}, false
		case "cnpj":
			cnpj := strings.ToUpper(regexp.MustCompile(`[^A-Z0-9]`).ReplaceAllString(val, ""))
			return map[string]any{"cnpj": cnpj, "valid_checksum": validCNPJ(cnpj)}, false
		default:
			return map[string]any{
				"models": []map[string]any{
					{"kind": "nfe", "model": 55, "purposes": []string{"1: Normal", "2: Complementar", "3: Ajuste", "4: Devolução"}},
					{"kind": "nfce", "model": 65, "purposes": []string{"1: Venda a Consumidor Final"}},
				},
			}, false
		}

	default:
		return map[string]string{"error": "Tool não encontrada: " + name}, true
	}
}
