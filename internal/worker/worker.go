package worker

import (
	"allspark.local/fiscal/internal/engine"
	"allspark.local/fiscal/internal/fiscal"
	"allspark.local/fiscal/internal/queue"
	"allspark.local/fiscal/internal/store"
	"allspark.local/fiscal/internal/vault"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
	"log"
	"strings"
	"sync"
	"time"
)

type Worker struct {
	Store       *store.Store
	Engine      engine.FiscalProvider
	Vault       *vault.Vault
	Objects     store.Objects
	Redis       *redis.Client
	Concurrency int
}

type Job struct {
	ID, DocumentID, Action, TenantID, CompanyID, Token string
	Input                                              store.Input
	Number                                             int64
	CreatedAt                                          time.Time
	Attempts                                           int
}

func (w *Worker) Run(ctx context.Context, b *queue.Broker) error {
	workers := w.Concurrency
	if workers <= 0 {
		workers = 8
	}
	if e := b.Channel.Qos(workers, 0, false); e != nil {
		return e
	}
	deliveries, e := b.Channel.Consume(queue.Main, "", false, false, false, false, nil)
	if e != nil {
		return e
	}
	errCh := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case d, ok := <-deliveries:
					if !ok {
						select {
						case errCh <- errors.New("consumer closed"):
						default:
						}
						return
					}
					var m queue.Message
					if json.Unmarshal(d.Body, &m) != nil {
						_ = d.Nack(false, false)
						continue
					}
					if _, e := uuid.Parse(m.JobID); e != nil {
						_ = d.Nack(false, false)
						continue
					}
					j, e := w.claim(ctx, m.JobID)
					if errors.Is(e, pgx.ErrNoRows) {
						_ = d.Ack(false)
						continue
					}
					if e != nil {
						select {
						case errCh <- e:
						default:
						}
						return
					}
					if e = w.process(ctx, j); e != nil {
						log.Printf("worker job=%s persistence_failed: %v", j.ID, e)
						select {
						case errCh <- e:
						default:
						}
						return
					}
					if e = d.Ack(false); e != nil {
						select {
						case errCh <- e:
						default:
						}
						return
					}
				}
			}
		}()
	}
	var runErr error
	select {
	case <-ctx.Done():
		runErr = ctx.Err()
	case runErr = <-errCh:
	}
	wg.Wait()
	return runErr
}

func (w *Worker) claim(ctx context.Context, id string) (Job, error) {
	var j Job
	j.ID = id
	j.Token = uuid.NewString()
	tx, e := w.Store.DB.Begin(ctx)
	if e != nil {
		return j, e
	}
	defer tx.Rollback(ctx)
	e = tx.QueryRow(ctx, `UPDATE jobs SET state='processing',attempts=attempts+1,lease_token=$2,lease_until=now()+interval '5 minutes',updated_at=now() WHERE id=$1 AND state='queued' RETURNING document_id::text,action,attempts`, id, j.Token).Scan(&j.DocumentID, &j.Action, &j.Attempts)
	if e != nil {
		return j, e
	}
	var payload []byte
	e = tx.QueryRow(ctx, "SELECT tenant_id::text,company_id::text,payload,number,created_at FROM documents WHERE id=$1", j.DocumentID).Scan(&j.TenantID, &j.CompanyID, &payload, &j.Number, &j.CreatedAt)
	if e != nil {
		return j, e
	}
	if e = json.Unmarshal(payload, &j.Input); e != nil {
		return j, e
	}
	if _, e = tx.Exec(ctx, "UPDATE documents SET status='processing',updated_at=now() WHERE id=$1", j.DocumentID); e != nil {
		return j, e
	}
	return j, tx.Commit(ctx)
}

func (w *Worker) checkSefazGovernor(ctx context.Context, companyID string) (time.Duration, bool) {
	if w.Redis == nil {
		return 0, true
	}
	ttl, err := w.Redis.TTL(ctx, "sefaz:cooldown:656:"+companyID).Result()
	if err == nil && ttl > 0 {
		return ttl, false
	}
	// Rate-pace per CNPJ: max 20 live SEFAZ transmissions per second per company
	rpsKey := "sefaz:rps:" + companyID
	if n, err := w.Redis.Incr(ctx, rpsKey).Result(); err == nil {
		if n == 1 {
			_ = w.Redis.Expire(ctx, rpsKey, time.Second).Err()
		}
		if n > 20 {
			return 2 * time.Second, false
		}
	}
	return 0, true
}

func (w *Worker) acquireSefazSlot(ctx context.Context, companyID string) (func(), bool) {
	if w.Redis == nil {
		return func() {}, true
	}
	key := "sefaz:inflight:" + companyID
	n, err := w.Redis.Incr(ctx, key).Result()
	if err != nil {
		return func() {}, true
	}
	if n == 1 {
		_ = w.Redis.Expire(ctx, key, 60*time.Second).Err()
	}
	release := func() {
		rCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = w.Redis.Decr(rCtx, key).Err()
	}
	// Max 5 simultaneous mTLS connections to SEFAZ per CNPJ to prevent firewall drops
	if n > 5 {
		release()
		return func() {}, false
	}
	return release, true
}

func (w *Worker) tripSefaz656(ctx context.Context, companyID string) {
	if w.Redis == nil {
		return
	}
	_ = w.Redis.Set(ctx, "sefaz:cooldown:656:"+companyID, "1", 62*time.Minute).Err()
}

func (w *Worker) recordSefazOutcome(ctx context.Context, companyID string, res engine.Result) {
	if w.Redis == nil {
		return
	}
	streakKey := "sefaz:rej_streak:" + companyID
	if res.Status == "authorized" {
		_ = w.Redis.Del(ctx, streakKey).Err()
		return
	}
	for _, er := range res.Errors {
		if strings.HasSuffix(er.Code, "656") {
			w.tripSefaz656(ctx, companyID)
			return
		}
	}
	if res.Status == "rejected" {
		n, err := w.Redis.Incr(ctx, streakKey).Result()
		if err == nil {
			if n == 1 {
				_ = w.Redis.Expire(ctx, streakKey, 15*time.Minute).Err()
			}
			// NT 2018.002 blocks at 30 rejected batches. Pause for 5 min at 15 consecutive rejections to protect the CNPJ.
			if n >= 15 {
				_ = w.Redis.Set(ctx, "sefaz:cooldown:656:"+companyID, "pre_656_guard", 5*time.Minute).Err()
			}
		}
	}
}

func (w *Worker) process(ctx context.Context, j Job) error {
	if j.Action == "issue" || j.Action == "reconcile" {
		if ttl, ok := w.checkSefazGovernor(ctx, j.CompanyID); !ok {
			return w.postponeBy(ctx, j, ttl)
		}
		releaseSlot, slotOK := w.acquireSefazSlot(ctx, j.CompanyID)
		if !slotOK {
			return w.postponeBy(ctx, j, 2*time.Second)
		}
		defer releaseSlot()
		conn, err := w.Store.DB.Acquire(ctx)
		if err != nil {
			return w.failed(ctx, j, "LOCK_UNAVAILABLE", true)
		}
		// Lock per document_id so multiple invoices of the same company can be signed and transmitted in parallel
		// (supporting 50,000+ notes/day per company), while preventing duplicate transmission of the same document.
		lane := "doc:" + j.DocumentID
		var locked bool
		err = conn.QueryRow(ctx, "SELECT pg_try_advisory_lock(hashtextextended($1,1))", lane).Scan(&locked)
		if err != nil {
			conn.Release()
			return err
		}
		if !locked {
			conn.Release()
			return w.postponeBy(ctx, j, 2*time.Second)
		}
		defer func() {
			release, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if _, err := conn.Exec(release, "SELECT pg_advisory_unlock(hashtextextended($1,1))", lane); err != nil {
				_ = conn.Conn().Close(release)
			}
			conn.Release()
		}()
	}

	company, e := w.Store.Company(ctx, j.TenantID, j.CompanyID)
	if e != nil {
		return w.failed(ctx, j, "COMPANY_UNAVAILABLE", true)
	}
	var certID string
	var sealed []byte
	e = w.Store.DB.QueryRow(ctx, "SELECT id::text,sealed FROM certificates WHERE tenant_id=$1 AND company_id=$2 AND active AND expires_at>now()", j.TenantID, j.CompanyID).Scan(&certID, &sealed)
	if e != nil {
		if errors.Is(e, pgx.ErrNoRows) {
			return w.failed(ctx, j, "CERTIFICATE_REQUIRED", false)
		}
		return w.failed(ctx, j, "CERTIFICATE_UNAVAILABLE", true)
	}
	plain, e := w.Vault.Open(sealed, j.TenantID+"/"+j.CompanyID+"/"+certID)
	if e != nil {
		return w.failed(ctx, j, "CERTIFICATE_DECRYPT_FAILED", false)
	}
	var cert engine.Certificate
	if json.Unmarshal(plain, &cert) != nil {
		return w.failed(ctx, j, "CERTIFICATE_INVALID", false)
	}
	invoice, err := fiscal.DecodeInvoice(j.Input.Invoice)
	if err != nil {
		return w.failed(ctx, j, "INVALID_INVOICE", false)
	}
	if company.Address == nil && j.Action != "reconcile" {
		return w.failed(ctx, j, "COMPANY_ADDRESS_REQUIRED", false)
	}
	invoice.ApplyCompanyContext(company.Address, company.UF, company.CRT, j.Input.Kind)
	internalInput := j.Input
	internalInput.Invoice, _ = json.Marshal(invoice.Tags(j.Input.Kind))
	in := map[string]any{"certificate": cert, "company": company, "document": internalInput, "number": j.Number, "document_id": j.DocumentID, "issued_at": j.CreatedAt.Format("2006-01-02T15:04:05-07:00")}
	if j.Action == "reconcile" {
		var a store.Artifact
		e = w.Store.DB.QueryRow(ctx, "SELECT id::text FROM artifacts WHERE tenant_id=$1 AND document_id=$2 AND kind='signed_xml'", j.TenantID, j.DocumentID).Scan(&a.ID)
		if e != nil {
			return w.failed(ctx, j, "SIGNED_XML_REQUIRED", false)
		}
		data, e := w.Objects.Get(j.TenantID, j.DocumentID, a.ID)
		if e != nil {
			return w.failed(ctx, j, "ARTIFACT_UNAVAILABLE", true)
		}
		in["signed_xml"] = string(data)
		var res engine.Result
		e = w.Engine.Call(ctx, "reconcile", in, &res)
		if e != nil {
			return w.finish(ctx, j, engine.Result{Status: "reconciliation_required", Errors: []engine.Error{enrichEngineError(engine.Error{Code: "RECONCILIATION_UNAVAILABLE", Retryable: true})}}, nil)
		}
		return w.saveResult(ctx, j, res)
	}
	var prepared engine.Result
	e = w.Engine.Call(ctx, "prepare", in, &prepared)
	if e != nil {
		var ee *engine.Error
		if errors.As(e, &ee) {
			return w.failed(ctx, j, ee.Code, ee.Retryable)
		}
		return w.failed(ctx, j, "ENGINE_UNAVAILABLE", true)
	}
	if j.Action == "generate" {
		return w.saveResult(ctx, j, prepared)
	}
	// The signed XML is durable before transmitting. Only the owner of a live lease
	// can cross this boundary; crashes after it require consultation, never resubmission.
	var signed string
	var artifacts []store.Artifact
	for _, a := range prepared.Artifacts {
		data, e := base64.StdEncoding.DecodeString(a.Base64)
		if e != nil {
			return w.failed(ctx, j, "ENGINE_INVALID_ARTIFACT", false)
		}
		if a.Kind == "signed_xml" {
			signed = string(data)
		}
		saved, e := w.Objects.Put(j.TenantID, j.DocumentID, a.Kind, a.MediaType, data)
		if e != nil {
			return w.failed(ctx, j, "STORAGE_UNAVAILABLE", true)
		}
		artifacts = append(artifacts, saved)
	}
	if signed == "" {
		return w.failed(ctx, j, "SIGNED_XML_REQUIRED", false)
	}
	tx, e := w.Store.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	tag, e := tx.Exec(ctx, "UPDATE jobs SET state='transmitting',updated_at=now() WHERE id=$1 AND lease_token=$2 AND lease_until>now() AND state='processing'", j.ID, j.Token)
	if e != nil {
		return e
	}
	if tag.RowsAffected() != 1 {
		return errors.New("lease lost")
	}
	if e = saveArtifacts(ctx, tx, j, artifacts); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, "UPDATE documents SET status='transmitting',access_key=$2,updated_at=now() WHERE id=$1", j.DocumentID, prepared.AccessKey); e != nil {
		return e
	}
	if e = tx.Commit(ctx); e != nil {
		return e
	}
	in["signed_xml"] = signed
	var result engine.Result
	e = w.Engine.Call(ctx, "transmit", in, &result)
	if e != nil {
		return w.finish(ctx, j, engine.Result{Status: "reconciliation_required", AccessKey: prepared.AccessKey, Errors: []engine.Error{enrichEngineError(engine.Error{Code: "TRANSMISSION_RESULT_UNKNOWN", Retryable: false})}}, nil)
	}
	w.recordSefazOutcome(ctx, j.CompanyID, result)
	return w.saveResult(ctx, j, result)
}

func enrichEngineError(er engine.Error) engine.Error {
	g := fiscal.ExplainError(er.Code, er.Message, er.Retryable)
	er.SefazCode = g.SefazCode
	er.Message = g.Message
	er.Action = g.Action
	return er
}

func saveArtifacts(ctx context.Context, tx pgx.Tx, j Job, artifacts []store.Artifact) error {
	for _, a := range artifacts {
		_, e := tx.Exec(ctx, `INSERT INTO artifacts(id,tenant_id,document_id,kind,media_type,sha256,size) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(tenant_id,document_id,kind) DO UPDATE SET id=EXCLUDED.id,media_type=EXCLUDED.media_type,sha256=EXCLUDED.sha256,size=EXCLUDED.size`, a.ID, j.TenantID, j.DocumentID, a.Kind, a.MediaType, a.SHA256, a.Size)
		if e != nil {
			return e
		}
	}
	return nil
}

func (w *Worker) saveResult(ctx context.Context, j Job, res engine.Result) error {
	var artifacts []store.Artifact
	for _, a := range res.Artifacts {
		data, e := base64.StdEncoding.DecodeString(a.Base64)
		if e != nil {
			return errors.New("invalid artifact")
		}
		saved, e := w.Objects.Put(j.TenantID, j.DocumentID, a.Kind, a.MediaType, data)
		if e != nil {
			return e
		}
		artifacts = append(artifacts, saved)
	}
	for i := range res.Errors {
		res.Errors[i] = enrichEngineError(res.Errors[i])
	}
	return w.finish(ctx, j, res, artifacts)
}

func (w *Worker) finish(ctx context.Context, j Job, res engine.Result, artifacts []store.Artifact) error {
	tx, e := w.Store.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	tag, e := tx.Exec(ctx, "UPDATE jobs SET state='done',lease_until=NULL,updated_at=now() WHERE id=$1 AND lease_token=$2 AND state IN ('processing','transmitting')", j.ID, j.Token)
	if e != nil {
		return e
	}
	if tag.RowsAffected() != 1 {
		return errors.New("lease lost")
	}
	if e = saveArtifacts(ctx, tx, j, artifacts); e != nil {
		return e
	}
	res.Artifacts = nil
	raw, _ := json.Marshal(res)
	if _, e = tx.Exec(ctx, "UPDATE documents SET status=$2,result=$3,access_key=coalesce(nullif($4,''),access_key),updated_at=now() WHERE id=$1", j.DocumentID, res.Status, raw, res.AccessKey); e != nil {
		return e
	}
	_, e = tx.Exec(ctx, "INSERT INTO audit_log(tenant_id,action,resource_id) VALUES($1,$2,$3)", j.TenantID, "document."+res.Status, j.DocumentID)
	if e != nil {
		return e
	}
	if e = tx.Commit(ctx); e != nil {
		return e
	}
	go func(tenant, docID string) {
		whCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_, _ = w.Store.DispatchDocumentWebhooks(whCtx, tenant, docID)
	}(j.TenantID, j.DocumentID)
	return nil
}

func (w *Worker) failed(ctx context.Context, j Job, code string, retry bool) error {
	// A reconciliation job can never convert an uncertain fiscal outcome to a local failure.
	if j.Action == "reconcile" {
		return w.finish(ctx, j, engine.Result{Status: "reconciliation_required", Errors: []engine.Error{enrichEngineError(engine.Error{Code: code, Retryable: retry})}}, nil)
	}
	tx, e := w.Store.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	state := "dead"
	status := "failed"
	route := queue.Dead
	if retry && j.Attempts < 5 {
		state = "queued"
		status = "queued"
		route = queue.Main
	}
	tag, e := tx.Exec(ctx, "UPDATE jobs SET state=$3,last_error=$4,lease_until=NULL,updated_at=now() WHERE id=$1 AND lease_token=$2 AND state='processing'", j.ID, j.Token, state, code)
	if e != nil {
		return e
	}
	if tag.RowsAffected() != 1 {
		return errors.New("lease lost")
	}
	result, _ := json.Marshal(map[string]any{"errors": []engine.Error{enrichEngineError(engine.Error{Code: code, Retryable: state == "queued"})}})
	if _, e = tx.Exec(ctx, "UPDATE documents SET status=$2,result=$3,updated_at=now() WHERE id=$1", j.DocumentID, status, result); e != nil {
		return e
	}
	delay := time.Duration(1<<min(j.Attempts, 8)) * time.Second
	if state == "dead" {
		delay = 0
	}
	_, e = tx.Exec(ctx, "UPDATE outbox SET routing_key=$2,published_at=NULL,available_at=now()+($3*interval '1 second') WHERE job_id=$1", j.ID, route, delay.Seconds())
	if e != nil {
		return e
	}
	if e = tx.Commit(ctx); e != nil {
		return e
	}
	if state == "dead" {
		go func(tenant, docID string) {
			whCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			_, _ = w.Store.DispatchDocumentWebhooks(whCtx, tenant, docID)
		}(j.TenantID, j.DocumentID)
	}
	return nil
}

// Recover is safe to run from multiple dispatchers. Expired transmissions are never retried.
func Recover(ctx context.Context, s *store.Store) error {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	rows, e := tx.Query(ctx, "SELECT id::text,document_id::text,state,action FROM jobs WHERE state IN ('processing','transmitting') AND lease_until<now() FOR UPDATE SKIP LOCKED LIMIT 100")
	if e != nil {
		return e
	}
	type stale struct{ id, doc, state, action string }
	var jobs []stale
	for rows.Next() {
		var j stale
		if e = rows.Scan(&j.id, &j.doc, &j.state, &j.action); e != nil {
			rows.Close()
			return e
		}
		jobs = append(jobs, j)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, j := range jobs {
		state := "queued"
		status := "queued"
		if j.state == "transmitting" || j.action == "reconcile" {
			state = "done"
			status = "reconciliation_required"
		}
		if _, e = tx.Exec(ctx, "UPDATE jobs SET state=$2,lease_token=NULL,lease_until=NULL,updated_at=now() WHERE id=$1", j.id, state); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, "UPDATE documents SET status=$2,updated_at=now() WHERE id=$1", j.doc, status); e != nil {
			return e
		}
		if state == "queued" {
			if _, e = tx.Exec(ctx, "UPDATE outbox SET published_at=NULL,available_at=now() WHERE job_id=$1", j.id); e != nil {
				return e
			}
		}
	}
	return tx.Commit(ctx)
}

func (w *Worker) postponeBy(ctx context.Context, j Job, wait time.Duration) error {
	if wait < 2*time.Second {
		wait = 2 * time.Second
	}
	tx, e := w.Store.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	tag, e := tx.Exec(ctx, "UPDATE jobs SET state='queued',attempts=greatest(attempts-1,0),lease_until=NULL WHERE id=$1 AND lease_token=$2 AND state='processing'", j.ID, j.Token)
	if e != nil {
		return e
	}
	if tag.RowsAffected() != 1 {
		return errors.New("lease lost")
	}
	if _, e = tx.Exec(ctx, "UPDATE documents SET status='queued',updated_at=now() WHERE id=$1", j.DocumentID); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, "UPDATE outbox SET published_at=NULL,available_at=now()+($2*interval '1 second') WHERE job_id=$1", j.ID, wait.Seconds()); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
