-- Observation tables hold fleet projections as JSON. The fleet package defines their format.
CREATE TABLE console_connection (
  id text PRIMARY KEY,
  projection jsonb NOT NULL
);

CREATE TABLE console_deployment (
  id text PRIMARY KEY,
  connection_id text NOT NULL REFERENCES console_connection(id),
  projection jsonb NOT NULL
);
CREATE INDEX console_deployment_connection ON console_deployment(connection_id);

CREATE TABLE console_observation (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  deployment_id text NOT NULL,
  connection_id text NOT NULL,
  recorded_at timestamptz NOT NULL,
  projection jsonb NOT NULL
);
CREATE INDEX console_observation_deployment ON console_observation(deployment_id, recorded_at DESC, id DESC);
CREATE INDEX console_observation_connection ON console_observation(connection_id);
CREATE INDEX console_observation_retention ON console_observation(recorded_at);

-- Activity keeps the scope of its deployment, so authorization applies before a limit and
-- events remain visible after their deployment is deleted.
CREATE TABLE console_activity (
  id text PRIMARY KEY,
  deployment_id text NOT NULL,
  connection_id text NOT NULL,
  namespace text NOT NULL,
  recorded_at timestamptz NOT NULL,
  projection jsonb NOT NULL
);
CREATE INDEX console_activity_scope ON console_activity(connection_id, namespace, recorded_at DESC, id DESC);
CREATE INDEX console_activity_retention ON console_activity(recorded_at);

-- Configuration and reader credentials are AES-GCM ciphertext. Revisions provide
-- compare-and-swap updates.
CREATE TABLE console_configuration (
  id boolean PRIMARY KEY DEFAULT true CHECK (id),
  revision bigint NOT NULL CHECK (revision > 0),
  ciphertext bytea NOT NULL
);

CREATE TABLE console_configuration_change (
  revision bigint PRIMARY KEY,
  actor text NOT NULL,
  summary text NOT NULL,
  recorded_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE console_reader_credential (
  connection_id text PRIMARY KEY,
  revision bigint NOT NULL CHECK (revision > 0),
  ciphertext bytea NOT NULL
);

-- Local account credentials: an Argon2id password hash, a TOTP secret, and recovery
-- code digests as AES-GCM ciphertext under the configuration key. The account itself is
-- part of the configuration. Revisions provide compare-and-swap updates.
CREATE TABLE console_local_credential (
  account_id text PRIMARY KEY,
  revision bigint NOT NULL CHECK (revision > 0),
  ciphertext bytea NOT NULL
);
