CREATE TABLE IF NOT EXISTS console_connection (
  id text PRIMARY KEY,
  projection jsonb NOT NULL
);
CREATE TABLE IF NOT EXISTS console_deployment (
  id text PRIMARY KEY,
  connection_id text NOT NULL REFERENCES console_connection(id),
  projection jsonb NOT NULL
);
CREATE INDEX IF NOT EXISTS console_deployment_connection ON console_deployment(connection_id);
CREATE TABLE IF NOT EXISTS console_observation (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  deployment_id text NOT NULL,
  recorded_at timestamptz NOT NULL,
  projection jsonb NOT NULL
);
CREATE INDEX IF NOT EXISTS console_observation_deployment ON console_observation(deployment_id, recorded_at DESC);
CREATE INDEX IF NOT EXISTS console_observation_retention ON console_observation(recorded_at);
CREATE TABLE IF NOT EXISTS console_activity (
  id text PRIMARY KEY,
  recorded_at timestamptz NOT NULL,
  projection jsonb NOT NULL
);
CREATE INDEX IF NOT EXISTS console_activity_retention ON console_activity(recorded_at);
