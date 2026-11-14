CREATE TABLE IF NOT EXISTS tenants (
 id uuid PRIMARY KEY, name text NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS api_keys (
 id uuid PRIMARY KEY, tenant_id uuid NOT NULL REFERENCES tenants(id), digest bytea NOT NULL UNIQUE,
 scopes text[] NOT NULL, revoked_at timestamptz, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS companies (
 id uuid PRIMARY KEY, tenant_id uuid NOT NULL REFERENCES tenants(id), cnpj text NOT NULL,
 name text NOT NULL, uf char(2) NOT NULL, ie text NOT NULL, crt smallint NOT NULL CHECK(crt BETWEEN 1 AND 4),
 created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(tenant_id,id), UNIQUE(tenant_id,cnpj)
);
CREATE TABLE IF NOT EXISTS certificates (
 id uuid PRIMARY KEY, tenant_id uuid NOT NULL, company_id uuid NOT NULL, sealed bytea NOT NULL,
 fingerprint text NOT NULL, expires_at timestamptz NOT NULL, active boolean NOT NULL DEFAULT true,
 created_at timestamptz NOT NULL DEFAULT now(), FOREIGN KEY(tenant_id,company_id) REFERENCES companies(tenant_id,id)
);
CREATE UNIQUE INDEX IF NOT EXISTS certificates_active ON certificates(tenant_id,company_id) WHERE active;
CREATE TABLE IF NOT EXISTS documents (
 id uuid PRIMARY KEY, tenant_id uuid NOT NULL, company_id uuid NOT NULL,
 idempotency_key text NOT NULL, payload_hash bytea NOT NULL, payload jsonb NOT NULL,
 kind text NOT NULL CHECK(kind IN ('nfe','nfce')), operation text NOT NULL CHECK(operation IN ('generate','issue')),
 environment text NOT NULL CHECK(environment='homologation'), series integer NOT NULL CHECK(series BETWEEN 0 AND 889),
 number bigint NOT NULL CHECK(number BETWEEN 1 AND 999999999),
 status text NOT NULL DEFAULT 'queued', access_key text, result jsonb NOT NULL DEFAULT '{}',
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(tenant_id,company_id) REFERENCES companies(tenant_id,id), UNIQUE(tenant_id,id),
 UNIQUE(tenant_id,idempotency_key)
);
CREATE INDEX IF NOT EXISTS documents_company ON documents(tenant_id,company_id,created_at DESC);
CREATE TABLE IF NOT EXISTS numbering (
 tenant_id uuid NOT NULL, company_id uuid NOT NULL, kind text NOT NULL, environment text NOT NULL,
 series integer NOT NULL, last_number bigint NOT NULL,
 PRIMARY KEY(tenant_id,company_id,kind,environment,series),
 FOREIGN KEY(tenant_id,company_id) REFERENCES companies(tenant_id,id)
);
CREATE TABLE IF NOT EXISTS jobs (
 id uuid PRIMARY KEY, document_id uuid NOT NULL REFERENCES documents(id), action text NOT NULL,
 state text NOT NULL DEFAULT 'queued', attempts integer NOT NULL DEFAULT 0,
 lease_token uuid, lease_until timestamptz, last_error text,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS jobs_one_active ON jobs(document_id) WHERE state IN ('queued','processing','transmitting');
CREATE INDEX IF NOT EXISTS jobs_expired ON jobs(lease_until) WHERE state IN ('processing','transmitting');
CREATE TABLE IF NOT EXISTS outbox (
 job_id uuid PRIMARY KEY REFERENCES jobs(id), routing_key text NOT NULL DEFAULT 'fiscal.jobs',
 available_at timestamptz NOT NULL DEFAULT now(), published_at timestamptz
);
CREATE INDEX IF NOT EXISTS outbox_pending ON outbox(available_at) WHERE published_at IS NULL;
CREATE TABLE IF NOT EXISTS artifacts (
 id uuid PRIMARY KEY, tenant_id uuid NOT NULL, document_id uuid NOT NULL, kind text NOT NULL,
 media_type text NOT NULL, sha256 text NOT NULL, size bigint NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(tenant_id,document_id,kind),
 FOREIGN KEY(tenant_id,document_id) REFERENCES documents(tenant_id,id)
);
CREATE TABLE IF NOT EXISTS audit_log (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, tenant_id uuid NOT NULL REFERENCES tenants(id),
 actor_id uuid, action text NOT NULL, resource_id uuid, request_id text,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS audit_tenant ON audit_log(tenant_id,created_at DESC);

GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA public TO allspark;
GRANT USAGE,SELECT ON ALL SEQUENCES IN SCHEMA public TO allspark;
REVOKE UPDATE,DELETE ON audit_log FROM allspark;

-- Preview numbers are synthetic and must not reserve a fiscal sequence.
ALTER TABLE documents DROP CONSTRAINT IF EXISTS documents_tenant_id_company_id_environment_kind_series_numb_key;
CREATE UNIQUE INDEX IF NOT EXISTS documents_fiscal_number ON documents(tenant_id,company_id,environment,kind,series,number) WHERE operation='issue';
CREATE INDEX IF NOT EXISTS documents_pending_tenant ON documents(tenant_id) WHERE status IN ('queued','processing','transmitting');
