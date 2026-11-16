package httpapi

import (
	"allspark.local/fiscal/internal/engine"
	"allspark.local/fiscal/internal/fiscal"
	"allspark.local/fiscal/internal/store"
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func (s *Server) featureRoutes(w http.ResponseWriter, r *http.Request, p store.Principal, path []string) bool {
	ctx := r.Context()
	reqID := w.Header().Get("X-Request-ID")

	// 1. Built-in Model Context Protocol (MCP) Server (/v1/mcp)
	if r.URL.Path == "/v1/mcp" {
		s.handleMCP(w, r, p)
		return true
	}

	// 2. Certificate Expiration Monitor (/v1/certificates/expiring)
	if r.URL.Path == "/v1/certificates/expiring" && r.Method == "GET" {
		days := 30
		if q := r.URL.Query().Get("days"); q != "" {
			if n, err := strconv.Atoi(q); err == nil && n > 0 && n <= 3650 {
				days = n
			}
		}
		items, err := s.Store.ExpiringCertificates(ctx, p.TenantID, days)
		if err != nil {
			failure(w, r, err)
			return true
		}
		reply(w, r, 200, map[string]any{"within_days": days, "items": items}, "", "")
		return true
	}

	// 3. Webhooks Management (/v1/webhooks and /v1/webhooks/{id})
	if r.URL.Path == "/v1/webhooks" {
		switch r.Method {
		case "GET":
			items, err := s.Store.ListWebhooks(ctx, p.TenantID)
			if err != nil {
				failure(w, r, err)
				return true
			}
			reply(w, r, 200, map[string]any{"items": items}, "", "")
			return true
		case "POST":
			var body struct {
				CompanyID *string  `json:"company_id"`
				URL       string   `json:"url"`
				Secret    string   `json:"secret"`
				Events    []string `json:"events"`
			}
			if decode(w, r, &body) != nil || (!strings.HasPrefix(body.URL, "http://") && !strings.HasPrefix(body.URL, "https://")) {
				reply(w, r, 422, nil, "INVALID_JSON", "Informe uma url válida iniciando com http:// ou https://.")
				return true
			}
			if body.CompanyID != nil && *body.CompanyID != "" && !validID(*body.CompanyID) {
				reply(w, r, 422, nil, "INVALID_COMPANY", "company_id inválido.")
				return true
			}
			wh, err := s.Store.CreateWebhook(ctx, p, body.CompanyID, body.URL, body.Secret, body.Events, reqID)
			if err != nil {
				failure(w, r, err)
				return true
			}
			reply(w, r, 201, wh, "", "")
			return true
		}
	}
	if len(path) == 3 && path[1] == "webhooks" && r.Method == "DELETE" {
		if !validID(path[2]) {
			reply(w, r, 404, nil, "NOT_FOUND", "Webhook não encontrado.")
			return true
		}
		if err := s.Store.DeleteWebhook(ctx, p, path[2], reqID); err != nil {
			failure(w, r, err)
			return true
		}
		reply(w, r, 200, map[string]bool{"deleted": true}, "", "")
		return true
	}

	// 4. Manual Webhook Resend (/v1/documents/{id}/webhook/resend)
	if len(path) == 5 && path[1] == "documents" && path[3] == "webhook" && path[4] == "resend" && r.Method == "POST" {
		if !validID(path[2]) {
			reply(w, r, 404, nil, "NOT_FOUND", "Documento não encontrado.")
			return true
		}
		deliveries, err := s.Store.DispatchDocumentWebhooks(ctx, p.TenantID, path[2])
		if err != nil {
			failure(w, r, err)
			return true
		}
		reply(w, r, 200, map[string]any{"document_id": path[2], "deliveries": deliveries}, "", "")
		return true
	}

	// 5. External Reference (ref) Document Routes (/v1/documents/ref/{ref}...)
	if len(path) >= 4 && path[1] == "documents" && path[2] == "ref" {
		ref := path[3]
		doc, err := s.Store.DocumentByRef(ctx, p.TenantID, ref)
		if err != nil {
			failure(w, r, err)
			return true
		}
		if len(path) == 4 && r.Method == "GET" {
			reply(w, r, 200, doc, "", "")
			return true
		}
		if len(path) == 5 && (path[4] == "xml" || path[4] == "pdf") && r.Method == "GET" {
			s.downloadByKind(w, r, p, doc.ID, path[4])
			return true
		}
		if len(path) == 5 && path[4] == "cancel" && r.Method == "POST" {
			s.cancelDocument(w, r, p, doc.ID)
			return true
		}
		if len(path) == 5 && path[4] == "correction-letter" && r.Method == "POST" {
			s.correctionLetter(w, r, p, doc.ID)
			return true
		}
		if len(path) == 6 && path[4] == "webhook" && path[5] == "resend" && r.Method == "POST" {
			deliveries, err := s.Store.DispatchDocumentWebhooks(ctx, p.TenantID, doc.ID)
			if err != nil {
				failure(w, r, err)
				return true
			}
			reply(w, r, 200, map[string]any{"document_id": doc.ID, "ref": ref, "deliveries": deliveries}, "", "")
			return true
		}
	}

	// 6. Beginner Order-to-Invoice & Order Preview Endpoints
	if r.URL.Path == "/v1/orders/invoice" && r.Method == "POST" {
		s.submitOrderInvoice(w, r, p)
		return true
	}
	if r.URL.Path == "/v1/orders/preview" && r.Method == "POST" {
		s.previewOrderInvoice(w, r, p)
		return true
	}

	// 7. Company Readiness, Tax Profiles, Manifestação do Destinatário (MDe) & Monthly ZIP Backups
	if len(path) >= 4 && path[1] == "companies" && validID(path[2]) {
		companyID := path[2]
		if len(path) == 4 && path[3] == "readiness" && r.Method == "GET" {
			out, err := s.companyReadiness(ctx, p.TenantID, companyID)
			if err != nil {
				failure(w, r, err)
				return true
			}
			reply(w, r, 200, out, "", "")
			return true
		}
		if len(path) == 4 && path[3] == "tax-profiles" {
			switch r.Method {
			case "GET":
				items, err := s.Store.ListTaxProfiles(ctx, p.TenantID, companyID)
				if err != nil {
					failure(w, r, err)
					return true
				}
				reply(w, r, 200, map[string]any{"items": items}, "", "")
				return true
			case "POST":
				var body store.TaxProfile
				if decode(w, r, &body) != nil || strings.TrimSpace(body.Name) == "" || len(body.Taxes) == 0 {
					reply(w, r, 422, nil, "INVALID_JSON", "Informe name e taxes para o perfil tributário.")
					return true
				}
				body.CompanyID = companyID
				tp, err := s.Store.UpsertTaxProfile(ctx, p, body, reqID)
				if err != nil {
					failure(w, r, err)
					return true
				}
				reply(w, r, 201, tp, "", "")
				return true
			}
		}
		if len(path) == 6 && path[3] == "dfe" && path[5] == "manifest" && r.Method == "POST" {
			s.manifestDFe(w, r, p, companyID, path[4])
			return true
		}
		if len(path) == 5 && path[3] == "backups" && r.Method == "GET" {
			s.downloadMonthlyBackup(w, r, p, companyID, path[4])
			return true
		}
	}

	// 8. Auxiliary Lookups (/v1/utils/...)
	if len(path) >= 3 && path[1] == "utils" && r.Method == "GET" {
		s.handleUtils(w, r, p, path)
		return true
	}

	return false
}

type orderInvoiceRequest struct {
	CompanyID     string `json:"company_id"`
	Ref           string `json:"ref"`
	WebhookURL    string `json:"webhook_url"`
	WebhookSecret string `json:"webhook_secret"`
	Kind          string `json:"kind"`
	Operation     string `json:"operation"`
	Environment   string `json:"environment"`
	Series        int    `json:"series"`
	TaxProfile    string `json:"tax_profile"`
	Nature        string `json:"nature"`
	Customer      *struct {
		TaxID       string          `json:"tax_id"`
		Name        string          `json:"name"`
		IEIndicator int             `json:"ie_indicator"`
		IE          string          `json:"ie"`
		Address     *fiscal.Address `json:"address"`
	} `json:"customer"`
	Items []struct {
		SKU         string          `json:"sku"`
		Description string          `json:"description"`
		NCM         string          `json:"ncm"`
		CFOP        string          `json:"cfop"`
		Unit        string          `json:"unit"`
		Quantity    string          `json:"quantity"`
		Price       string          `json:"price"`
		Taxes       json.RawMessage `json:"taxes"`
	} `json:"items"`
	PaymentMethod string `json:"payment_method"`
}

func (s *Server) buildOrderInput(ctx context.Context, tenantID string, order orderInvoiceRequest) store.Input {
	var profile *store.TaxProfile
	if order.TaxProfile != "" {
		if tp, err := s.Store.GetTaxProfile(ctx, tenantID, order.CompanyID, order.TaxProfile); err == nil {
			profile = &tp
		}
	}
	kind := order.Kind
	if kind == "" {
		if order.Customer == nil || order.Customer.Address == nil {
			kind = "nfce"
		} else {
			kind = "nfe"
		}
	}
	items := make([]map[string]any, 0, len(order.Items))
	for i, it := range order.Items {
		ncm := strings.TrimSpace(it.NCM)
		if ncm == "" && profile != nil && profile.NCMDefault != "" {
			ncm = profile.NCMDefault
		}
		if ncm == "" {
			ncm = "61091000"
		}
		row := map[string]any{
			"description": it.Description,
			"ncm":         ncm,
			"quantity":    it.Quantity,
			"unit_price":  it.Price,
		}
		if it.SKU != "" {
			row["sku"] = it.SKU
		} else {
			row["sku"] = fmt.Sprintf("ITEM-%d", i+1)
		}
		if it.Unit != "" {
			row["unit"] = it.Unit
		}
		if it.CFOP != "" {
			row["cfop"] = it.CFOP
		}
		if len(it.Taxes) > 0 {
			var txObj any
			if json.Unmarshal(it.Taxes, &txObj) == nil {
				row["taxes"] = txObj
			}
		} else if profile != nil && len(profile.Taxes) > 0 {
			var txObj any
			if json.Unmarshal(profile.Taxes, &txObj) == nil {
				row["taxes"] = txObj
			}
		}
		items = append(items, row)
	}
	invMap := map[string]any{
		"items": items,
	}
	if order.Nature != "" {
		invMap["nature"] = order.Nature
	}
	if order.Customer != nil && order.Customer.Address != nil {
		ieInd := order.Customer.IEIndicator
		if ieInd == 0 {
			ieInd = 9
		}
		invMap["recipient"] = map[string]any{
			"tax_id":       order.Customer.TaxID,
			"name":         order.Customer.Name,
			"ie_indicator": ieInd,
			"ie":           order.Customer.IE,
			"address":      order.Customer.Address,
		}
	}
	rawInv, _ := json.Marshal(invMap)
	in := store.Input{
		CompanyID:     order.CompanyID,
		Kind:          kind,
		Operation:     order.Operation,
		Environment:   order.Environment,
		Series:        order.Series,
		Ref:           strings.TrimSpace(order.Ref),
		WebhookURL:    strings.TrimSpace(order.WebhookURL),
		WebhookSecret: order.WebhookSecret,
		TaxProfile:    order.TaxProfile,
		Invoice:       rawInv,
	}
	normalizeInput(&in)
	return in
}

func (s *Server) submitOrderInvoice(w http.ResponseWriter, r *http.Request, p store.Principal) {
	var order orderInvoiceRequest
	if decode(w, r, &order) != nil || !validID(order.CompanyID) || len(order.Items) == 0 {
		reply(w, r, 422, nil, "INVALID_DOCUMENT", "Informe company_id válido e ao menos 1 item em items.")
		return
	}
	ctx := r.Context()
	in := s.buildOrderInput(ctx, p.TenantID, order)
	key := r.Header.Get("Idempotency-Key")
	if key == "" && in.Ref != "" {
		key = "order-ref-" + in.Ref
	}
	if why := s.validate(in, key); why != "" {
		reply(w, r, 422, nil, "INVALID_DOCUMENT", why)
		return
	}
	if e := s.admit(ctx); e != nil {
		s.rateError(w, r, e)
		return
	}
	id, replay, e := s.Store.Submit(ctx, p, in, key, w.Header().Get("X-Request-ID"))
	if e != nil {
		failure(w, r, e)
		return
	}
	w.Header().Set("Location", "/v1/documents/"+id)
	resp := map[string]any{
		"id":         id,
		"replayed":   replay,
		"kind":       in.Kind,
		"status_url": "/v1/documents/" + id,
		"xml_url":    "/v1/documents/" + id + "/xml",
		"pdf_url":    "/v1/documents/" + id + "/pdf",
	}
	if in.Ref != "" {
		resp["ref"] = in.Ref
		resp["ref_url"] = "/v1/documents/ref/" + in.Ref
	}
	if in.WebhookURL != "" {
		resp["webhook_url"] = in.WebhookURL
	}
	reply(w, r, 202, resp, "", "")
}

func (s *Server) previewOrderInvoice(w http.ResponseWriter, r *http.Request, p store.Principal) {
	var order orderInvoiceRequest
	if decode(w, r, &order) != nil || !validID(order.CompanyID) || len(order.Items) == 0 {
		reply(w, r, 422, nil, "INVALID_DOCUMENT", "Informe company_id válido e ao menos 1 item em items.")
		return
	}
	out, code, why, err := s.simulateOrder(r.Context(), p.TenantID, order)
	if err != nil {
		failure(w, r, err)
		return
	}
	if code != "" {
		reply(w, r, 422, nil, code, why)
		return
	}
	reply(w, r, 200, out, "", "")
}

func (s *Server) simulateOrder(ctx context.Context, tenantID string, order orderInvoiceRequest) (map[string]any, string, string, error) {
	in := s.buildOrderInput(ctx, tenantID, order)
	if why := s.validate(in, "order-preview-key"); why != "" {
		return nil, "INVALID_DOCUMENT", why, nil
	}
	c, err := s.Store.Company(ctx, tenantID, in.CompanyID)
	if err != nil {
		return nil, "", "", err
	}
	if c.Address == nil {
		return nil, "", "", store.ErrCompanyIncomplete
	}
	inv, _ := fiscal.DecodeInvoice(in.Invoice)
	inv.ApplyCompanyContext(c.Address, c.UF, c.CRT, in.Kind)
	tags := inv.Tags(in.Kind)
	var icmsTot, ibsCbsTot, isTot map[string]any
	for _, t := range tags {
		switch t.Tag {
		case "ICMSTot":
			icmsTot = t.Values
		case "IBSCBSTot":
			ibsCbsTot = t.Values
		case "ISTot":
			isTot = t.Values
		}
	}
	return map[string]any{
		"valid":       true,
		"persisted":   false,
		"company_id":  c.ID,
		"company_uf":  c.UF,
		"company_crt": c.CRT,
		"kind":        in.Kind,
		"environment": in.Environment,
		"series":      in.Series,
		"tax_profile": in.TaxProfile,
		"resolved": map[string]any{
			"nature":         inv.Nature,
			"destination":    inv.Destination,
			"direction":      inv.Direction,
			"purpose":        inv.Purpose,
			"city_code":      inv.CityCode,
			"final_consumer": inv.FinalConsumer,
			"items":          inv.Items,
			"payments":       inv.Payments,
			"icms_totals":    icmsTot,
			"ibs_cbs_totals": ibsCbsTot,
			"is_totals":      isTot,
		},
	}, "", "", nil
}

func (s *Server) companyReadiness(ctx context.Context, tenantID, companyID string) (map[string]any, error) {
	c, err := s.Store.Company(ctx, tenantID, companyID)
	if err != nil {
		return nil, err
	}
	settings, _ := s.Store.Settings(ctx, tenantID, companyID)
	profiles, _ := s.Store.ListTaxProfiles(ctx, tenantID, companyID)
	webhooks, _ := s.Store.ListWebhooks(ctx, tenantID)
	compWebhooks := 0
	for _, wh := range webhooks {
		if wh.CompanyID == nil || *wh.CompanyID == companyID {
			compWebhooks++
		}
	}
	certMeta, certErr := s.Store.CertificateMetadata(ctx, tenantID, companyID)
	hasValidCert := certErr == nil && !certMeta.Expired
	certDays := 0
	if certErr == nil {
		certDays = int(time.Until(certMeta.ExpiresAt).Hours() / 24)
	}
	hasAddress := c.Address != nil && c.Address.Street != "" && c.Address.CityCode != "" && c.Address.State != ""
	nfeEnabled := false
	nfceEnabled := false
	for _, st := range settings {
		if st.Kind == "nfe" && st.Enabled {
			nfeEnabled = true
		}
		if st.Kind == "nfce" && st.Enabled {
			nfceEnabled = true
		}
	}
	var pending []string
	if c.ArchivedAt != nil {
		pending = append(pending, "A empresa está desativada (archived_at != null).")
	}
	if !hasAddress {
		pending = append(pending, "Cadastre o endereço completo da empresa com código IBGE (city_code) em PATCH /v1/companies/"+companyID+".")
	}
	if !hasValidCert {
		pending = append(pending, "Envie um Certificado Digital ICP-Brasil A1 (.pfx) válido em POST /v1/companies/"+companyID+"/certificates.")
	}
	if !nfeEnabled && !nfceEnabled {
		pending = append(pending, "Habilite a série e o ambiente para NF-e ou NFC-e em PUT /v1/companies/"+companyID+"/fiscal/nfe (ou /nfce).")
	}
	active := c.ArchivedAt == nil
	return map[string]any{
		"company_id":                 c.ID,
		"cnpj":                       c.CNPJ,
		"name":                       c.Name,
		"uf":                         c.UF,
		"crt":                        c.CRT,
		"contingency_mode":           c.ContingencyMode,
		"active":                     active,
		"has_address":                hasAddress,
		"has_valid_certificate":      hasValidCert,
		"certificate_days_remaining": certDays,
		"fiscal_settings":            settings,
		"tax_profiles_count":         len(profiles),
		"webhooks_count":             compWebhooks,
		"ready_for_nfe":              active && hasAddress && hasValidCert && nfeEnabled,
		"ready_for_nfce":             active && hasAddress && hasValidCert && nfceEnabled,
		"pending_steps":              pending,
	}, nil
}

func (s *Server) manifestDFe(w http.ResponseWriter, r *http.Request, p store.Principal, companyID, accessKey string) {
	var body struct {
		Type        string `json:"type"`
		Reason      string `json:"reason,omitempty"`
		Sequence    int    `json:"sequence,omitempty"`
		Environment string `json:"environment,omitempty"`
	}
	if decode(w, r, &body) != nil {
		reply(w, r, 422, nil, "INVALID_JSON", "JSON inválido.")
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
	if body.Type == "" {
		body.Type = "ciencia"
	}
	if body.Sequence <= 0 {
		body.Sequence = 1
	}
	if !regexp.MustCompile(`^[0-9A-Z]{44}$`).MatchString(accessKey) || (body.Type != "ciencia" && body.Type != "confirmacao" && body.Type != "desconhecimento" && body.Type != "nao_realizada") {
		reply(w, r, 422, nil, "INVALID_DOCUMENT", "Informe chave de 44 dígitos e type entre ciencia, confirmacao, desconhecimento ou nao_realizada.")
		return
	}
	if (body.Type == "nao_realizada" || body.Type == "desconhecimento") && (len(strings.TrimSpace(body.Reason)) < 15 || len(strings.TrimSpace(body.Reason)) > 255) {
		reply(w, r, 422, nil, "INVALID_DOCUMENT", "Para nao_realizada ou desconhecimento, informe reason entre 15 e 255 caracteres.")
		return
	}
	ctx := r.Context()
	c, e := s.Store.Company(ctx, p.TenantID, companyID)
	if e != nil {
		failure(w, r, e)
		return
	}
	if !s.Issuance {
		reply(w, r, 422, nil, "ISSUANCE_DISABLED", "Transmissão fiscal desabilitada.")
		return
	}
	cert, e := s.Store.ActiveCertificate(ctx, s.Vault, p.TenantID, companyID)
	if e != nil {
		failure(w, r, e)
		return
	}
	var out map[string]any
	err := s.Engine.Call(ctx, "manifest", map[string]any{
		"company":     c,
		"certificate": cert,
		"document":    map[string]any{"kind": "nfe", "environment": env},
		"access_key":  accessKey,
		"type":        body.Type,
		"reason":      body.Reason,
		"sequence":    body.Sequence,
	}, &out)
	if err != nil {
		var ee *engine.Error
		if errors.As(err, &ee) && !ee.Retryable {
			reply(w, r, 422, nil, ee.Code, "Manifestação recusada pela SEFAZ.")
		} else {
			failure(w, r, err)
		}
		return
	}
	reply(w, r, 200, out, "", "")
}

func (s *Server) downloadMonthlyBackup(w http.ResponseWriter, r *http.Request, p store.Principal, companyID, rawPeriod string) {
	period := strings.TrimSuffix(rawPeriod, ".zip")
	t, err := time.Parse("2006-01", period)
	if err != nil {
		reply(w, r, 422, nil, "INVALID_DOCUMENT", "Informe o período no formato YYYY-MM (ex.: 2026-11 ou 2026-11.zip).")
		return
	}
	ctx := r.Context()
	c, err := s.Store.Company(ctx, p.TenantID, companyID)
	if err != nil {
		failure(w, r, err)
		return
	}
	start := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)
	items, err := s.Store.BackupArtifacts(ctx, p.TenantID, companyID, start, end)
	if err != nil {
		failure(w, r, err)
		return
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	manifestRows := make([]map[string]any, 0, len(items))
	for _, it := range items {
		data, err := s.Objects.Get(p.TenantID, it.DocumentID, it.ArtID)
		if err != nil {
			continue
		}
		ext := ".xml"
		if strings.HasSuffix(it.ArtKind, "pdf") {
			ext = ".pdf"
		}
		name := fmt.Sprintf("%s/%s-serie%d-num%d-%s%s", period, it.Kind, it.Series, it.Number, it.ArtKind, ext)
		if it.AccessKey != "" {
			name = fmt.Sprintf("%s/%s-%s-%s%s", period, it.Kind, it.AccessKey, it.ArtKind, ext)
		}
		f, err := zw.Create(name)
		if err == nil {
			_, _ = f.Write(data)
			manifestRows = append(manifestRows, map[string]any{
				"document_id": it.DocumentID,
				"kind":        it.Kind,
				"series":      it.Series,
				"number":      it.Number,
				"access_key":  it.AccessKey,
				"artifact":    it.ArtKind,
				"file":        name,
			})
		}
	}
	mf, _ := zw.Create(period + "/resumo-contador.json")
	summary, _ := json.MarshalIndent(map[string]any{
		"company_id":  c.ID,
		"cnpj":        c.CNPJ,
		"name":        c.Name,
		"period":      period,
		"files_count": len(manifestRows),
		"files":       manifestRows,
	}, "", "  ")
	_, _ = mf.Write(summary)
	_ = zw.Close()

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="backup-%s-%s.zip"`, c.CNPJ, period))
	w.WriteHeader(200)
	_, _ = w.Write(buf.Bytes())
}

var cfopCatalog = []map[string]string{
	{"cfop": "5101", "scope": "internal", "description": "Venda de produção do estabelecimento (mesma UF)"},
	{"cfop": "5102", "scope": "internal", "description": "Venda de mercadoria adquirida ou recebida de terceiros (mesma UF)"},
	{"cfop": "5405", "scope": "internal", "description": "Venda de mercadoria sujeita ao regime de substituição tributária (mesma UF)"},
	{"cfop": "6101", "scope": "interstate", "description": "Venda de produção do estabelecimento (outra UF)"},
	{"cfop": "6102", "scope": "interstate", "description": "Venda de mercadoria adquirida ou recebida de terceiros (outra UF)"},
	{"cfop": "6108", "scope": "interstate", "description": "Venda de mercadoria adquirida de terceiros destinada a não contribuinte (outra UF / DIFAL)"},
	{"cfop": "5202", "scope": "internal", "description": "Devolução de compra para comercialização (mesma UF)"},
	{"cfop": "6202", "scope": "interstate", "description": "Devolução de compra para comercialização (outra UF)"},
	{"cfop": "1202", "scope": "internal_entry", "description": "Devolução de venda de mercadoria adquirida ou recebida de terceiros (entrada mesma UF)"},
	{"cfop": "2202", "scope": "interstate_entry", "description": "Devolução de venda de mercadoria adquirida ou recebida de terceiros (entrada outra UF)"},
	{"cfop": "7102", "scope": "foreign", "description": "Venda de mercadoria adquirida ou recebida de terceiros para o exterior"},
}

var ncmCatalog = []map[string]string{
	{"ncm": "61091000", "description": "Camisetas (t-shirts) e camisetas interiores, de malha, de algodão"},
	{"ncm": "84713012", "description": "Máquinas automáticas para processamento de dados, portáteis (notebooks/tablets)"},
	{"ncm": "85171300", "description": "Telefones para redes celulares (smartphones)"},
	{"ncm": "49019900", "description": "Livros, brochuras e impressos semelhantes"},
	{"ncm": "22021000", "description": "Águas, incluindo as águas minerais e as águas gaseificadas, adicionadas de açúcar"},
	{"ncm": "30049099", "description": "Outros medicamentos constituídos por produtos misturados ou não misturados"},
}

func (s *Server) handleUtils(w http.ResponseWriter, r *http.Request, p store.Principal, path []string) {
	switch path[2] {
	case "errors":
		if len(path) == 4 && path[3] != "" {
			g := fiscal.ExplainError(path[3], "", false)
			reply(w, r, 200, g, "", "")
			return
		}
		reply(w, r, 200, map[string]any{"items": fiscal.AllErrorGuides()}, "", "")
	case "cfop":
		reply(w, r, 200, map[string]any{"items": cfopCatalog}, "", "")
	case "ncm":
		reply(w, r, 200, map[string]any{"items": ncmCatalog}, "", "")
	case "cep":
		if len(path) < 4 {
			reply(w, r, 422, nil, "INVALID_JSON", "Informe o CEP de 8 dígitos em /v1/utils/cep/{cep}.")
			return
		}
		cep := regexp.MustCompile(`[^0-9]`).ReplaceAllString(path[3], "")
		if len(cep) != 8 {
			reply(w, r, 422, nil, "INVALID_JSON", "O CEP deve conter exatamente 8 dígitos numéricos.")
			return
		}
		reply(w, r, 200, map[string]any{"postal_code": cep, "valid_format": true}, "", "")
	case "cnpj":
		if len(path) < 4 {
			reply(w, r, 422, nil, "INVALID_JSON", "Informe o CNPJ de 14 caracteres em /v1/utils/cnpj/{cnpj}.")
			return
		}
		cnpj := strings.ToUpper(regexp.MustCompile(`[^A-Z0-9]`).ReplaceAllString(path[3], ""))
		reply(w, r, 200, map[string]any{
			"cnpj":            cnpj,
			"valid_checksum":  validCNPJ(cnpj),
			"is_alphanumeric": regexp.MustCompile(`[A-Z]`).MatchString(cnpj),
		}, "", "")
	default:
		reply(w, r, 404, nil, "NOT_FOUND", "Utilitário não encontrado.")
	}
}
