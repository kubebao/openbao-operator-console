CREATE TABLE console_reader_credential (
 connection_id text PRIMARY KEY,
 revision bigint NOT NULL CHECK (revision > 0),
 ciphertext bytea NOT NULL
);
