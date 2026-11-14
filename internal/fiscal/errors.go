package fiscal

import "strings"

// ErrorGuide describes a fiscal or API error with a human-readable explanation
// ("Message" = what happened) and a concrete step-by-step fix ("Action" = what to do).
type ErrorGuide struct {
	Code      string `json:"code"`
	SefazCode string `json:"sefaz_code,omitempty"`
	Field     string `json:"field,omitempty"`
	Message   string `json:"message"`
	Action    string `json:"action"`
	Source    string `json:"source"`
	Retryable bool   `json:"retryable"`
}

var errorCatalog = map[string]ErrorGuide{
	"UNAUTHORIZED": {
		Code:      "UNAUTHORIZED",
		Message:   "Credencial de acesso ausente, inválida ou revogada.",
		Action:    "Envie o header 'Authorization: Bearer af_...' com uma chave ativa do seu tenant.",
		Source:    "allspark",
		Retryable: false,
	},
	"FORBIDDEN": {
		Code:      "FORBIDDEN",
		Message:   "A chave de API utilizada não possui permissão (scope) para esta operação.",
		Action:    "Utilize uma chave com os escopos necessários (ex.: companies:write, certificates:write, documents:write).",
		Source:    "allspark",
		Retryable: false,
	},
	"NOT_FOUND": {
		Code:      "NOT_FOUND",
		Message:   "O recurso solicitado não foi encontrado ou pertence a outro tenant.",
		Action:    "Confira o UUID ou a referência externa (ref) informada na URL e se pertence à mesma conta.",
		Source:    "allspark",
		Retryable: false,
	},
	"INVALID_JSON": {
		Code:      "INVALID_JSON",
		Message:   "O corpo da requisição não é um JSON válido ou contém campos desconhecidos.",
		Action:    "Verifique a sintaxe do JSON (aspas, vírgulas), o header 'Content-Type: application/json' e os nomes dos campos.",
		Source:    "allspark",
		Retryable: false,
	},
	"INVALID_DOCUMENT": {
		Code:      "INVALID_DOCUMENT",
		Field:     "invoice",
		Message:   "Os dados da nota fiscal contêm campos inválidos ou inconsistentes.",
		Action:    "Leia a mensagem detalhada do erro ou utilize o modo simplificado omitindo impostos/totais para cálculo automático.",
		Source:    "allspark",
		Retryable: false,
	},
	"COMPANY_ADDRESS_REQUIRED": {
		Code:      "COMPANY_ADDRESS_REQUIRED",
		Field:     "company.address",
		Message:   "A empresa emitente não possui endereço completo compatível com a sua UF.",
		Action:    "Atualize a empresa em PATCH /v1/companies/{id} informando street, number, district, city, state, postal_code (8 dígitos) e city_code IBGE (7 dígitos).",
		Source:    "allspark",
		Retryable: false,
	},
	"COMPANY_ARCHIVED": {
		Code:      "COMPANY_ARCHIVED",
		Field:     "company_id",
		Message:   "A empresa emitente está desativada (arquivada).",
		Action:    "Utilize uma empresa ativa para gerar ou emitir novos documentos fiscais.",
		Source:    "allspark",
		Retryable: false,
	},
	"COMPANY_BUSY": {
		Code:      "COMPANY_BUSY",
		Field:     "company_id",
		Message:   "A empresa possui documentos em processamento ou aguardando reconciliação na SEFAZ.",
		Action:    "Aguarde alguns segundos até a conclusão das notas na fila ou reconcilie documentos pendentes em POST /v1/documents/{id}/reconcile.",
		Source:    "allspark",
		Retryable: true,
	},
	"FISCAL_CONFIG_REQUIRED": {
		Code:      "FISCAL_CONFIG_REQUIRED",
		Field:     "series / environment",
		Message:   "O tipo de nota (nfe/nfce), ambiente ou série não está habilitado na configuração fiscal da empresa.",
		Action:    "Chame PUT /v1/companies/{id}/fiscal/nfe (ou /nfce) com {\"environment\":\"homologation\",\"series\":1,\"enabled\":true}.",
		Source:    "allspark",
		Retryable: false,
	},
	"CERTIFICATE_REQUIRED": {
		Code:      "CERTIFICATE_REQUIRED",
		Field:     "certificate",
		Message:   "A empresa não possui um Certificado Digital ICP-Brasil A1 válido e ativo.",
		Action:    "Envie o arquivo .pfx/.p12 em Base64 e a senha em PUT /v1/companies/{id}/certificate antes de emitir.",
		Source:    "allspark",
		Retryable: false,
	},
	"CERTIFICATE_EXPIRED": {
		Code:      "CERTIFICATE_EXPIRED",
		Field:     "certificate",
		Message:   "O Certificado Digital A1 da empresa está vencido.",
		Action:    "Renove o certificado A1 junto à autoridade certificadora e envie o novo arquivo .pfx em PUT /v1/companies/{id}/certificate.",
		Source:    "allspark",
		Retryable: false,
	},
	"CERTIFICATE_CNPJ_MISMATCH": {
		Code:      "CERTIFICATE_CNPJ_MISMATCH",
		Field:     "certificate.pfx_base64",
		Message:   "O CNPJ contido no Certificado Digital A1 é diferente do CNPJ da empresa cadastrada.",
		Action:    "Envie o certificado A1 que pertence exatamente ao CNPJ da empresa emitente.",
		Source:    "allspark",
		Retryable: false,
	},
	"CSC_REQUIRED": {
		Code:      "CSC_REQUIRED",
		Field:     "certificate.csc",
		Message:   "Para emitir NFC-e (Modelo 65), é obrigatório informar o CSC e o CSC ID da SEFAZ estadual.",
		Action:    "Cadastre o certificado em PUT /v1/companies/{id}/certificate incluindo os campos 'csc' e 'csc_id' gerados no portal da SEFAZ.",
		Source:    "allspark",
		Retryable: false,
	},
	"ISSUANCE_DISABLED": {
		Code:      "ISSUANCE_DISABLED",
		Field:     "operation",
		Message:   "A transmissão externa para a SEFAZ está desabilitada nesta instância.",
		Action:    "Use operation='generate' para gerar XML assinado e DANFE em prévia, ou habilite ENABLE_ISSUANCE=true no ambiente.",
		Source:    "allspark",
		Retryable: false,
	},
	"CONFLICT": {
		Code:      "CONFLICT",
		Field:     "Idempotency-Key / ref",
		Message:   "Conflito de idempotência, referência externa já utilizada com outro conteúdo ou estado do documento incompatível.",
		Action:    "Se for uma nova nota, gere um novo Idempotency-Key e um novo 'ref'. Se quiser consultar a nota existente, faça GET /v1/documents/{id}.",
		Source:    "allspark",
		Retryable: false,
	},
	"RATE_LIMITED": {
		Code:      "RATE_LIMITED",
		Message:   "Limite de requisições por segundo excedido ou proteção Anti-656 da SEFAZ acionada.",
		Action:    "Utilize o endpoint de lote POST /v1/documents/bulk (até 50 notas por chamada) e aguarde o retorno via Webhook em vez de fazer polling contínuo.",
		Source:    "allspark",
		Retryable: true,
	},
	"SEFAZ_COOLDOWN_656": {
		Code:      "SEFAZ_COOLDOWN_656",
		SefazCode: "656",
		Message:   "Proteção Anti-656 ativa: a SEFAZ reportou Consumo Indevido para este CNPJ.",
		Action:    "Não tente forçar o reenvio. Nossa fila pausou automaticamente as transmissões deste CNPJ por 62 minutos para a SEFAZ liberar o bloqueio de 1 hora. Corrija os dados da nota rejeitada.",
		Source:    "sefaz",
		Retryable: true,
	},
	"SEFAZ_204": {
		Code:      "SEFAZ_204",
		SefazCode: "204",
		Field:     "series / number",
		Message:   "Rejeição 204: Duplicidade de NF-e (esta chave de acesso já foi autorizada na SEFAZ).",
		Action:    "Não emita uma nova nota! Execute POST /v1/documents/{id}/query (ou /reconcile) para recuperar o protocolo de autorização da SEFAZ.",
		Source:    "sefaz",
		Retryable: false,
	},
	"SEFAZ_213": {
		Code:      "SEFAZ_213",
		SefazCode: "213",
		Field:     "certificate",
		Message:   "Rejeição 213: CNPJ-Base do Emitente difere do CNPJ-Base do Certificado Digital.",
		Action:    "Atualize o certificado A1 da empresa em PUT /v1/companies/{id}/certificate com um certificado do mesmo CNPJ raiz.",
		Source:    "sefaz",
		Retryable: false,
	},
	"SEFAZ_215": {
		Code:      "SEFAZ_215",
		SefazCode: "215",
		Field:     "invoice",
		Message:   "Rejeição 215 / Falha de Schema XML: o XML gerado não passou na validação XSD da SEFAZ.",
		Action:    "Verifique códigos IBGE de município (7 dígitos), CEP (8 dígitos), NCM (8 dígitos) e caracteres especiais nos textos.",
		Source:    "sefaz",
		Retryable: false,
	},
	"FISCAL_VALIDATION_FAILED": {
		Code:      "FISCAL_VALIDATION_FAILED",
		Field:     "invoice",
		Message:   "O XML da nota fiscal não passou na validação do Schema XSD oficial (PL_010_V1.30).",
		Action:    "Confira códigos fiscais (CFOP, NCM, CST/CSOSN, cClassTrib) ou use o modo simplificado omitindo 'taxes' para preenchimento seguro.",
		Source:    "engine",
		Retryable: false,
	},
	"SEFAZ_290": {
		Code:      "SEFAZ_290",
		SefazCode: "290",
		Field:     "certificate",
		Message:   "Rejeição 290: Certificado Assinatura inválido ou corrompido.",
		Action:    "Substitua o certificado A1 em PUT /v1/companies/{id}/certificate por um certificado ICP-Brasil válido.",
		Source:    "sefaz",
		Retryable: false,
	},
	"SEFAZ_291": {
		Code:      "SEFAZ_291",
		SefazCode: "291",
		Field:     "certificate",
		Message:   "Rejeição 291: Certificado Assinatura Data Validade vencida na SEFAZ.",
		Action:    "Renove o certificado A1 da empresa e envie o novo arquivo .pfx em PUT /v1/companies/{id}/certificate.",
		Source:    "sefaz",
		Retryable: false,
	},
	"SEFAZ_301": {
		Code:      "SEFAZ_301",
		SefazCode: "301",
		Field:     "company.ie",
		Message:   "Uso Denegado (301): Irregularidade fiscal do emitente na SEFAZ estadual.",
		Action:    "A Inscrição Estadual (IE) da empresa emitente está suspensa ou irregular na SEFAZ. O contador deve regularizar a IE no estado.",
		Source:    "sefaz",
		Retryable: false,
	},
	"SEFAZ_302": {
		Code:      "SEFAZ_302",
		SefazCode: "302",
		Field:     "invoice.recipient.ie",
		Message:   "Uso Denegado (302): Irregularidade fiscal do destinatário na SEFAZ estadual.",
		Action:    "A IE do cliente destinatário está suspensa/baixada no SINTEGRA. Confira a IE do cliente ou, se for não contribuinte, use ie_indicator=9 sem IE.",
		Source:    "sefaz",
		Retryable: false,
	},
	"SEFAZ_462": {
		Code:      "SEFAZ_462",
		SefazCode: "462",
		Field:     "certificate.csc",
		Message:   "Rejeição 462: Código Identificador do CSC no QR-Code não cadastrado na SEFAZ.",
		Action:    "Confira se o CSC e o CSC ID informados no certificado pertencem ao mesmo ambiente (homologação/produção) no portal da SEFAZ estadual.",
		Source:    "sefaz",
		Retryable: false,
	},
	"SEFAZ_539": {
		Code:      "SEFAZ_539",
		SefazCode: "539",
		Field:     "series / number",
		Message:   "Rejeição 539: Duplicidade de NF-e com diferença na Chave de Acesso.",
		Action:    "Este número e série já foram usados em outra nota autorizada na SEFAZ. Ajuste a série ou avance o último número na tabela numbering.",
		Source:    "sefaz",
		Retryable: false,
	},
	"SEFAZ_590": {
		Code:      "SEFAZ_590",
		SefazCode: "590",
		Field:     "invoice.items[].taxes.icms",
		Message:   "Rejeição 590: Informado CST para emissor do Simples Nacional (CRT=1 ou 4).",
		Action:    "Empresas do Simples Nacional/MEI devem usar CSOSN (ex.: '102', '101', '500') em vez de CST, ou omitir 'taxes' para preenchimento automático.",
		Source:    "sefaz",
		Retryable: false,
	},
	"SEFAZ_591": {
		Code:      "SEFAZ_591",
		SefazCode: "591",
		Field:     "invoice.items[].taxes.icms",
		Message:   "Rejeição 591: Informado CSOSN para emissor que não é do Simples Nacional (CRT=3).",
		Action:    "Empresas do Regime Normal (CRT=3) devem usar CST (ex.: '00', '20', '40', '41', '60') em vez de CSOSN.",
		Source:    "sefaz",
		Retryable: false,
	},
	"SEFAZ_610": {
		Code:      "SEFAZ_610",
		SefazCode: "610",
		Field:     "invoice.payments / invoice.items[].total",
		Message:   "Rejeição 610: Total da NF-e difere do somatório dos valores que compõem o valor total da nota.",
		Action:    "Omita 'total' no item e 'payments' na nota para cálculo automático exato, ou confira vNF = vProd - vDesc + vFrete + vSeg + vOutro + vIPI + vST.",
		Source:    "sefaz",
		Retryable: false,
	},
	"SEFAZ_656": {
		Code:      "SEFAZ_656",
		SefazCode: "656",
		Message:   "Rejeição 656: Consumo Indevido — bloqueio temporário de 1 hora pela SEFAZ.",
		Action:    "Pare imediatamente de reenviar a mesma requisição. O escudo Anti-656 pausou transmissões deste CNPJ por 62 minutos até a liberação automática da SEFAZ. Corrija o erro original da nota.",
		Source:    "sefaz",
		Retryable: false,
	},
	"SEFAZ_778": {
		Code:      "SEFAZ_778",
		SefazCode: "778",
		Field:     "invoice.items[].ncm",
		Message:   "Rejeição 778: Informado NCM inexistente na tabela vigente.",
		Action:    "Consulte GET /v1/utils/ncm para obter um código NCM de 8 dígitos válido e ativo na Receita Federal.",
		Source:    "sefaz",
		Retryable: false,
	},
	"SEFAZ_108": {
		Code:      "SEFAZ_108",
		SefazCode: "108",
		Message:   "Serviço da SEFAZ Paralisado Momentaneamente (108).",
		Action:    "Instabilidade temporária na SEFAZ estadual. O job será reprocessado automaticamente na fila com backoff.",
		Source:    "sefaz",
		Retryable: true,
	},
	"SEFAZ_109": {
		Code:      "SEFAZ_109",
		SefazCode: "109",
		Message:   "Serviço da SEFAZ Paralisado sem Previsão (109).",
		Action:    "A SEFAZ estadual está em manutenção ou queda prolongada. Utilize contingência SVC-AN/SVC-RS ou aguarde a retomada automática da fila.",
		Source:    "sefaz",
		Retryable: true,
	},
}

// ExplainError enriches an error code and optional custom message with actionable guidance.
func ExplainError(code, customMsg string, defaultRetryable bool) ErrorGuide {
	norm := strings.ToUpper(strings.TrimSpace(code))
	if g, ok := errorCatalog[norm]; ok {
		if customMsg != "" && (norm == "INVALID_DOCUMENT" || norm == "INVALID_COMPANY" || norm == "INVALID_JSON") {
			g.Message = customMsg
		}
		return g
	}
	if strings.HasPrefix(norm, "SEFAZ_") {
		cstat := strings.TrimPrefix(norm, "SEFAZ_")
		if g, ok := errorCatalog["SEFAZ_"+cstat]; ok {
			return g
		}
		return ErrorGuide{
			Code:      norm,
			SefazCode: cstat,
			Message:   firstNonEmpty(customMsg, "A SEFAZ retornou o código fiscal "+cstat+"."),
			Action:    "Consulte a descrição oficial do código cStat " + cstat + " no retorno do protocolo e ajuste o campo correspondente antes de reenviar.",
			Source:    "sefaz",
			Retryable: defaultRetryable,
		}
	}
	return ErrorGuide{
		Code:      norm,
		Message:   firstNonEmpty(customMsg, "Não foi possível concluir a operação ("+norm+")."),
		Action:    "Verifique os parâmetros enviados na requisição ou consulte GET /v1/utils/errors/" + norm + " para diagnóstico.",
		Source:    "allspark",
		Retryable: defaultRetryable,
	}
}

// AllErrorGuides returns the full dictionary of error codes and resolutions.
func AllErrorGuides() []ErrorGuide {
	keys := []string{
		"UNAUTHORIZED", "FORBIDDEN", "NOT_FOUND", "INVALID_JSON", "INVALID_DOCUMENT",
		"COMPANY_ADDRESS_REQUIRED", "COMPANY_ARCHIVED", "COMPANY_BUSY", "FISCAL_CONFIG_REQUIRED",
		"CERTIFICATE_REQUIRED", "CERTIFICATE_EXPIRED", "CERTIFICATE_CNPJ_MISMATCH", "CSC_REQUIRED",
		"ISSUANCE_DISABLED", "CONFLICT", "RATE_LIMITED", "SEFAZ_COOLDOWN_656", "FISCAL_VALIDATION_FAILED",
		"SEFAZ_204", "SEFAZ_213", "SEFAZ_215", "SEFAZ_290", "SEFAZ_291", "SEFAZ_301", "SEFAZ_302",
		"SEFAZ_462", "SEFAZ_539", "SEFAZ_590", "SEFAZ_591", "SEFAZ_610", "SEFAZ_656", "SEFAZ_778",
		"SEFAZ_108", "SEFAZ_109",
	}
	out := make([]ErrorGuide, 0, len(keys))
	for _, k := range keys {
		if g, ok := errorCatalog[k]; ok {
			out = append(out, g)
		}
	}
	return out
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

