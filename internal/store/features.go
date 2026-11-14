package store

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

func (s *Store) DocumentByRef(ctx context.Context, tenant, ref string) (Document, error) {
	var id string
	e := s.DB.QueryRow(ctx, "SELECT id::text FROM documents WHERE tenant_id=$1 AND external_ref=$2", tenant, ref).Scan(&id)
	if e != nil {
		return Document{}, e
	}
	return s.Document(ctx, tenant, id)
}

type Webhook struct {
	ID        string    `json:"id"`
	CompanyID *string   `json:"company_id,omitempty"`
	URL       string    `json:"url"`
	Secret    string    `json:"secret,omitempty"`
	Events    []string  `json:"events"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
}

type WebhookDelivery struct {
	ID         string    `json:"id"`
	DocumentID string    `json:"document_id"`
	URL        string    `json:"url"`
	Event      string    `json:"event"`
	StatusCode int       `json:"status_code"`
	Attempts   int       `json:"attempts"`
	Success    bool      `json:"success"`
	CreatedAt  time.Time `json:"created_at"`
}

func (s *Store) CreateWebhook(ctx context.Context, p Principal, companyID *string, url, secret string, events []string, request string) (Webhook, error) {
	var w Webhook
	w.ID = uuid.NewString()
	w.CompanyID = companyID
	w.URL = strings.TrimSpace(url)
	if strings.TrimSpace(secret) == "" {
		b := make([]byte, 16)
		_, _ = rand.Read(b)
		secret = "whsec_" + hex.EncodeToString(b)
	}
	w.Secret = secret
	if len(events) == 0 {
		events = []string{"document.authorized", "document.rejected", "document.canceled", "document.validated", "document.reconciliation_required"}
	}
	w.Events = events
	w.Active = true
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return w, e
	}
	defer tx.Rollback(ctx)
	e = tx.QueryRow(ctx, `INSERT INTO webhooks(id,tenant_id,company_id,url,secret,events,active) VALUES($1,$2,$3,$4,$5,$6,true) RETURNING created_at`, w.ID, p.TenantID, companyID, w.URL, w.Secret, w.Events).Scan(&w.CreatedAt)
	if e != nil {
		return w, e
	}
	if e = Audit(ctx, tx, p, "webhook.created", w.ID, request); e != nil {
		return w, e
	}
	return w, tx.Commit(ctx)
}

func (s *Store) ListWebhooks(ctx context.Context, tenant string) ([]Webhook, error) {
	rows, e := s.DB.Query(ctx, "SELECT id::text,company_id::text,url,events,active,created_at FROM webhooks WHERE tenant_id=$1 AND active ORDER BY created_at DESC", tenant)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Webhook{}
	for rows.Next() {
		var w Webhook
		if e = rows.Scan(&w.ID, &w.CompanyID, &w.URL, &w.Events, &w.Active, &w.CreatedAt); e != nil {
			return nil, e
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

func (s *Store) DeleteWebhook(ctx context.Context, p Principal, id, request string) error {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	tag, e := tx.Exec(ctx, "UPDATE webhooks SET active=false WHERE tenant_id=$1 AND id=$2 AND active", p.TenantID, id)
	if e != nil {
		return e
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	if e = Audit(ctx, tx, p, "webhook.deleted", id, request); e != nil {
		return e
	}
	return tx.Commit(ctx)
}

type webhookTarget struct {
	url, secret string
}

// webhookSem bounds concurrent outbound HTTP webhook deliveries per process
// so bursts of thousands of documents never flood or trigger WAF blocks on client servers.
var webhookSem = make(chan struct{}, 16)

// VerifyWebhookSignature validates the HMAC-SHA256 signature header ("sha256=<hex>") against the raw payload body.
func VerifyWebhookSignature(secret, signatureHeader string, body []byte) bool {
	if secret == "" || !strings.HasPrefix(signatureHeader, "sha256=") {
		return false
	}
	sigHex := strings.TrimPrefix(signatureHeader, "sha256=")
	sigBytes, err := hex.DecodeString(sigHex)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(sigBytes, mac.Sum(nil))
}

func isRetryableHTTPStatus(code int) bool {
	return code == 408 || code == 425 || code == 429 || code == 500 || code == 502 || code == 503 || code == 504
}

// DispatchDocumentWebhooks sends HMAC-SHA256 signed webhook notifications for a document
// both to the per-request webhook_url (if provided on the document) and to any active
// tenant/company webhooks subscribed to the event, with bounded concurrency and automatic retry.
func (s *Store) DispatchDocumentWebhooks(ctx context.Context, tenant, docID string) ([]WebhookDelivery, error) {
	doc, e := s.Document(ctx, tenant, docID)
	if e != nil {
		return nil, e
	}
	event := "document." + doc.Status
	targets := []webhookTarget{}
	seen := map[string]bool{}

	var docURL, docSecret *string
	_ = s.DB.QueryRow(ctx, "SELECT webhook_url,webhook_secret FROM documents WHERE tenant_id=$1 AND id=$2", tenant, docID).Scan(&docURL, &docSecret)
	if docURL != nil && strings.TrimSpace(*docURL) != "" {
		sec := ""
		if docSecret != nil {
			sec = *docSecret
		}
		targets = append(targets, webhookTarget{url: strings.TrimSpace(*docURL), secret: sec})
		seen[strings.TrimSpace(*docURL)] = true
	}

	rows, e := s.DB.Query(ctx, "SELECT url,secret,events FROM webhooks WHERE tenant_id=$1 AND active AND (company_id IS NULL OR company_id=$2::uuid)", tenant, doc.CompanyID)
	if e == nil {
		for rows.Next() {
			var u, sec string
			var evs []string
			if rows.Scan(&u, &sec, &evs) == nil && !seen[u] {
				matched := len(evs) == 0
				for _, ev := range evs {
					if ev == "*" || ev == event || ev == doc.Status || (ev == "document.cancelled" && doc.Status == "canceled") {
						matched = true
						break
					}
				}
				if matched {
					targets = append(targets, webhookTarget{url: u, secret: sec})
					seen[u] = true
				}
			}
		}
		rows.Close()
	}

	if len(targets) == 0 {
		return nil, nil
	}

	nowUTC := time.Now().UTC()
	var parsedResult any
	if len(doc.Result) > 0 {
		_ = json.Unmarshal(doc.Result, &parsedResult)
	}
	payload := map[string]any{
		"event":        event,
		"triggered_at": nowUTC.Format(time.RFC3339),
		"timestamp":    nowUTC.Format(time.RFC3339),
		"document_id":  doc.ID,
		"company_id":   doc.CompanyID,
		"ref":          doc.Ref,
		"kind":         doc.Kind,
		"environment":  doc.Environment,
		"series":       doc.Series,
		"number":       doc.Number,
		"status":       doc.Status,
		"access_key":   doc.AccessKey,
		"xml_url":      doc.XMLURL,
		"pdf_url":      doc.PDFURL,
		"result":       parsedResult,
		"document":     doc,
	}
	body, _ := json.Marshal(payload)
	client := &http.Client{Timeout: 8 * time.Second}
	deliveries := make([]WebhookDelivery, 0, len(targets))

	// Acquire concurrency slot to prevent flooding receivers during high-volume bursts
	select {
	case webhookSem <- struct{}{}:
		defer func() { <-webhookSem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	for _, t := range targets {
		del := WebhookDelivery{
			ID:         uuid.NewString(),
			DocumentID: doc.ID,
			URL:        t.url,
			Event:      event,
			CreatedAt:  time.Now().UTC(),
		}
		var lastBody string
		for attempt := 1; attempt <= 3; attempt++ {
			del.Attempts = attempt
			tsHeader := strconv.FormatInt(time.Now().Unix(), 10)
			req, err := http.NewRequestWithContext(ctx, "POST", t.url, bytes.NewReader(body))
			if err != nil {
				lastBody = err.Error()
				break
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("User-Agent", "AllSpark-Fiscal-Webhook/1.0")
			req.Header.Set("X-AllSpark-Event", event)
			req.Header.Set("X-AllSpark-Delivery", del.ID)
			req.Header.Set("X-AllSpark-Timestamp", tsHeader)
			if t.secret != "" {
				mac := hmac.New(sha256.New, []byte(t.secret))
				mac.Write(body)
				sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))
				req.Header.Set("X-AllSpark-Signature", sig)
				req.Header.Set("X-AllSpark-Signature-256", sig)
			}
			resp, err := client.Do(req)
			if err == nil {
				del.StatusCode = resp.StatusCode
				del.Success = resp.StatusCode >= 200 && resp.StatusCode < 300
				rb, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
				resp.Body.Close()
				lastBody = string(rb)
				if del.Success || !isRetryableHTTPStatus(resp.StatusCode) || attempt == 3 {
					break
				}
				wait := time.Duration(attempt*400) * time.Millisecond
				if ra := resp.Header.Get("Retry-After"); ra != "" {
					if secs, err := strconv.Atoi(strings.TrimSpace(ra)); err == nil && secs > 0 && secs <= 5 {
						wait = time.Duration(secs) * time.Second
					}
				}
				select {
				case <-time.After(wait):
				case <-ctx.Done():
				}
			} else {
				lastBody = err.Error()
				if attempt == 3 {
					break
				}
				select {
				case <-time.After(time.Duration(attempt*400) * time.Millisecond):
				case <-ctx.Done():
				}
			}
		}
		_, _ = s.DB.Exec(ctx, `INSERT INTO webhook_deliveries(id,tenant_id,document_id,url,event,status_code,response_body,attempts,success) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
			del.ID, tenant, doc.ID, t.url, event, del.StatusCode, lastBody, del.Attempts, del.Success)
		deliveries = append(deliveries, del)
	}
	return deliveries, nil
}

type TaxProfile struct {
	ID             string          `json:"id"`
	CompanyID      string          `json:"company_id"`
	Name           string          `json:"name"`
	Description    string          `json:"description"`
	CFOPInternal   string          `json:"cfop_internal"`
	CFOPInterstate string          `json:"cfop_interstate"`
	NCMDefault     string          `json:"ncm_default"`
	Taxes          json.RawMessage `json:"taxes"`
	CreatedAt      time.Time       `json:"created_at"`
}

func (s *Store) UpsertTaxProfile(ctx context.Context, p Principal, prof TaxProfile, request string) (TaxProfile, error) {
	prof.ID = uuid.NewString()
	prof.Name = strings.TrimSpace(prof.Name)
	if prof.CFOPInternal == "" {
		prof.CFOPInternal = "5102"
	}
	if prof.CFOPInterstate == "" {
		prof.CFOPInterstate = "6102"
	}
	if prof.NCMDefault == "" {
		prof.NCMDefault = "61091000"
	}
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return prof, e
	}
	defer tx.Rollback(ctx)
	e = tx.QueryRow(ctx, `INSERT INTO tax_profiles(id,tenant_id,company_id,name,description,cfop_internal,cfop_interstate,ncm_default,taxes)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT(tenant_id,company_id,name) DO UPDATE SET
			description=EXCLUDED.description,
			cfop_internal=EXCLUDED.cfop_internal,
			cfop_interstate=EXCLUDED.cfop_interstate,
			ncm_default=EXCLUDED.ncm_default,
			taxes=EXCLUDED.taxes
		RETURNING id::text,created_at`,
		prof.ID, p.TenantID, prof.CompanyID, prof.Name, prof.Description, prof.CFOPInternal, prof.CFOPInterstate, prof.NCMDefault, prof.Taxes,
	).Scan(&prof.ID, &prof.CreatedAt)
	if e != nil {
		return prof, e
	}
	if e = Audit(ctx, tx, p, "company.tax_profile.upserted", prof.CompanyID, request); e != nil {
		return prof, e
	}
	return prof, tx.Commit(ctx)
}

func (s *Store) ListTaxProfiles(ctx context.Context, tenant, company string) ([]TaxProfile, error) {
	rows, e := s.DB.Query(ctx, "SELECT id::text,company_id::text,name,description,cfop_internal,cfop_interstate,ncm_default,taxes,created_at FROM tax_profiles WHERE tenant_id=$1 AND company_id=$2 ORDER BY name", tenant, company)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []TaxProfile{}
	for rows.Next() {
		var tp TaxProfile
		if e = rows.Scan(&tp.ID, &tp.CompanyID, &tp.Name, &tp.Description, &tp.CFOPInternal, &tp.CFOPInterstate, &tp.NCMDefault, &tp.Taxes, &tp.CreatedAt); e != nil {
			return nil, e
		}
		out = append(out, tp)
	}
	return out, rows.Err()
}

func (s *Store) GetTaxProfile(ctx context.Context, tenant, company, name string) (TaxProfile, error) {
	var tp TaxProfile
	e := s.DB.QueryRow(ctx, "SELECT id::text,company_id::text,name,description,cfop_internal,cfop_interstate,ncm_default,taxes,created_at FROM tax_profiles WHERE tenant_id=$1 AND company_id=$2 AND name=$3", tenant, company, name).Scan(&tp.ID, &tp.CompanyID, &tp.Name, &tp.Description, &tp.CFOPInternal, &tp.CFOPInterstate, &tp.NCMDefault, &tp.Taxes, &tp.CreatedAt)
	return tp, e
}

type ExpiringCertificate struct {
	CertificateID string    `json:"certificate_id"`
	CompanyID     string    `json:"company_id"`
	CNPJ          string    `json:"cnpj"`
	CompanyName   string    `json:"company_name"`
	Fingerprint   string    `json:"fingerprint"`
	ExpiresAt     time.Time `json:"expires_at"`
	DaysRemaining int       `json:"days_remaining"`
	Expired       bool      `json:"expired"`
}

func (s *Store) ExpiringCertificates(ctx context.Context, tenant string, withinDays int) ([]ExpiringCertificate, error) {
	if withinDays <= 0 {
		withinDays = 30
	}
	rows, e := s.DB.Query(ctx, `SELECT cert.id::text,c.id::text,c.cnpj,c.name,cert.fingerprint,cert.expires_at,
		EXTRACT(DAY FROM (cert.expires_at - now()))::int, cert.expires_at <= now()
		FROM certificates cert
		JOIN companies c ON c.tenant_id = cert.tenant_id AND c.id = cert.company_id
		WHERE cert.tenant_id=$1 AND cert.active AND cert.expires_at <= now() + ($2 * interval '1 day')
		ORDER BY cert.expires_at ASC`, tenant, withinDays)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []ExpiringCertificate{}
	for rows.Next() {
		var item ExpiringCertificate
		if e = rows.Scan(&item.CertificateID, &item.CompanyID, &item.CNPJ, &item.CompanyName, &item.Fingerprint, &item.ExpiresAt, &item.DaysRemaining, &item.Expired); e != nil {
			return nil, e
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

type BackupArtifactItem struct {
	DocumentID string
	Kind       string
	Series     int
	Number     int64
	AccessKey  string
	ArtID      string
	ArtKind    string
}

func (s *Store) BackupArtifacts(ctx context.Context, tenant, company string, start, end time.Time) ([]BackupArtifactItem, error) {
	rows, e := s.DB.Query(ctx, `SELECT d.id::text,d.kind,d.series,d.number,coalesce(d.access_key,''),a.id::text,a.kind
		FROM documents d
		JOIN artifacts a ON a.tenant_id=d.tenant_id AND a.document_id=d.id
		WHERE d.tenant_id=$1 AND d.company_id=$2 AND d.created_at >= $3 AND d.created_at < $4
		ORDER BY d.created_at ASC`, tenant, company, start, end)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []BackupArtifactItem{}
	for rows.Next() {
		var b BackupArtifactItem
		if e = rows.Scan(&b.DocumentID, &b.Kind, &b.Series, &b.Number, &b.AccessKey, &b.ArtID, &b.ArtKind); e != nil {
			return nil, e
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

