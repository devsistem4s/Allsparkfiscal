ALTER TABLE companies
  ADD COLUMN IF NOT EXISTS logo_base64 text,
  ADD COLUMN IF NOT EXISTS contingency_mode text NOT NULL DEFAULT 'auto' CHECK (contingency_mode IN ('auto', 'off', 'svc', 'offline'));

ALTER TABLE documents
  ADD COLUMN IF NOT EXISTS external_ref text,
  ADD COLUMN IF NOT EXISTS webhook_url text,
  ADD COLUMN IF NOT EXISTS webhook_secret text;

CREATE UNIQUE INDEX IF NOT EXISTS documents_tenant_ref
  ON documents(tenant_id, external_ref)
  WHERE external_ref IS NOT NULL AND external_ref <> '';

CREATE TABLE IF NOT EXISTS webhooks (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  company_id uuid,
  url text NOT NULL,
  secret text NOT NULL,
  events text[] NOT NULL DEFAULT ARRAY['document.authorized','document.rejected','document.canceled','document.validated','document.reconciliation_required'],
  active boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (tenant_id, company_id) REFERENCES companies(tenant_id, id)
);

CREATE INDEX IF NOT EXISTS webhooks_tenant_active
  ON webhooks(tenant_id)
  WHERE active;

CREATE TABLE IF NOT EXISTS webhook_deliveries (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  document_id uuid REFERENCES documents(id),
  url text NOT NULL,
  event text NOT NULL,
  status_code integer,
  response_body text,
  attempts integer NOT NULL DEFAULT 1,
  success boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS webhook_deliveries_doc
  ON webhook_deliveries(tenant_id, document_id, created_at DESC);

CREATE TABLE IF NOT EXISTS tax_profiles (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  company_id uuid NOT NULL,
  name text NOT NULL,
  description text NOT NULL DEFAULT '',
  cfop_internal text NOT NULL DEFAULT '5102',
  cfop_interstate text NOT NULL DEFAULT '6102',
  ncm_default text NOT NULL DEFAULT '00000000',
  taxes jsonb NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, company_id, name),
  FOREIGN KEY (tenant_id, company_id) REFERENCES companies(tenant_id, id)
);

GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO allspark;

