-- ============================================================
-- TempMail v12 migration - API Key 有效期（商业版）
-- 旧账户 expires_at 保持 NULL，视为永久有效。可重复执行。
-- ============================================================

ALTER TABLE accounts
  ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ NULL;

CREATE INDEX IF NOT EXISTS idx_accounts_expires_at
  ON accounts (expires_at) WHERE expires_at IS NOT NULL;
