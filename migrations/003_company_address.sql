-- Existing companies are preserved; complete their address before new submissions.
ALTER TABLE companies ADD COLUMN IF NOT EXISTS address jsonb;
