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
