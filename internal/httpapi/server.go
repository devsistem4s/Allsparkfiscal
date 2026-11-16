package httpapi

import (
	"allspark.local/fiscal/internal/engine"
	"allspark.local/fiscal/internal/fiscal"
	"allspark.local/fiscal/internal/observability"
	"allspark.local/fiscal/internal/store"
	"allspark.local/fiscal/internal/vault"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/redis/go-redis/v9"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"
)

type Server struct {
	Metrics    *observability.Metrics
	Store      *store.Store
	Redis      *redis.Client
	Vault      *vault.Vault
	Objects    store.Objects
	Engine     engine.FiscalProvider
	Issuance   bool
	Rate       int
	MaxPending int
}
type APIError struct {
	Code      string `json:"code"`
	SefazCode string `json:"sefaz_code,omitempty"`
	Field     string `json:"field,omitempty"`
	Message   string `json:"message"`
	Action    string `json:"action,omitempty"`
	Source    string `json:"source"`
	Retryable bool   `json:"retryable"`
}
type Envelope struct {
	Data      any       `json:"data"`
	Error     *APIError `json:"error"`
	RequestID string    `json:"request_id"`
}

func makeAPIError(code, message string, retryable bool) APIError {
	g := fiscal.ExplainError(code, message, retryable)
	return APIError{
		Code:      g.Code,
		SefazCode: g.SefazCode,
		Field:     g.Field,
		Message:   g.Message,
		Action:    g.Action,
		Source:    g.Source,
		Retryable: g.Retryable,
	}
}

func reply(w http.ResponseWriter, r *http.Request, status int, data any, code, message string) {
	out := Envelope{Data: data, RequestID: w.Header().Get("X-Request-ID")}
	if code != "" {
		er := makeAPIError(code, message, status == 429 || status == 503)
		out.Error = &er
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(out)
}
func failure(w http.ResponseWriter, r *http.Request, e error) {
	var pe *pgconn.PgError
	switch {
	case errors.Is(e, pgx.ErrNoRows) || errors.Is(e, store.ErrNotFound):
		reply(w, r, 404, nil, "NOT_FOUND", "Recurso não encontrado.")
	case errors.Is(e, store.ErrCompanyIncomplete):
		reply(w, r, 422, nil, "COMPANY_ADDRESS_REQUIRED", "Cadastre endereço válido e compatível com a UF da empresa.")
	case errors.Is(e, store.ErrArchived):
		reply(w, r, 409, nil, "COMPANY_ARCHIVED", "Empresa desativada.")
	case errors.Is(e, store.ErrBusy):
		reply(w, r, 409, nil, "COMPANY_BUSY", "Aguarde a conclusão ou reconciliação dos documentos pendentes.")
	case errors.Is(e, store.ErrFiscalConfig):
		reply(w, r, 422, nil, "FISCAL_CONFIG_REQUIRED", "Habilite o tipo de nota com ambiente e série correspondentes na configuração fiscal.")
	case errors.Is(e, store.ErrCertificateRequired):
		reply(w, r, 422, nil, "CERTIFICATE_REQUIRED", "Cadastre um certificado A1 válido antes de emitir.")
	case errors.Is(e, store.ErrConflict):
		reply(w, r, 409, nil, "CONFLICT", "Chave reutilizada com outro conteúdo ou estado incompatível.")
	case errors.As(e, &pe) && pe.Code == "23505":
		reply(w, r, 409, nil, "CONFLICT", "Registro já existente.")
	default:
		reply(w, r, 503, nil, "SERVICE_UNAVAILABLE", "Serviço temporariamente indisponível.")
	}
}
func decode(w http.ResponseWriter, r *http.Request, dst any) error {
	if strings.Split(r.Header.Get("Content-Type"), ";")[0] != "application/json" {
		return errors.New("content type")
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if e := d.Decode(dst); e != nil {
		return e
	}
	var extra any
	if e := d.Decode(&extra); e != io.EOF {
		return errors.New("extra input")
	}
	return nil
}

var rateScript = redis.NewScript(`local n=redis.call('INCRBY',KEYS[1],ARGV[1]); if n==tonumber(ARGV[1]) then redis.call('EXPIRE',KEYS[1],ARGV[2]) end; return n`)

func (s *Server) limit(ctx context.Context, key string, cost, limit, ttl int) error {
	n, e := rateScript.Run(ctx, s.Redis, []string{"rate:" + key}, cost, ttl).Int()
	if e != nil {
		return e
	}
	if n > limit {
		return errRate
	}
	return nil
}

var errRate = errors.New("rate limit")

func (s *Server) Handler() http.Handler { return http.HandlerFunc(s.serve) }
func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	if s.Metrics != nil {
		start := time.Now()
		defer s.Metrics.Observe(start)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Request-ID", uuid.NewString())
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	ctx, cancel := context.WithTimeout(r.Context(), 100*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	if r.URL.Path == "/healthz" && r.Method == "GET" {
		reply(w, r, 200, map[string]string{"status": "ok"}, "", "")
		return
	}
	if r.URL.Path == "/readyz" && r.Method == "GET" {
		if s.Store.DB.Ping(ctx) != nil || s.Redis.Ping(ctx).Err() != nil {
			reply(w, r, 503, nil, "NOT_READY", "Dependência indisponível.")
			return
		}
		reply(w, r, 200, map[string]bool{"ready": true}, "", "")
		return
	}
	if !strings.HasPrefix(r.URL.Path, "/v1/") {
		reply(w, r, 404, nil, "NOT_FOUND", "Recurso não encontrado.")
		return
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	sum := sha256.Sum256([]byte(host))
	if e := s.limit(ctx, "ip:"+hex.EncodeToString(sum[:]), 1, 10000, 60); e != nil {
		s.rateError(w, r, e)
		return
	}
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") || len(token) > 128 || len(token) < 32 {
		reply(w, r, 401, nil, "UNAUTHORIZED", "Credencial inválida.")
		return
	}
	p, e := s.Store.Authenticate(ctx, token)
	if e != nil {
		if errors.Is(e, pgx.ErrNoRows) {
			reply(w, r, 401, nil, "UNAUTHORIZED", "Credencial inválida.")
		} else {
			failure(w, r, e)
		}
		return
	}
	if e = s.limit(ctx, "tenant:"+p.TenantID, 1, s.Rate, 1); e != nil {
		s.rateError(w, r, e)
		return
	}
	path := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	scope := requiredScope(r.Method, path)
	if !p.Can(scope) && !(strings.HasSuffix(scope, ":read") && p.Can(strings.TrimSuffix(scope, ":read")+":write")) {
		reply(w, r, 403, nil, "FORBIDDEN", "Permissão insuficiente.")
		return
	}
	if s.companyRoutes(w, r, p, path) {
		return
	}
	if s.featureRoutes(w, r, p, path) {
		return
	}
	switch {
	case r.URL.Path == "/v1/capabilities" && r.Method == "GET":
		reply(w, r, 200, map[string]any{
			"multi_tenant":           true,
			"queue":                  "rabbitmq",
			"generation":             true,
			"issuance_enabled":       s.Issuance,
			"simplified_national":    true,
			"reforma_tributaria_rtc": true,
			"difal":                  true,
			"webhooks":               true,
			"mcp_server":             "/v1/mcp",
			"worker_concurrency":     true,
			"sefaz_anti_656_shield":  true,
			"environments":           []string{"homologation", "production"},
			"document_types":         []string{"nfe", "nfce"},
			"events":                 []string{"cancel", "correction_letter", "void_range", "query", "sefaz_status", "dist_dfe", "manifest"},
			"nfse":                   "not_implemented",
			"bulk_max":               50,
		}, "", "")
	case r.URL.Path == "/v1/companies" && r.Method == "POST":
		s.createCompany(w, r, p)
	case len(path) == 4 && path[1] == "companies" && path[3] == "certificates" && r.Method == "POST":
		if !validID(path[2]) {
			reply(w, r, 404, nil, "NOT_FOUND", "Recurso não encontrado.")
			return
		}
		s.certificate(w, r, p, path[2])
	case r.URL.Path == "/v1/documents" && r.Method == "POST":
		s.submit(w, r, p)
	case r.URL.Path == "/v1/documents/bulk" && r.Method == "POST":
		s.bulk(w, r, p)
	case len(path) == 3 && path[1] == "documents" && r.Method == "GET":
		if !validID(path[2]) {
			reply(w, r, 404, nil, "NOT_FOUND", "Recurso não encontrado.")
			return
		}
		d, e := s.Store.Document(ctx, p.TenantID, path[2])
		if e != nil {
			failure(w, r, e)
			return
		}
		reply(w, r, 200, d, "", "")
	case len(path) == 4 && path[1] == "documents" && path[3] == "reconcile" && r.Method == "POST":
		if !validID(path[2]) {
			reply(w, r, 404, nil, "NOT_FOUND", "Recurso não encontrado.")
			return
		}
		if e = s.Store.Reconcile(ctx, p, path[2], w.Header().Get("X-Request-ID")); e != nil {
			failure(w, r, e)
			return
		}
		reply(w, r, 202, map[string]string{"id": path[2], "status": "queued"}, "", "")
	case len(path) == 4 && path[1] == "documents" && path[3] == "query" && r.Method == "POST":
		if !validID(path[2]) {
			reply(w, r, 404, nil, "NOT_FOUND", "Recurso não encontrado.")
			return
		}
		if e := s.limit(ctx, "sefaz_query:"+path[2], 1, 8, 3600); e != nil {
			s.rateError(w, r, e)
			return
		}
		if e = s.Store.QueryDocument(ctx, p, path[2], w.Header().Get("X-Request-ID")); e != nil {
			failure(w, r, e)
			return
		}
		reply(w, r, 202, map[string]string{"id": path[2], "status": "queued"}, "", "")
	case len(path) == 4 && path[1] == "documents" && (path[3] == "xml" || path[3] == "pdf") && r.Method == "GET":
		s.downloadByKind(w, r, p, path[2], path[3])
	case len(path) == 4 && path[1] == "documents" && path[3] == "cancel" && r.Method == "POST":
		s.cancelDocument(w, r, p, path[2])
	case len(path) == 4 && path[1] == "documents" && path[3] == "correction-letter" && r.Method == "POST":
		s.correctionLetter(w, r, p, path[2])
	case len(path) == 5 && path[1] == "documents" && path[3] == "artifacts" && r.Method == "GET":
		s.download(w, r, p, path[2], path[4])
	default:
		reply(w, r, 404, nil, "NOT_FOUND", "Rota ou método não encontrado.")
	}
}
func (s *Server) rateError(w http.ResponseWriter, r *http.Request, e error) {
	if errors.Is(e, errRate) {
		w.Header().Set("Retry-After", "1")
		reply(w, r, 429, nil, "RATE_LIMITED", "Limite temporário atingido.")
	} else {
		failure(w, r, e)
	}
}
func validID(id string) bool { _, e := uuid.Parse(id); return e == nil }

var cnpjPattern = regexp.MustCompile(`^[A-Z0-9]{12}[0-9]{2}$`)

func validCNPJ(v string) bool {
	if !cnpjPattern.MatchString(v) || v == strings.Repeat(v[:1], 14) {
		return false
	}
	for size := 12; size <= 13; size++ {
		sum := 0
		weight := 2
		for i := size - 1; i >= 0; i-- {
			sum += (int(v[i]) - 48) * weight
			weight++
			if weight > 9 {
				weight = 2
			}
		}
		digit := 11 - sum%11
		if digit >= 10 {
			digit = 0
		}
		if int(v[size]-'0') != digit {
			return false
		}
	}
	return true
}
func (s *Server) createCompany(w http.ResponseWriter, r *http.Request, p store.Principal) {
	var input struct {
		Address         *fiscal.Address `json:"address"`
		CNPJ            string          `json:"cnpj"`
		Name            string          `json:"name"`
		UF              string          `json:"uf"`
		IE              string          `json:"ie"`
		CRT             int             `json:"crt"`
		LogoBase64      *string         `json:"logo_base64"`
		ContingencyMode string          `json:"contingency_mode"`
	}
	if decode(w, r, &input) != nil {
		reply(w, r, 422, nil, "INVALID_COMPANY", "Campos inválidos.")
		return
	}
	c := store.Company{Address: input.Address, CNPJ: input.CNPJ, Name: strings.TrimSpace(input.Name), UF: input.UF, IE: input.IE, CRT: input.CRT, LogoBase64: input.LogoBase64, ContingencyMode: input.ContingencyMode}
	if c.ContingencyMode != "" && c.ContingencyMode != "auto" && c.ContingencyMode != "off" && c.ContingencyMode != "svc" && c.ContingencyMode != "offline" {
		reply(w, r, 422, nil, "INVALID_COMPANY", "contingency_mode deve ser auto, off, svc ou offline.")
		return
	}
	if c.Address == nil || !validAddress(c.Address) || c.Address.State != c.UF || !validCNPJ(c.CNPJ) || len(c.Name) < 2 || len(c.Name) > 120 || !strings.Contains("|AC|AL|AP|AM|BA|CE|DF|ES|GO|MA|MT|MS|MG|PA|PB|PR|PE|PI|RJ|RN|RS|RO|RR|SC|SP|SE|TO|", "|"+c.UF+"|") || len(c.UF) != 2 || !validIE(c.IE) || c.CRT < 1 || c.CRT > 4 {
		reply(w, r, 422, nil, "INVALID_COMPANY", "Informe CNPJ, razão social, UF, IE e CRT válidos.")
		return
	}
	c, e := s.Store.CreateCompany(r.Context(), p, c, w.Header().Get("X-Request-ID"))
	if e != nil {
		failure(w, r, e)
		return
	}
	reply(w, r, 201, c, "", "")
}
func (s *Server) certificate(w http.ResponseWriter, r *http.Request, p store.Principal, company string) {
	if e := s.limit(r.Context(), "certificate:"+p.TenantID, 1, 5, 60); e != nil {
		s.rateError(w, r, e)
		return
	}

	c, e := s.Store.Company(r.Context(), p.TenantID, company)
	if e != nil {
		failure(w, r, e)
		return
	}
	var in engine.Certificate
	if decode(w, r, &in) != nil || len(in.PFX) > 2<<20 || len(in.PFX) < 16 || len(in.Password) > 1024 {
		reply(w, r, 422, nil, "INVALID_CERTIFICATE", "Informe certificado A1 e senha.")
		return
	}
	var info engine.CertInfo
	if e = s.Engine.Call(r.Context(), "certificate", in, &info); e != nil {
		var ee *engine.Error
		if errors.As(e, &ee) && !ee.Retryable {
			reply(w, r, 422, nil, ee.Code, "Certificado recusado: formato, senha, titularidade, cadeia ou validade inválidos.")
		} else {
			failure(w, r, e)
		}
		return
	}
	if info.CNPJ != c.CNPJ || !info.ExpiresAt.After(time.Now()) {
		reply(w, r, 422, nil, "CERTIFICATE_COMPANY_MISMATCH", "O certificado deve estar válido e pertencer ao CNPJ cadastrado.")
		return
	}
	id := uuid.NewString()
	raw, _ := json.Marshal(in)
	sealed := s.Vault.Seal(raw, p.TenantID+"/"+company+"/"+id)
	if e = s.Store.SaveCertificate(r.Context(), p, company, id, sealed, info.Fingerprint, info.ExpiresAt, w.Header().Get("X-Request-ID")); e != nil {
		failure(w, r, e)
		return
	}
	reply(w, r, 201, map[string]any{"id": id, "fingerprint": info.Fingerprint, "expires_at": info.ExpiresAt}, "", "")
}
func (s *Server) validate(in store.Input, key string) string {
	if !validID(in.CompanyID) || len(key) < 8 || len(key) > 128 || strings.TrimSpace(key) != key {
		return "company_id ou Idempotency-Key inválido"
	}
	if in.Kind != "nfe" && in.Kind != "nfce" {
		return "tipo não implementado"
	}
	if in.Environment != "homologation" && in.Environment != "production" {
		return "environment deve ser homologation ou production"
	}
	if in.Operation != "generate" && in.Operation != "issue" {
		return "operation deve ser generate ou issue"
	}
	if in.Operation == "issue" && !s.Issuance {
		return "transmissão fiscal desabilitada"
	}
	if in.Series < 0 || in.Series > 889 {
		return "série inválida"
	}
	if len(in.Invoice) > 2<<20 {
		return "invoice acima de 2 MiB"
	}
	invoice, err := fiscal.DecodeInvoice(in.Invoice)
	if err != nil {
		return err.Error()
	}
	if err = invoice.ValidateSupportedTaxes(); err != nil {
		return err.Error()
	}

	return ""
}
func (s *Server) admit(ctx context.Context) error {
	var n int
	e := s.Store.DB.QueryRow(ctx, "SELECT count(*) FROM (SELECT 1 FROM jobs WHERE state IN ('queued','processing','transmitting') LIMIT $1) q", s.MaxPending).Scan(&n)
	if e != nil {
		return e
	}
	if n >= s.MaxPending {
		return errRate
	}
	return nil
}
func normalizeInput(in *store.Input) {
	if in.Kind == "" {
		in.Kind = "nfe"
	}
	if in.Operation == "" {
		in.Operation = "issue"
	}
	if in.Environment == "" {
		in.Environment = "homologation"
	}
	if in.Series == 0 {
		in.Series = 1
	}
}

func (s *Server) submit(w http.ResponseWriter, r *http.Request, p store.Principal) {
	var in store.Input
	if decode(w, r, &in) != nil {
		reply(w, r, 422, nil, "INVALID_JSON", "JSON inválido ou acima do limite.")
		return
	}
	if in.Ref == "" {
		in.Ref = strings.TrimSpace(r.URL.Query().Get("ref"))
	}
	if in.WebhookURL != "" && !strings.HasPrefix(in.WebhookURL, "http://") && !strings.HasPrefix(in.WebhookURL, "https://") {
		reply(w, r, 422, nil, "INVALID_DOCUMENT", "webhook_url deve iniciar com http:// ou https://")
		return
	}
	normalizeInput(&in)
	key := r.Header.Get("Idempotency-Key")
	if why := s.validate(in, key); why != "" {
		reply(w, r, 422, nil, "INVALID_DOCUMENT", why)
		return
	}
	if e := s.admit(r.Context()); e != nil {
		s.rateError(w, r, e)
		return
	}
	id, replay, e := s.Store.Submit(r.Context(), p, in, key, w.Header().Get("X-Request-ID"))
	if e != nil {
		failure(w, r, e)
		return
	}
	w.Header().Set("Location", "/v1/documents/"+id)
	resp := map[string]any{"id": id, "replayed": replay, "status_url": "/v1/documents/" + id, "xml_url": "/v1/documents/" + id + "/xml", "pdf_url": "/v1/documents/" + id + "/pdf"}
	if in.Ref != "" {
		resp["ref"] = in.Ref
		resp["ref_url"] = "/v1/documents/ref/" + in.Ref
	}
	if in.WebhookURL != "" {
		resp["webhook_url"] = in.WebhookURL
	}
	reply(w, r, 202, resp, "", "")
}
func (s *Server) bulk(w http.ResponseWriter, r *http.Request, p store.Principal) {
	var b struct {
		Documents []struct {
			IdempotencyKey string      `json:"idempotency_key"`
			Document       store.Input `json:"document"`
		} `json:"documents"`
	}
	if decode(w, r, &b) != nil || len(b.Documents) == 0 || len(b.Documents) > 50 {
		reply(w, r, 422, nil, "INVALID_BATCH", "Envie entre 1 e 50 documentos.")
		return
	}
	if e := s.limit(r.Context(), "tenant:"+p.TenantID, len(b.Documents), s.Rate, 1); e != nil {
		s.rateError(w, r, e)
		return
	}
	if e := s.admit(r.Context()); e != nil {
		s.rateError(w, r, e)
		return
	}
	results := make([]map[string]any, 0, len(b.Documents))
	for i, item := range b.Documents {
		normalizeInput(&item.Document)
		row := map[string]any{"index": i}
		if why := s.validate(item.Document, item.IdempotencyKey); why != "" {
			row["status"] = 422
			row["error"] = makeAPIError("INVALID_DOCUMENT", why, false)
		} else {
			id, replay, e := s.Store.Submit(r.Context(), p, item.Document, item.IdempotencyKey, w.Header().Get("X-Request-ID"))
			if e != nil {
				status := 503
				code := "SERVICE_UNAVAILABLE"
				if errors.Is(e, store.ErrConflict) {
					status = 409
					code = "CONFLICT"
				}
				if errors.Is(e, store.ErrNotFound) {
					status = 404
					code = "NOT_FOUND"
				}
				if errors.Is(e, store.ErrArchived) || errors.Is(e, store.ErrBusy) {
					status = 409
					code = "COMPANY_UNAVAILABLE"
				}
				if errors.Is(e, store.ErrFiscalConfig) {
					status = 422
					code = "FISCAL_CONFIG_REQUIRED"
				}
				if errors.Is(e, store.ErrCertificateRequired) {
					status = 422
					code = "CERTIFICATE_REQUIRED"
				}
				if errors.Is(e, store.ErrCompanyIncomplete) {
					status = 422
					code = "COMPANY_ADDRESS_REQUIRED"
				}
				row["status"] = status
				row["error"] = makeAPIError(code, "Documento não aceito.", status == 503)
			} else {
				row["status"] = 202
				row["id"] = id
				row["replayed"] = replay
			}
		}
		results = append(results, row)
	}
	reply(w, r, 207, results, "", "")
}
func (s *Server) download(w http.ResponseWriter, r *http.Request, p store.Principal, doc, id string) {
	if !validID(doc) || !validID(id) {
		reply(w, r, 404, nil, "NOT_FOUND", "Arquivo não encontrado.")
		return
	}
	a, e := s.Store.Artifact(r.Context(), p.TenantID, doc, id)
	if e != nil {
		failure(w, r, e)
		return
	}
	b, e := s.Objects.Get(p.TenantID, doc, id)
	if e != nil {
		failure(w, r, e)
		return
	}
	w.Header().Set("Content-Type", a.MediaType)
	w.Header().Set("Content-Disposition", `attachment; filename="`+a.ID+`"`)
	w.WriteHeader(200)
	_, _ = w.Write(b)
}
