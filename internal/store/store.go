package store

import (
	"allspark.local/fiscal/internal/fiscal"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

var ErrConflict = errors.New("idempotency conflict")
var ErrNotFound = errors.New("not found")

type Store struct{ DB *pgxpool.Pool }

func Open(ctx context.Context, url string, max int) (*Store, error) {
	c, e := pgxpool.ParseConfig(url)
	if e != nil {
		return nil, e
	}
	c.MaxConns = int32(max)
	c.ConnConfig.RuntimeParams["statement_timeout"] = "15000"
	p, e := pgxpool.NewWithConfig(ctx, c)
	if e != nil {
		return nil, e
	}
	if e = p.Ping(ctx); e != nil {
		p.Close()
		return nil, e
	}
	return &Store{p}, nil
}

type Principal struct {
	TenantID, KeyID string
	Scopes          []string
}

func (p Principal) Can(s string) bool {
	for _, v := range p.Scopes {
		if v == s {
			return true
		}
	}
	return false
}
func (s *Store) Authenticate(ctx context.Context, token string) (Principal, error) {
	h := sha256.Sum256([]byte(token))
	var p Principal
	e := s.DB.QueryRow(ctx, "SELECT tenant_id::text,id::text,scopes FROM api_keys WHERE digest=$1 AND revoked_at IS NULL", h[:]).Scan(&p.TenantID, &p.KeyID, &p.Scopes)
	return p, e
}
func (s *Store) CreateTenant(ctx context.Context, name string) (string, string, error) {
	id := uuid.NewString()
	key := make([]byte, 32)
	if _, e := rand.Read(key); e != nil {
		return "", "", e
	}
	token := "af_" + base64.RawURLEncoding.EncodeToString(key)
	h := sha256.Sum256([]byte(token))
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return "", "", e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, "INSERT INTO tenants(id,name) VALUES($1,$2)", id, name); e != nil {
		return "", "", e
	}
	if _, e = tx.Exec(ctx, "INSERT INTO api_keys(id,tenant_id,digest,scopes) VALUES($1,$2,$3,$4)", uuid.NewString(), id, h[:], []string{"companies:read", "companies:write", "certificates:read", "certificates:write", "documents:write", "documents:read"}); e != nil {
		return "", "", e
	}
	e = tx.Commit(ctx)
	return id, token, e
}

type Company struct {
	Address         *fiscal.Address `json:"address"`
	ArchivedAt      *time.Time      `json:"archived_at"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
	ID              string          `json:"id"`
	CNPJ            string          `json:"cnpj"`
	Name            string          `json:"name"`
	UF              string          `json:"uf"`
	IE              string          `json:"ie"`
	CRT             int             `json:"crt"`
	LogoBase64      *string         `json:"logo_base64,omitempty"`
	ContingencyMode string          `json:"contingency_mode,omitempty"`
}

func (s *Store) Company(ctx context.Context, tenant, id string) (Company, error) {
	var c Company
	e := s.DB.QueryRow(ctx, "SELECT id::text,cnpj,name,uf,ie,crt,archived_at,created_at,updated_at,address,logo_base64,coalesce(contingency_mode,'auto') FROM companies WHERE tenant_id=$1 AND id=$2", tenant, id).Scan(&c.ID, &c.CNPJ, &c.Name, &c.UF, &c.IE, &c.CRT, &c.ArchivedAt, &c.CreatedAt, &c.UpdatedAt, &c.Address, &c.LogoBase64, &c.ContingencyMode)
	return c, e
}
func Audit(ctx context.Context, tx pgx.Tx, p Principal, action, id, request string) error {
	_, e := tx.Exec(ctx, "INSERT INTO audit_log(tenant_id,actor_id,action,resource_id,request_id) VALUES($1,$2,$3,$4,$5)", p.TenantID, p.KeyID, action, id, request)
	return e
}
func (s *Store) CreateCompany(ctx context.Context, p Principal, c Company, request string) (Company, error) {
	c.ID = uuid.NewString()
	if c.ContingencyMode == "" {
		c.ContingencyMode = "auto"
	}
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return c, e
	}
	defer tx.Rollback(ctx)
	_, e = tx.Exec(ctx, "INSERT INTO companies(id,tenant_id,cnpj,name,uf,ie,crt,address,logo_base64,contingency_mode) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)", c.ID, p.TenantID, c.CNPJ, c.Name, c.UF, c.IE, c.CRT, c.Address, c.LogoBase64, c.ContingencyMode)
	if e != nil {
		return c, e
	}
	if e = Audit(ctx, tx, p, "company.created", c.ID, request); e != nil {
		return c, e
	}
	if e = tx.Commit(ctx); e != nil {
		return c, e
	}
	return s.Company(ctx, p.TenantID, c.ID)
}
func (s *Store) SaveCertificate(ctx context.Context, p Principal, company, id string, sealed []byte, fingerprint string, expires time.Time, request string) error {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if e = lockCompany(ctx, tx, p.TenantID, company, false); e != nil {
		return e
	}
	// Rotation must remain possible to recover an uncertain result after expiry.
	// The company lock excludes admission/other rotations; active work still blocks it.
	var busy bool
	if e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM documents WHERE tenant_id=$1 AND company_id=$2 AND status IN ('queued','processing','transmitting'))", p.TenantID, company).Scan(&busy); e != nil {
		return e
	}
	if busy {
		return ErrBusy
	}
	if _, e = tx.Exec(ctx, "UPDATE certificates SET active=false WHERE tenant_id=$1 AND company_id=$2 AND active", p.TenantID, company); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, "INSERT INTO certificates(id,tenant_id,company_id,sealed,fingerprint,expires_at) VALUES($1,$2,$3,$4,$5,$6)", id, p.TenantID, company, sealed, fingerprint, expires); e != nil {
		return e
	}
	if e = Audit(ctx, tx, p, "certificate.registered", id, request); e != nil {
		return e
	}
	return tx.Commit(ctx)
}

type Input struct {
	CompanyID     string          `json:"company_id"`
	Kind          string          `json:"kind"`
	Operation     string          `json:"operation"`
	Environment   string          `json:"environment"`
	Series        int             `json:"series"`
	Ref           string          `json:"ref,omitempty"`
	WebhookURL    string          `json:"webhook_url,omitempty"`
	WebhookSecret string          `json:"webhook_secret,omitempty"`
	TaxProfile    string          `json:"tax_profile,omitempty"`
	Invoice       json.RawMessage `json:"invoice"`
}
type Document struct {
	ID          string          `json:"id"`
	CompanyID   string          `json:"company_id"`
	Ref         *string         `json:"ref,omitempty"`
	WebhookURL  *string         `json:"webhook_url,omitempty"`
	Kind        string          `json:"kind"`
	Operation   string          `json:"operation"`
	Environment string          `json:"environment"`
	Series      int             `json:"series"`
	Number      int64           `json:"number"`
	Status      string          `json:"status"`
	AccessKey   *string         `json:"access_key"`
	XMLURL      string          `json:"xml_url,omitempty"`
	PDFURL      string          `json:"pdf_url,omitempty"`
	Result      json.RawMessage `json:"result"`
	CreatedAt   time.Time       `json:"created_at"`
	Artifacts   []Artifact      `json:"artifacts"`
}

func (s *Store) Submit(ctx context.Context, p Principal, input Input, key, request string) (string, bool, error) {
	raw, e := json.Marshal(input)
	if e != nil {
		return "", false, e
	}
	var canonical any
	if e = json.Unmarshal(raw, &canonical); e != nil {
		return "", false, e
	}
	raw, _ = json.Marshal(canonical)
	hash := sha256.Sum256(raw)
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return "", false, e
	}
	defer tx.Rollback(ctx)
	// Serializes concurrent reuse before reserving a number; no number is consumed by replays.
	if _, e = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", p.TenantID+":"+key); e != nil {
		return "", false, e
	}
	var existing string
	var prev []byte
	e = tx.QueryRow(ctx, "SELECT id::text,payload_hash FROM documents WHERE tenant_id=$1 AND idempotency_key=$2", p.TenantID, key).Scan(&existing, &prev)
	if e == nil {
		if string(prev) != string(hash[:]) {
			return "", false, ErrConflict
		}
		return existing, true, tx.Commit(ctx)
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return "", false, e
	}
	if input.Ref != "" {
		var refDoc string
		var refHash []byte
		e = tx.QueryRow(ctx, "SELECT id::text,payload_hash FROM documents WHERE tenant_id=$1 AND external_ref=$2", p.TenantID, input.Ref).Scan(&refDoc, &refHash)
		if e == nil {
			if string(refHash) != string(hash[:]) {
				return "", false, ErrConflict
			}
			return refDoc, true, tx.Commit(ctx)
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return "", false, e
		}
	}
	var archived *time.Time
	var address *fiscal.Address
	if e = tx.QueryRow(ctx, "SELECT archived_at,address FROM companies WHERE tenant_id=$1 AND id=$2 FOR SHARE", p.TenantID, input.CompanyID).Scan(&archived, &address); e != nil {
		if errors.Is(e, pgx.ErrNoRows) {
			return "", false, ErrNotFound
		}
		return "", false, e
	}
	if archived != nil {
		return "", false, ErrArchived
	}
	if address == nil {
		return "", false, ErrCompanyIncomplete
	}
	if input.Operation == "issue" {
		if e = checkIssue(ctx, tx, p.TenantID, input); e != nil {
			return "", false, e
		}
	}

	number := int64(1) // Non-fiscal placeholder for previews; never reserves a sequence.
	if input.Operation == "issue" {
		e = tx.QueryRow(ctx, `INSERT INTO numbering(tenant_id,company_id,kind,environment,series,last_number) VALUES($1,$2,$3,$4,$5,1) ON CONFLICT(tenant_id,company_id,kind,environment,series) DO UPDATE SET last_number=numbering.last_number+1 RETURNING last_number`, p.TenantID, input.CompanyID, input.Kind, input.Environment, input.Series).Scan(&number)
		if e != nil {
			return "", false, e
		}
	}
	id := uuid.NewString()
	var extRef, whURL, whSecret *string
	if input.Ref != "" {
		extRef = &input.Ref
	}
	if input.WebhookURL != "" {
		whURL = &input.WebhookURL
	}
	if input.WebhookSecret != "" {
		whSecret = &input.WebhookSecret
	}
	_, e = tx.Exec(ctx, `INSERT INTO documents(id,tenant_id,company_id,idempotency_key,payload_hash,payload,kind,operation,environment,series,number,external_ref,webhook_url,webhook_secret) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, id, p.TenantID, input.CompanyID, key, hash[:], raw, input.Kind, input.Operation, input.Environment, input.Series, number, extRef, whURL, whSecret)
	if e != nil {
		return "", false, e
	}
	if e = Enqueue(ctx, tx, id, input.Operation); e != nil {
		return "", false, e
	}
	if e = Audit(ctx, tx, p, "document.queued", id, request); e != nil {
		return "", false, e
	}
	return id, false, tx.Commit(ctx)
}
func Enqueue(ctx context.Context, tx pgx.Tx, document, action string) error {
	id := uuid.NewString()
	if _, e := tx.Exec(ctx, "INSERT INTO jobs(id,document_id,action) VALUES($1,$2,$3)", id, document, action); e != nil {
		return e
	}
	_, e := tx.Exec(ctx, "INSERT INTO outbox(job_id) VALUES($1)", id)
	return e
}
func (s *Store) Document(ctx context.Context, tenant, id string) (Document, error) {
	var d Document
	d.Artifacts = []Artifact{}
	e := s.DB.QueryRow(ctx, `SELECT id::text,company_id::text,external_ref,webhook_url,kind,operation,environment,series,number,status,access_key,result,created_at FROM documents WHERE tenant_id=$1 AND id=$2`, tenant, id).Scan(&d.ID, &d.CompanyID, &d.Ref, &d.WebhookURL, &d.Kind, &d.Operation, &d.Environment, &d.Series, &d.Number, &d.Status, &d.AccessKey, &d.Result, &d.CreatedAt)
	if e != nil {
		return d, e
	}
	d.XMLURL = "/v1/documents/" + d.ID + "/xml"
	d.PDFURL = "/v1/documents/" + d.ID + "/pdf"
	rows, e := s.DB.Query(ctx, "SELECT id::text,kind,media_type,sha256,size FROM artifacts WHERE tenant_id=$1 AND document_id=$2 ORDER BY created_at", tenant, id)
	if e != nil {
		return d, e
	}
	defer rows.Close()
	for rows.Next() {
		var a Artifact
		if e = rows.Scan(&a.ID, &a.Kind, &a.MediaType, &a.SHA256, &a.Size); e != nil {
			return d, e
		}
		a.DownloadURL = "/v1/documents/" + id + "/artifacts/" + a.ID
		d.Artifacts = append(d.Artifacts, a)
	}
	return d, rows.Err()
}

type Artifact struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	MediaType   string `json:"media_type"`
	SHA256      string `json:"sha256"`
	Size        int64  `json:"size"`
	DownloadURL string `json:"download_url"`
}

func (s *Store) Reconcile(ctx context.Context, p Principal, id, request string) error {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	var status string
	e = tx.QueryRow(ctx, "SELECT status FROM documents WHERE tenant_id=$1 AND id=$2 FOR UPDATE", p.TenantID, id).Scan(&status)
	if e != nil {
		return e
	}
	if status != "reconciliation_required" {
		return ErrConflict
	}
	if e = Enqueue(ctx, tx, id, "reconcile"); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, "UPDATE documents SET status='queued',updated_at=now() WHERE id=$1", id); e != nil {
		return e
	}
	if e = Audit(ctx, tx, p, "document.reconcile", id, request); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
