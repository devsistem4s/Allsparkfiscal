-- AllSpark Fiscal — Migração 005: Habilitação de Produção e Homologação
ALTER TABLE documents DROP CONSTRAINT IF EXISTS documents_environment_check;
ALTER TABLE documents ADD CONSTRAINT documents_environment_check CHECK (environment IN ('homologation', 'production'));

ALTER TABLE company_fiscal_settings DROP CONSTRAINT IF EXISTS company_fiscal_settings_environment_check;
ALTER TABLE company_fiscal_settings ADD CONSTRAINT company_fiscal_settings_environment_check CHECK (environment IN ('homologation', 'production'));
