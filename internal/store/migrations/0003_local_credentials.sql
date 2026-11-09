-- Local account credentials: an Argon2id password hash, a TOTP secret, and recovery
-- code digests as AES-GCM ciphertext under the configuration key. The account itself is
-- part of the configuration. Revisions provide compare-and-swap updates.
CREATE TABLE console_local_credential (
  account_id text PRIMARY KEY,
  revision bigint NOT NULL CHECK (revision > 0),
  ciphertext bytea NOT NULL
);
