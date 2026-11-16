package httpapi

import (
	"allspark.local/fiscal/internal/engine"
	"allspark.local/fiscal/internal/fiscal"
	"allspark.local/fiscal/internal/store"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var iePattern = regexp.MustCompile(`^(ISENTO|[0-9]{2,14})$`)

func validIE(v string) bool { return iePattern.MatchString(v) }
func requiredScope(method string, path []string) string {
	if len(path) > 1 && path[1] == "mcp" {
		return "documents:read"
	}
	if len(path) == 3 && path[1] == "orders" && path[2] == "preview" {
		return "documents:read"
	}
	resource := "documents"
	if len(path) > 1 && path[1] == "certificates" {
		resource = "certificates"
	} else if len(path) > 1 && path[1] == "companies" {
		resource = "companies"
		if len(path) > 3 && (path[3] == "certificate" || path[3] == "certificates") {
			resource = "certificates"
		}
	}
	action := "write"
	if method == "GET" || method == "HEAD" {
		action = "read"
	}
	return resource + ":" + action
}
func pagination(r *http.Request) (string, int, bool) {
	c := r.URL.Query().Get("cursor")
	n := 50
	var e error
	if v := r.URL.Query().Get("limit"); v != "" {
		n, e = strconv.Atoi(v)
	}
	return c, n, e == nil && n > 0 && n <= 100 && (c == "" || validID(c))
}

var brazilianStates = []map[string]any{
	{"uf": "AC", "code": 12, "name": "Acre", "authorizer": "SVRS"},
	{"uf": "AL", "code": 27, "name": "Alagoas", "authorizer": "SVRS"},
	{"uf": "AM", "code": 13, "name": "Amazonas", "authorizer": "AM"},
	{"uf": "AP", "code": 16, "name": "Amapá", "authorizer": "SVRS"},
	{"uf": "BA", "code": 29, "name": "Bahia", "authorizer": "BA"},
	{"uf": "CE", "code": 23, "name": "Ceará", "authorizer": "CE"},
	{"uf": "DF", "code": 53, "name": "Distrito Federal", "authorizer": "SVRS"},
	{"uf": "ES", "code": 32, "name": "Espírito Santo", "authorizer": "SVRS"},
	{"uf": "GO", "code": 52, "name": "Goiás", "authorizer": "GO"},
	{"uf": "MA", "code": 21, "name": "Maranhão", "authorizer": "SVAN"},
	{"uf": "MG", "code": 31, "name": "Minas Gerais", "authorizer": "MG"},
	{"uf": "MS", "code": 50, "name": "Mato Grosso do Sul", "authorizer": "MS"},
	{"uf": "MT", "code": 51, "name": "Mato Grosso", "authorizer": "MT"},
	{"uf": "PA", "code": 15, "name": "Pará", "authorizer": "SVRS"},
	{"uf": "PB", "code": 25, "name": "Paraíba", "authorizer": "SVRS"},
	{"uf": "PE", "code": 26, "name": "Pernambuco", "authorizer": "PE"},
	{"uf": "PI", "code": 22, "name": "Piauí", "authorizer": "SVRS"},
	{"uf": "PR", "code": 41, "name": "Paraná", "authorizer": "PR"},
	{"uf": "RJ", "code": 33, "name": "Rio de Janeiro", "authorizer": "SVRS"},
	{"uf": "RN", "code": 24, "name": "Rio Grande do Norte", "authorizer": "SVRS"},
	{"uf": "RO", "code": 11, "name": "Rondônia", "authorizer": "SVRS"},
	{"uf": "RR", "code": 14, "name": "Roraima", "authorizer": "SVRS"},
	{"uf": "RS", "code": 43, "name": "Rio Grande do Sul", "authorizer": "RS"},
	{"uf": "SC", "code": 42, "name": "Santa Catarina", "authorizer": "SVRS"},
	{"uf": "SE", "code": 28, "name": "Sergipe", "authorizer": "SVRS"},
	{"uf": "SP", "code": 35, "name": "São Paulo", "authorizer": "SP"},
	{"uf": "TO", "code": 17, "name": "Tocantins", "authorizer": "SVRS"},
}

func (s *Server) companyRoutes(w http.ResponseWriter, r *http.Request, p store.Principal, path []string) bool {
	ctx := r.Context()
	req := w.Header().Get("X-Request-ID")
	if r.URL.Path == "/v1/document-types" && r.Method == "GET" {
		reply(w, r, 200, []map[string]any{
			{
				"kind":                   "nfe",
				"model":                  55,
				"name":                   "NF-e — Nota Fiscal Eletrônica de Produtos e Mercadorias",
				"implemented":            true,
				"schema_version":         "PL_010_V1.30 (4.00 + Reforma Tributária IBS/CBS/IS)",
				"reforma_tributaria_rtc": true,
				"difal_supported":        true,
				"purposes": []map[string]any{
					{"code": 1, "name": "NF-e Normal (Venda, Remessa, Transferência, Demonstração, Bonificação)"},
					{"code": 2, "name": "NF-e Complementar (Complemento de Preço, Quantidade, ICMS, IPI)"},
					{"code": 3, "name": "NF-e de Ajuste (Escrituração e Ajustes Contábeis/Fiscais)"},
					{"code": 4, "name": "NF-e de Devolução de Mercadoria (Retorno/Estorno vinculando referenced_keys)"},
				},
				"directions": []map[string]any{
					{"code": 0, "name": "Entrada (Compra de Produtor Rural, Importação, Retorno de Consumidor)"},
					{"code": 1, "name": "Saída (Venda Interna, Interestadual, Exportação, Remessa)"},
				},
				"destinations": []map[string]any{
					{"code": 1, "name": "Operação Interna (Dentro da mesma UF — CFOP 5xxx / 1xxx)"},
					{"code": 2, "name": "Operação Interestadual (Entre UFs distintas — CFOP 6xxx / 2xxx + DIFAL)"},
					{"code": 3, "name": "Operação com Exterior (Exportação / Importação — CFOP 7xxx / 3xxx, UF EX)"},
				},
				"events": []string{"cancel (110111)", "correction_letter (110110)", "void_range (inutilizacao)", "manifest (210200/210210/210220/210240)", "dist_dfe"},
			},
			{
				"kind":                   "nfce",
				"model":                  65,
				"name":                   "NFC-e — Nota Fiscal de Consumidor Eletrônica (Varejo / PDV / Frente de Caixa)",
				"implemented":            true,
				"schema_version":         "PL_010_V1.30 (4.00 + QR Code + Reforma Tributária IBS/CBS)",
				"reforma_tributaria_rtc": true,
				"purposes": []map[string]any{
					{"code": 1, "name": "NFC-e Normal (Venda presencial ou entrega em domicílio a consumidor final na mesma UF)"},
				},
				"events": []string{"cancel (110111)", "void_range (inutilizacao)"},
			},
			{
				"kind":        "nfse",
				"name":        "NFS-e — Nota Fiscal de Serviços Eletrônica (Padrão Nacional ADN / Municipal)",
				"implemented": false,
			},
		}, "", "")
		return true
	}
	if r.URL.Path == "/v1/states" && r.Method == "GET" {
		reply(w, r, 200, brazilianStates, "", "")
		return true
	}
	if r.URL.Path == "/v1/documents/validate" && r.Method == "POST" {
		var in store.Input
		if decode(w, r, &in) != nil {
			reply(w, r, 422, nil, "INVALID_JSON", "JSON inválido.")
			return true
		}
		normalizeInput(&in)
		if why := s.validate(in, "validation-only"); why != "" {
			reply(w, r, 422, nil, "INVALID_DOCUMENT", why)
			return true
		}
		c, e := s.Store.Company(ctx, p.TenantID, in.CompanyID)
		if e != nil {
			failure(w, r, e)
			return true
		}
		if c.Address == nil {
			failure(w, r, store.ErrCompanyIncomplete)
			return true
		}
		if c.ArchivedAt != nil {
			failure(w, r, store.ErrArchived)
			return true
		}
		inv, _ := fiscal.DecodeInvoice(in.Invoice)
		inv.ApplyCompanyContext(c.Address, c.UF, c.CRT, in.Kind)
		reply(w, r, 200, map[string]any{
			"valid":         true,
			"stage":         "input",
			"persisted":     false,
			"xsd_validated": false,
			"signed":        false,
			"authorized":    false,
			"resolved": map[string]any{
				"kind":        in.Kind,
				"operation":   in.Operation,
				"environment": in.Environment,
				"series":      in.Series,
				"destination": inv.Destination,
				"city_code":   inv.CityCode,
				"items_count": len(inv.Items),
				"payments":    inv.Payments,
			},
		}, "", "")
		return true
	}
	if r.Method == "GET" && (r.URL.Path == "/v1/companies" || r.URL.Path == "/v1/documents") {
		cursor, n, ok := pagination(r)
		if !ok {
			reply(w, r, 422, nil, "INVALID_PAGINATION", "limit deve estar entre 1 e 100; cursor deve ser UUID.")
			return true
		}
		if path[1] == "companies" {
			items, e := s.Store.Companies(ctx, p.TenantID, cursor, n+1)
			if e != nil {
				failure(w, r, e)
				return true
			}
			out := store.Page[store.Company]{Items: items}
			if len(items) > n {
				out.HasMore = true
				out.Items = items[:n]
				out.NextCursor = items[n-1].ID
			}
			reply(w, r, 200, out, "", "")
		} else {
			company, kind := r.URL.Query().Get("company_id"), r.URL.Query().Get("kind")
			if company != "" && !validID(company) || kind != "" && kind != "nfe" && kind != "nfce" {
				reply(w, r, 422, nil, "INVALID_FILTER", "Filtros inválidos.")
				return true
			}
			items, e := s.Store.Documents(ctx, p.TenantID, company, kind, cursor, n+1)
			if e != nil {
				failure(w, r, e)
				return true
			}
			out := store.Page[store.Document]{Items: items}
			if len(items) > n {
				out.HasMore = true
				out.Items = items[:n]
				out.NextCursor = items[n-1].ID
			}
			reply(w, r, 200, out, "", "")
		}
		return true
	}
	if len(path) < 3 || path[1] != "companies" {
		return false
	}
	id := path[2]
	if !validID(id) {
		reply(w, r, 404, nil, "NOT_FOUND", "Empresa não encontrada.")
		return true
	}
	if len(path) == 3 {
		switch r.Method {
		case "GET":
			c, e := s.Store.Company(ctx, p.TenantID, id)
			if e != nil {
				failure(w, r, e)
			} else {
				reply(w, r, 200, c, "", "")
			}
			return true
		case "PATCH":
			var patch store.CompanyPatch
			if decode(w, r, &patch) != nil || patch.Name == nil && patch.IE == nil && patch.CRT == nil && patch.Address == nil && patch.LogoBase64 == nil && patch.ContingencyMode == nil {
				reply(w, r, 422, nil, "INVALID_COMPANY", "Informe name, ie, crt, address, logo_base64 ou contingency_mode; CNPJ e UF são imutáveis.")
				return true
			}
			if patch.Name != nil {
				*patch.Name = strings.TrimSpace(*patch.Name)
			}
			if patch.ContingencyMode != nil && *patch.ContingencyMode != "auto" && *patch.ContingencyMode != "off" && *patch.ContingencyMode != "svc" && *patch.ContingencyMode != "offline" {
				reply(w, r, 422, nil, "INVALID_COMPANY", "contingency_mode deve ser auto, off, svc ou offline.")
				return true
			}
			if patch.Address != nil && !validAddress(patch.Address) || patch.Name != nil && (len(*patch.Name) < 2 || len(*patch.Name) > 120) || patch.IE != nil && !validIE(*patch.IE) || patch.CRT != nil && (*patch.CRT < 1 || *patch.CRT > 4) {
				reply(w, r, 422, nil, "INVALID_COMPANY", "Razão social, IE ou CRT inválido.")
				return true
			}
			c, e := s.Store.PatchCompany(ctx, p, id, patch, req)
			if e != nil {
				failure(w, r, e)
			} else {
				reply(w, r, 200, c, "", "")
			}
			return true
		case "DELETE":
			if e := s.Store.ArchiveCompany(ctx, p, id, req); e != nil {
				failure(w, r, e)
			} else {
				reply(w, r, 200, map[string]bool{"archived": true}, "", "")
			}
			return true
		}
	}
	if len(path) == 4 && path[3] == "certificate" {
		switch r.Method {
		case "PUT":
			s.certificate(w, r, p, id)
			return true
		case "GET":
			c, e := s.Store.CertificateMetadata(ctx, p.TenantID, id)
			if e != nil {
				failure(w, r, e)
			} else {
				reply(w, r, 200, c, "", "")
			}
			return true
		case "DELETE":
			if e := s.Store.DeactivateCertificate(ctx, p, id, req); e != nil {
				failure(w, r, e)
			} else {
				reply(w, r, 200, map[string]bool{"deactivated": true}, "", "")
			}
			return true
		}
	}
	if len(path) == 4 && path[3] == "fiscal" && r.Method == "GET" {
		if _, e := s.Store.Company(ctx, p.TenantID, id); e != nil {
			failure(w, r, e)
			return true
		}
		f, e := s.Store.Settings(ctx, p.TenantID, id)
		if e != nil {
			failure(w, r, e)
		} else {
			reply(w, r, 200, f, "", "")
		}
		return true
	}
	if len(path) == 5 && path[3] == "fiscal" && r.Method == "PUT" {
		var in struct {
			Environment string `json:"environment"`
			Series      *int   `json:"series"`
			Enabled     *bool  `json:"enabled"`
		}
		if decode(w, r, &in) != nil || (in.Environment != "homologation" && in.Environment != "production") || in.Series == nil || *in.Series < 0 || *in.Series > 889 || in.Enabled == nil || (path[4] != "nfe" && path[4] != "nfce") {
			reply(w, r, 422, nil, "INVALID_FISCAL_CONFIG", "Informe environment (homologation ou production), series 0..889 e enabled; tipo nfe ou nfce.")
			return true
		}
		f, e := s.Store.PutSettings(ctx, p, id, store.FiscalSettings{Kind: path[4], Environment: in.Environment, Series: *in.Series, Enabled: *in.Enabled}, req)
		if e != nil {
			failure(w, r, e)
		} else {
			reply(w, r, 200, f, "", "")
		}
		return true
	}
	if len(path) == 5 && path[3] == "sefaz" && path[4] == "status" && r.Method == "GET" {
		s.sefazStatus(w, r, p, id)
		return true
	}
	if len(path) == 4 && path[3] == "void-range" && r.Method == "POST" {
		s.voidRange(w, r, p, id)
		return true
	}
	if len(path) == 5 && path[3] == "dfe" && path[4] == "sync" && r.Method == "POST" {
		s.dfeSync(w, r, p, id)
		return true
	}
	return false
}

func (s *Server) downloadByKind(w http.ResponseWriter, r *http.Request, p store.Principal, doc, format string) {
	if !validID(doc) {
		reply(w, r, 404, nil, "NOT_FOUND", "Documento não encontrado.")
		return
	}
	if _, e := s.Store.Document(r.Context(), p.TenantID, doc); e != nil {
		failure(w, r, e)
		return
	}
	var kinds []string
	var ext string
	if format == "xml" {
		kinds = []string{"authorized_xml", "signed_xml", "cancel_xml", "cce_xml", "protocol_xml"}
		ext = ".xml"
	} else {
		kinds = []string{"danfe_pdf", "preview_pdf", "cce_pdf"}
		ext = ".pdf"
	}
	a, e := s.Store.ArtifactByKind(r.Context(), p.TenantID, doc, kinds...)
	if e != nil {
		failure(w, r, e)
		return
	}
	b, e := s.Objects.Get(p.TenantID, doc, a.ID)
	if e != nil {
		failure(w, r, e)
		return
	}
	w.Header().Set("Content-Type", a.MediaType)
	w.Header().Set("Content-Disposition", `attachment; filename="`+doc+`-`+a.Kind+ext+`"`)
	w.WriteHeader(200)
	_, _ = w.Write(b)
}

func (s *Server) sefazStatus(w http.ResponseWriter, r *http.Request, p store.Principal, companyID string) {
	if !s.Issuance {
		reply(w, r, 422, nil, "TRANSMISSION_DISABLED", "Comunicação SEFAZ desabilitada neste ambiente.")
		return
	}
	ctx := r.Context()
	c, e := s.Store.Company(ctx, p.TenantID, companyID)
	if e != nil {
		failure(w, r, e)
		return
	}
	cert, e := s.Store.ActiveCertificate(ctx, s.Vault, p.TenantID, companyID)
	if e != nil {
		failure(w, r, e)
		return
	}
	uf := r.URL.Query().Get("uf")
	if uf == "" {
		uf = c.UF
	}
	env := r.URL.Query().Get("environment")
	if env == "" {
		env = "homologation"
	}
	if env != "homologation" && env != "production" {
		reply(w, r, 422, nil, "INVALID_ENVIRONMENT", "environment deve ser homologation ou production.")
		return
	}
	var out map[string]any
	if e = s.Engine.Call(ctx, "status", map[string]any{"company": c, "certificate": cert, "uf": uf, "environment": env}, &out); e != nil {
		var ee *engine.Error
		if errors.As(e, &ee) && !ee.Retryable {
			reply(w, r, 422, nil, ee.Code, "Não foi possível consultar status na SEFAZ.")
		} else {
			failure(w, r, e)
		}
		return
	}
	reply(w, r, 200, out, "", "")
}

func (s *Server) cancelDocument(w http.ResponseWriter, r *http.Request, p store.Principal, docID string) {
	if !validID(docID) {
		reply(w, r, 404, nil, "NOT_FOUND", "Documento não encontrado.")
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	if decode(w, r, &body) != nil {
		reply(w, r, 422, nil, "INVALID_JSON", "JSON inválido.")
		return
	}
	reason := strings.TrimSpace(body.Reason)
	if len(reason) < 15 || len(reason) > 255 {
		reply(w, r, 422, nil, "INVALID_CANCEL_REASON", "A justificativa de cancelamento deve ter entre 15 e 255 caracteres.")
		return
	}
	ctx := r.Context()
	d, e := s.Store.Document(ctx, p.TenantID, docID)
	if e != nil {
		failure(w, r, e)
		return
	}
	if !s.Issuance {
		reply(w, r, 422, nil, "TRANSMISSION_DISABLED", "Transmissão fiscal desabilitada.")
		return
	}
	if d.Status != "authorized" || d.AccessKey == nil || *d.AccessKey == "" {
		reply(w, r, 409, nil, "INVALID_DOCUMENT_STATE", "Somente documentos autorizados com chave de acesso podem ser cancelados.")
		return
	}
	var prevResult struct {
		Protocol struct {
			Number string `json:"number"`
		} `json:"protocol"`
	}
	_ = json.Unmarshal(d.Result, &prevResult)
	if prevResult.Protocol.Number == "" {
		reply(w, r, 409, nil, "PROTOCOL_REQUIRED", "Protocolo de autorização ausente no documento.")
		return
	}
	c, e := s.Store.Company(ctx, p.TenantID, d.CompanyID)
	if e != nil {
		failure(w, r, e)
		return
	}
	cert, e := s.Store.ActiveCertificate(ctx, s.Vault, p.TenantID, d.CompanyID)
	if e != nil {
		failure(w, r, e)
		return
	}
	var res engine.Result
	err := s.Engine.Call(ctx, "cancel", map[string]any{
		"company":         c,
		"certificate":     cert,
		"document":        map[string]any{"kind": d.Kind, "environment": d.Environment},
		"access_key":      *d.AccessKey,
		"reason":          reason,
		"protocol_number": prevResult.Protocol.Number,
	}, &res)
	if err != nil {
		var ee *engine.Error
		if errors.As(err, &ee) && !ee.Retryable {
			reply(w, r, 422, nil, ee.Code, "Cancelamento recusado pelo motor fiscal.")
		} else {
			failure(w, r, err)
		}
		return
	}
	var saved []store.Artifact
	for _, a := range res.Artifacts {
		rawBytes, decErr := base64.StdEncoding.DecodeString(a.Base64)
		if decErr == nil {
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
	if e = s.Store.RecordDocumentEvent(ctx, p, docID, "document.canceled", newStatus, res, saved, w.Header().Get("X-Request-ID")); e != nil {
		failure(w, r, e)
		return
	}
	go func(tenantID, dID string) {
		whCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_, _ = s.Store.DispatchDocumentWebhooks(whCtx, tenantID, dID)
	}(p.TenantID, docID)
	reply(w, r, 200, map[string]any{"id": docID, "status": res.Status, "protocol": res.Protocol, "artifacts": saved}, "", "")
}

func (s *Server) correctionLetter(w http.ResponseWriter, r *http.Request, p store.Principal, docID string) {
	if !validID(docID) {
		reply(w, r, 404, nil, "NOT_FOUND", "Documento não encontrado.")
		return
	}
	var body struct {
		Correction string `json:"correction"`
		Sequence   int    `json:"sequence"`
	}
	if decode(w, r, &body) != nil {
		reply(w, r, 422, nil, "INVALID_JSON", "JSON inválido.")
		return
	}
	if body.Sequence == 0 {
		body.Sequence = 1
	}
	correction := strings.TrimSpace(body.Correction)
	if len(correction) < 15 || len(correction) > 1000 || body.Sequence < 1 || body.Sequence > 20 {
		reply(w, r, 422, nil, "INVALID_CCE_INPUT", "A correção deve ter entre 15 e 1000 caracteres e sequência entre 1 e 20.")
		return
	}
	ctx := r.Context()
	d, e := s.Store.Document(ctx, p.TenantID, docID)
	if e != nil {
		failure(w, r, e)
		return
	}
	if !s.Issuance {
		reply(w, r, 422, nil, "TRANSMISSION_DISABLED", "Transmissão fiscal desabilitada.")
		return
	}
	if d.Kind != "nfe" || d.Status != "authorized" || d.AccessKey == nil || *d.AccessKey == "" {
		reply(w, r, 409, nil, "INVALID_DOCUMENT_STATE", "Carta de correção aplica-se somente a NF-e (modelo 55) autorizada.")
		return
	}
	c, e := s.Store.Company(ctx, p.TenantID, d.CompanyID)
	if e != nil {
		failure(w, r, e)
		return
	}
	cert, e := s.Store.ActiveCertificate(ctx, s.Vault, p.TenantID, d.CompanyID)
	if e != nil {
		failure(w, r, e)
		return
	}
	var res engine.Result
	err := s.Engine.Call(ctx, "correction_letter", map[string]any{
		"company":     c,
		"certificate": cert,
		"document":    map[string]any{"kind": d.Kind, "environment": d.Environment},
		"access_key":  *d.AccessKey,
		"correction":  correction,
		"sequence":    body.Sequence,
	}, &res)
	if err != nil {
		var ee *engine.Error
		if errors.As(err, &ee) && !ee.Retryable {
			reply(w, r, 422, nil, ee.Code, "Carta de correção recusada pelo motor fiscal.")
		} else {
			failure(w, r, err)
		}
		return
	}
	var saved []store.Artifact
	for _, a := range res.Artifacts {
		rawBytes, decErr := base64.StdEncoding.DecodeString(a.Base64)
		if decErr == nil {
			if art, putErr := s.Objects.Put(p.TenantID, docID, a.Kind, a.MediaType, rawBytes); putErr == nil {
				saved = append(saved, art)
			}
		}
	}
	res.Artifacts = nil
	if e = s.Store.RecordDocumentEvent(ctx, p, docID, "document.cce", "", res, saved, w.Header().Get("X-Request-ID")); e != nil {
		failure(w, r, e)
		return
	}
	go func(tenantID, dID string) {
		whCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_, _ = s.Store.DispatchDocumentWebhooks(whCtx, tenantID, dID)
	}(p.TenantID, docID)
	reply(w, r, 200, map[string]any{"id": docID, "status": res.Status, "sequence": body.Sequence, "protocol": res.Protocol, "artifacts": saved}, "", "")
}

func (s *Server) voidRange(w http.ResponseWriter, r *http.Request, p store.Principal, companyID string) {
	var body struct {
		Kind        string `json:"kind"`
		Environment string `json:"environment"`
		Series      int    `json:"series"`
		StartNumber int    `json:"start_number"`
		EndNumber   int    `json:"end_number"`
		Reason      string `json:"reason"`
		Year        string `json:"year,omitempty"`
	}
	if decode(w, r, &body) != nil {
		reply(w, r, 422, nil, "INVALID_JSON", "JSON inválido.")
		return
	}
	if body.Kind == "" {
		body.Kind = "nfe"
	}
	if body.Environment == "" {
		body.Environment = "homologation"
	}
	reason := strings.TrimSpace(body.Reason)
	if (body.Kind != "nfe" && body.Kind != "nfce") || (body.Environment != "homologation" && body.Environment != "production") || body.Series < 0 || body.Series > 889 || body.StartNumber < 1 || body.EndNumber < body.StartNumber || len(reason) < 15 || len(reason) > 255 {
		reply(w, r, 422, nil, "INVALID_VOID_INPUT", "Informe kind (nfe/nfce), environment (homologation ou production), series (0..889), start_number, end_number e reason (15..255 caracteres).")
		return
	}
	ctx := r.Context()
	c, e := s.Store.Company(ctx, p.TenantID, companyID)
	if e != nil {
		failure(w, r, e)
		return
	}
	if !s.Issuance {
		reply(w, r, 422, nil, "TRANSMISSION_DISABLED", "Transmissão fiscal desabilitada.")
		return
	}
	cert, e := s.Store.ActiveCertificate(ctx, s.Vault, p.TenantID, companyID)
	if e != nil {
		failure(w, r, e)
		return
	}
	var res engine.Result
	err := s.Engine.Call(ctx, "void_range", map[string]any{
		"company":      c,
		"certificate":  cert,
		"document":     map[string]any{"kind": body.Kind, "environment": body.Environment},
		"series":       body.Series,
		"start_number": body.StartNumber,
		"end_number":   body.EndNumber,
		"reason":       reason,
		"year":         body.Year,
	}, &res)
	if err != nil {
		var ee *engine.Error
		if errors.As(err, &ee) && !ee.Retryable {
			reply(w, r, 422, nil, ee.Code, "Inutilização recusada pelo motor fiscal.")
		} else {
			failure(w, r, err)
		}
		return
	}
	reply(w, r, 200, map[string]any{"company_id": companyID, "status": res.Status, "protocol": res.Protocol}, "", "")
}

func (s *Server) dfeSync(w http.ResponseWriter, r *http.Request, p store.Principal, companyID string) {
	var body struct {
		LastNSU     int    `json:"last_nsu"`
		NSU         int    `json:"nsu"`
		AccessKey   string `json:"access_key,omitempty"`
		Environment string `json:"environment,omitempty"`
	}
	if decode(w, r, &body) != nil || body.LastNSU < 0 || body.NSU < 0 {
		reply(w, r, 422, nil, "INVALID_DFE_INPUT", "Informe last_nsu, nsu ou access_key válidos.")
		return
	}
	env := body.Environment
	if env == "" {
		env = r.URL.Query().Get("environment")
	}
	if env == "" {
		env = "production"
	}
	if env != "homologation" && env != "production" {
		reply(w, r, 422, nil, "INVALID_ENVIRONMENT", "environment deve ser homologation ou production.")
		return
	}
	ctx := r.Context()
	c, e := s.Store.Company(ctx, p.TenantID, companyID)
	if e != nil {
		failure(w, r, e)
		return
	}
	if !s.Issuance {
		reply(w, r, 422, nil, "TRANSMISSION_DISABLED", "Transmissão fiscal desabilitada.")
		return
	}
	if s.Redis != nil {
		if ttl, err := s.Redis.TTL(ctx, "sefaz:distdfe:cooldown:"+companyID).Result(); err == nil && ttl > 0 {
			reply(w, r, 429, nil, "SEFAZ_COOLDOWN_656", "Proteção Anti-656 DistDFe ativa (NT 2014.002): aguarde o intervalo obrigatório de 1 hora após cStat 137.")
			return
		}
	}
	cert, e := s.Store.ActiveCertificate(ctx, s.Vault, p.TenantID, companyID)
	if e != nil {
		failure(w, r, e)
		return
	}
	var out map[string]any
	err := s.Engine.Call(ctx, "dist_dfe", map[string]any{
		"company":     c,
		"certificate": cert,
		"document":    map[string]any{"kind": "nfe", "environment": env},
		"last_nsu":    body.LastNSU,
		"nsu":         body.NSU,
		"access_key":  body.AccessKey,
	}, &out)
	if err != nil {
		var ee *engine.Error
		if errors.As(err, &ee) && !ee.Retryable {
			reply(w, r, 422, nil, ee.Code, "Consulta de DF-e recusada.")
		} else {
			failure(w, r, err)
		}
		return
	}
	if s.Redis != nil {
		if sc, _ := out["status_code"].(string); sc == "137" || sc == "656" {
			_ = s.Redis.Set(ctx, "sefaz:distdfe:cooldown:"+companyID, sc, 61*time.Minute).Err()
		}
	}
	reply(w, r, 200, out, "", "")
}

func validAddress(a *fiscal.Address) bool {
	return a != nil && strings.TrimSpace(a.Street) != "" && strings.TrimSpace(a.Number) != "" && strings.TrimSpace(a.District) != "" && strings.TrimSpace(a.City) != "" && regexp.MustCompile(`^[0-9]{7}$`).MatchString(a.CityCode) && regexp.MustCompile(`^[0-9]{8}$`).MatchString(a.PostalCode) && len(a.State) == 2
}
