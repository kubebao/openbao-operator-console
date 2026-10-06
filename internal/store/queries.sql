-- name: UpsertConnection :exec
INSERT INTO console_connection(id, projection) VALUES ($1, $2)
ON CONFLICT(id) DO UPDATE SET projection = excluded.projection;

-- name: GetConnection :one
SELECT projection FROM console_connection WHERE id = $1;

-- name: GetConnectionDeployments :many
SELECT projection FROM console_deployment WHERE connection_id = $1 ORDER BY id;

-- name: UpsertDeployment :exec
INSERT INTO console_deployment(id, connection_id, projection) VALUES ($1, $2, $3)
ON CONFLICT(id) DO UPDATE SET projection = excluded.projection;

-- name: DeleteMissingDeployments :exec
DELETE FROM console_deployment WHERE connection_id = $1 AND NOT (id = ANY($2::text[]));

-- name: InsertObservation :exec
INSERT INTO console_observation(deployment_id, recorded_at, projection) VALUES ($1, $2, $3);

-- name: InsertActivity :exec
INSERT INTO console_activity(id, recorded_at, projection) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING;

-- name: ListConnections :many
SELECT projection FROM console_connection ORDER BY id;

-- name: ListDeployments :many
SELECT projection FROM console_deployment ORDER BY id;

-- name: ListActivity :many
SELECT projection FROM console_activity ORDER BY recorded_at DESC LIMIT 100;

-- name: GetHistory :many
SELECT recorded_at, projection FROM console_observation WHERE deployment_id = $1 ORDER BY recorded_at DESC LIMIT 100;

-- name: PruneObservations :exec
DELETE FROM console_observation WHERE recorded_at < $1;

-- name: PruneActivity :exec
DELETE FROM console_activity WHERE recorded_at < $1;

-- name: GetSettings :one
SELECT revision, ciphertext FROM console_configuration WHERE id = true;

-- name: CreateSettings :execrows
INSERT INTO console_configuration(id, revision, ciphertext) VALUES (true, 1, $1) ON CONFLICT DO NOTHING;

-- name: UpdateSettings :execrows
UPDATE console_configuration SET revision = revision + 1, ciphertext = $1 WHERE id = true AND revision = $2;

-- name: InsertSettingsChange :exec
INSERT INTO console_configuration_change(revision, actor, summary) VALUES ($1, $2, $3);

-- name: ListSettingsChanges :many
SELECT revision, actor, summary, recorded_at FROM console_configuration_change ORDER BY revision DESC LIMIT 100;

-- name: ListReaderCredentials :many
SELECT connection_id, revision, ciphertext FROM console_reader_credential ORDER BY connection_id;

-- name: CreateReaderCredential :execrows
INSERT INTO console_reader_credential(connection_id, revision, ciphertext) VALUES ($1, 1, $2) ON CONFLICT DO NOTHING;

-- name: UpdateReaderCredential :execrows
UPDATE console_reader_credential SET revision = revision + 1, ciphertext = $1 WHERE connection_id = $2 AND revision = $3;

-- name: DeleteRemovedCredentials :exec
DELETE FROM console_reader_credential WHERE NOT (connection_id = ANY($1::text[]));

-- name: LockSettingsRevision :one
SELECT revision FROM console_configuration WHERE id = true FOR SHARE;
