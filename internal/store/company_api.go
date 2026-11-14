package store

import (
	"allspark.local/fiscal/internal/engine"
	"allspark.local/fiscal/internal/fiscal"
	"allspark.local/fiscal/internal/vault"
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"time"
)

var ErrCompanyIncomplete = errors.New("company address required")
var ErrArchived = errors.New("company archived")
var ErrBusy = errors.New("company has unfinished documents")
var ErrFiscalConfig = errors.New("fiscal settings missing, disabled or incompatible")
var ErrCertificateRequired = errors.New("valid certificate required")

type Page[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor,omitempty"`
	HasMore    bool   `json:"has_more"`
}

func (s *Store) Companies(ctx context.Context, tenant, cursor string, limit int) ([]Company, error) {
	rows, e := s.DB.Query(ctx, "SELECT id::text,cnpj,name,uf,ie,crt,archived_at,created_at,updated_at,address,logo_base64,coalesce(contingency_mode,'auto') FROM companies WHERE tenant_id=$1 AND ($2='' OR id>NULLIF($2,'')::uuid) ORDER BY id LIMIT $3", tenant, cursor, limit)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Company{}
	for rows.Next() {
		var c Company
		if e = rows.Scan(&c.ID, &c.CNPJ, &c.Name, &c.UF, &c.IE, &c.CRT, &c.ArchivedAt, &c.CreatedAt, &c.UpdatedAt, &c.Address, &c.LogoBase64, &c.ContingencyMode); e != nil {
			return nil, e
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

type CompanyPatch struct {
	Address         *fiscal.Address `json:"address"`
	Name            *string         `json:"name"`
	IE              *string         `json:"ie"`
	CRT             *int            `json:"crt"`
	LogoBase64      *string         `json:"logo_base64"`
	ContingencyMode *string         `json:"contingency_mode"`
}

func lockCompany(ctx context.Context, tx pgx.Tx, tenant, id string, requireIdle bool) error {
	var archived *time.Time
	if e := tx.QueryRow(ctx, "SELECT archived_at FROM companies WHERE tenant_id=$1 AND id=$2 FOR UPDATE", tenant, id).Scan(&archived); e != nil {
		return e
	}
	if archived != nil {
		return ErrArchived
	}
	if requireIdle {
		var busy bool
		if e := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM documents WHERE tenant_id=$1 AND company_id=$2 AND status IN ('queued','processing','transmitting','reconciliation_required'))", tenant, id).Scan(&busy); e != nil {
			return e
		}
		if busy {
			return ErrBusy
		}
	}
	return nil
}
func (s *Store) PatchCompany(ctx context.Context, p Principal, id string, patch CompanyPatch, request string) (Company, error) {
	var c Company
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return c, e
	}
	defer tx.Rollback(ctx)
	if e = lockCompany(ctx, tx, p.TenantID, id, true); e != nil {
		return c, e
	}
	if patch.Address != nil {
		var uf string
		if e = tx.QueryRow(ctx, "SELECT uf FROM companies WHERE tenant_id=$1 AND id=$2", p.TenantID, id).Scan(&uf); e != nil {
			return c, e
		}
		if patch.Address.State != uf {
			return c, ErrCompanyIncomplete
		}
	}
	_, e = tx.Exec(ctx, "UPDATE companies SET name=coalesce($3,name),ie=coalesce($4,ie),crt=coalesce($5,crt),address=coalesce($6,address),logo_base64=coalesce($7,logo_base64),contingency_mode=coalesce($8,contingency_mode),updated_at=now() WHERE tenant_id=$1 AND id=$2", p.TenantID, id, patch.Name, patch.IE, patch.CRT, patch.Address, patch.LogoBase64, patch.ContingencyMode)
	if e != nil {
		return c, e
	}
	if e = Audit(ctx, tx, p, "company.updated", id, request); e != nil {
		return c, e
	}
	if e = tx.Commit(ctx); e != nil {
		return c, e
	}
	return s.Company(ctx, p.TenantID, id)
}
func (s *Store) ArchiveCompany(ctx context.Context, p Principal, id, request string) error {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	e = lockCompany(ctx, tx, p.TenantID, id, true)
	if errors.Is(e, ErrArchived) {
		return tx.Commit(ctx)
	}
	if e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, "UPDATE companies SET archived_at=now(),updated_at=now() WHERE tenant_id=$1 AND id=$2", p.TenantID, id); e != nil {
		return e
	}
	if e = Audit(ctx, tx, p, "company.archived", id, request); e != nil {
		return e
	}
	return tx.Commit(ctx)
}

type FiscalSettings struct {
	Kind        string    `json:"kind"`
	Environment string    `json:"environment"`
	Series      int       `json:"series"`
	Enabled     bool      `json:"enabled"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (s *Store) Settings(ctx context.Context, tenant, company string) ([]FiscalSettings, error) {
	rows, e := s.DB.Query(ctx, "SELECT kind,environment,series,enabled,updated_at FROM company_fiscal_settings WHERE tenant_id=$1 AND company_id=$2 ORDER BY kind", tenant, company)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []FiscalSettings{}
	for rows.Next() {
		var f FiscalSettings
		if e = rows.Scan(&f.Kind, &f.Environment, &f.Series, &f.Enabled, &f.UpdatedAt); e != nil {
			return nil, e
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
func (s *Store) PutSettings(ctx context.Context, p Principal, company string, f FiscalSettings, request string) (FiscalSettings, error) {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return f, e
	}
	defer tx.Rollback(ctx)
	if e = lockCompany(ctx, tx, p.TenantID, company, true); e != nil {
		return f, e
	}
	e = tx.QueryRow(ctx, `INSERT INTO company_fiscal_settings(tenant_id,company_id,kind,environment,series,enabled) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(tenant_id,company_id,kind) DO UPDATE SET environment=EXCLUDED.environment,series=EXCLUDED.series,enabled=EXCLUDED.enabled,updated_at=now() RETURNING updated_at`, p.TenantID, company, f.Kind, f.Environment, f.Series, f.Enabled).Scan(&f.UpdatedAt)
	if e != nil {
		return f, e
	}
	if e = Audit(ctx, tx, p, "company.fiscal.updated", company, request); e != nil {
		return f, e
	}
	return f, tx.Commit(ctx)
}

type CertificateMetadata struct {
	ID          string    `json:"id"`
	Fingerprint string    `json:"fingerprint"`
	ExpiresAt   time.Time `json:"expires_at"`
	CreatedAt   time.Time `json:"created_at"`
	Expired     bool      `json:"expired"`
}

func (s *Store) CertificateMetadata(ctx context.Context, tenant, company string) (CertificateMetadata, error) {
	var c CertificateMetadata
	e := s.DB.QueryRow(ctx, "SELECT id::text,fingerprint,expires_at,created_at,expires_at<=now() FROM certificates WHERE tenant_id=$1 AND company_id=$2 AND active", tenant, company).Scan(&c.ID, &c.Fingerprint, &c.ExpiresAt, &c.CreatedAt, &c.Expired)
	return c, e
}
func (s *Store) DeactivateCertificate(ctx context.Context, p Principal, company, request string) error {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if e = lockCompany(ctx, tx, p.TenantID, company, true); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, "UPDATE certificates SET active=false WHERE tenant_id=$1 AND company_id=$2 AND active", p.TenantID, company); e != nil {
		return e
	}
	if e = Audit(ctx, tx, p, "certificate.deactivated", company, request); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func (s *Store) Documents(ctx context.Context, tenant, company, kind, cursor string, limit int) ([]Document, error) {
	rows, e := s.DB.Query(ctx, `SELECT id::text,company_id::text,external_ref,webhook_url,kind,operation,environment,series,number,status,access_key,result,created_at FROM documents WHERE tenant_id=$1 AND ($2='' OR company_id=NULLIF($2,'')::uuid) AND ($3='' OR kind=$3) AND ($4='' OR id>NULLIF($4,'')::uuid) ORDER BY id LIMIT $5`, tenant, company, kind, cursor, limit)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Document{}
	for rows.Next() {
		var d Document
		d.Artifacts = []Artifact{}
		if e = rows.Scan(&d.ID, &d.CompanyID, &d.Ref, &d.WebhookURL, &d.Kind, &d.Operation, &d.Environment, &d.Series, &d.Number, &d.Status, &d.AccessKey, &d.Result, &d.CreatedAt); e != nil {
			return nil, e
		}
		d.XMLURL = "/v1/documents/" + d.ID + "/xml"
		d.PDFURL = "/v1/documents/" + d.ID + "/pdf"
		out = append(out, d)
	}
	return out, rows.Err()
}

// CheckIssue runs inside the admission transaction after a share lock on the company.
func checkIssue(ctx context.Context, tx pgx.Tx, tenant string, in Input) error {
	var enabled bool
	var series int
	var environment string
	e := tx.QueryRow(ctx, "SELECT enabled,series,environment FROM company_fiscal_settings WHERE tenant_id=$1 AND company_id=$2 AND kind=$3", tenant, in.CompanyID, in.Kind).Scan(&enabled, &series, &environment)
	if errors.Is(e, pgx.ErrNoRows) {
		return ErrFiscalConfig
	}
	if e != nil {
		return e
	}
	if !enabled || series != in.Series || environment != in.Environment {
		return ErrFiscalConfig
	}
	var valid bool
	if e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM certificates WHERE tenant_id=$1 AND company_id=$2 AND active AND expires_at>now())", tenant, in.CompanyID).Scan(&valid); e != nil {
		return e
	}
	if !valid {
		return ErrCertificateRequired
	}
	return nil
}

func (s *Store) ActiveCertificate(ctx context.Context, v *vault.Vault, tenant, company string) (engine.Certificate, error) {
	var cert engine.Certificate
	var certID string
	var sealed []byte
	e := s.DB.QueryRow(ctx, "SELECT id::text,sealed FROM certificates WHERE tenant_id=$1 AND company_id=$2 AND active AND expires_at>now()", tenant, company).Scan(&certID, &sealed)
	if e != nil {
		if errors.Is(e, pgx.ErrNoRows) {
			return cert, ErrCertificateRequired
		}
		return cert, e
	}
	plain, e := v.Open(sealed, tenant+"/"+company+"/"+certID)
	if e != nil {
		return cert, ErrCertificateRequired
	}
	if e = json.Unmarshal(plain, &cert); e != nil {
		return cert, ErrCertificateRequired
	}
	return cert, nil
}

func (s *Store) ArtifactByKind(ctx context.Context, tenant, doc string, kinds ...string) (Artifact, error) {
	for _, k := range kinds {
		var a Artifact
		e := s.DB.QueryRow(ctx, "SELECT id::text,kind,media_type,sha256,size FROM artifacts WHERE tenant_id=$1 AND document_id=$2 AND kind=$3", tenant, doc, k).Scan(&a.ID, &a.Kind, &a.MediaType, &a.SHA256, &a.Size)
		if e == nil {
			a.DownloadURL = "/v1/documents/" + doc + "/artifacts/" + a.ID
			return a, nil
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return a, e
		}
	}
	return Artifact{}, ErrNotFound
}

func (s *Store) QueryDocument(ctx context.Context, p Principal, id, request string) error {
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
	if status != "reconciliation_required" && status != "authorized" && status != "rejected" && status != "validated" {
		return ErrConflict
	}
	var hasSigned bool
	if e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM artifacts WHERE tenant_id=$1 AND document_id=$2 AND kind='signed_xml')", p.TenantID, id).Scan(&hasSigned); e != nil {
		return e
	}
	if !hasSigned {
		return ErrConflict
	}
	if e = Enqueue(ctx, tx, id, "reconcile"); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, "UPDATE documents SET status='queued',updated_at=now() WHERE id=$1", id); e != nil {
		return e
	}
	if e = Audit(ctx, tx, p, "document.query", id, request); e != nil {
		return e
	}
	return tx.Commit(ctx)
}

func (s *Store) RecordDocumentEvent(ctx context.Context, p Principal, docID, action, newStatus string, result any, artifacts []Artifact, request string) error {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	for _, a := range artifacts {
		_, e = tx.Exec(ctx, `INSERT INTO artifacts(id,tenant_id,document_id,kind,media_type,sha256,size) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(tenant_id,document_id,kind) DO UPDATE SET id=EXCLUDED.id,media_type=EXCLUDED.media_type,sha256=EXCLUDED.sha256,size=EXCLUDED.size`, a.ID, p.TenantID, docID, a.Kind, a.MediaType, a.SHA256, a.Size)
		if e != nil {
			return e
		}
	}
	raw, _ := json.Marshal(result)
	if newStatus != "" {
		if _, e = tx.Exec(ctx, "UPDATE documents SET status=$2,result=$3,updated_at=now() WHERE tenant_id=$4 AND id=$1", docID, newStatus, raw, p.TenantID); e != nil {
			return e
		}
	} else {
		if _, e = tx.Exec(ctx, "UPDATE documents SET result=$2,updated_at=now() WHERE tenant_id=$3 AND id=$1", docID, raw, p.TenantID); e != nil {
			return e
		}
	}
	if e = Audit(ctx, tx, p, action, docID, request); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
