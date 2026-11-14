ALTER TABLE companies ADD COLUMN IF NOT EXISTS archived_at timestamptz;
ALTER TABLE companies ADD COLUMN IF NOT EXISTS updated_at timestamptz NOT NULL DEFAULT now();
CREATE TABLE IF NOT EXISTS company_fiscal_settings (
 tenant_id uuid NOT NULL, company_id uuid NOT NULL, kind text NOT NULL CHECK(kind IN ('nfe','nfce')),
 environment text NOT NULL CHECK(environment='homologation'), series integer NOT NULL CHECK(series BETWEEN 0 AND 889),
 enabled boolean NOT NULL DEFAULT false, updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(tenant_id,company_id,kind),
 FOREIGN KEY(tenant_id,company_id) REFERENCES companies(tenant_id,id)
);
CREATE INDEX IF NOT EXISTS companies_tenant_cursor ON companies(tenant_id,id);
CREATE INDEX IF NOT EXISTS documents_tenant_cursor ON documents(tenant_id,id);
GRANT SELECT,INSERT,UPDATE,DELETE ON company_fiscal_settings TO allspark;
