-- name: UpsertConnection :exec
INSERT INTO console_connection(id, projection) VALUES (?1, ?2)
ON CONFLICT(id) DO UPDATE SET projection = excluded.projection;

-- name: GetConnection :one
SELECT projection FROM console_connection WHERE id = ?1;

-- name: GetConnectionDeployments :many
SELECT projection FROM console_deployment WHERE connection_id = ?1 ORDER BY id;

-- name: UpsertDeployment :exec
INSERT INTO console_deployment(id, connection_id, projection) VALUES (?1, ?2, ?3)
ON CONFLICT(id) DO UPDATE SET projection = excluded.projection;

-- name: DeleteMissingDeployments :exec
DELETE FROM console_deployment WHERE connection_id = ?1 AND id NOT IN (SELECT value FROM json_each(CAST(?2 AS TEXT)));

-- name: InsertObservation :exec
INSERT INTO console_observation(deployment_id, connection_id, recorded_at, projection) VALUES (?1, ?2, ?3, ?4);

-- name: ListRemovedConnections :many
SELECT id FROM console_connection WHERE id NOT IN (SELECT value FROM json_each(CAST(@enrolled AS TEXT))) ORDER BY id;

-- name: DeleteConnectionActivity :exec
DELETE FROM console_activity WHERE connection_id IN (SELECT value FROM json_each(CAST(@connection_ids AS TEXT)));

-- name: DeleteConnectionObservations :exec
DELETE FROM console_observation WHERE connection_id IN (SELECT value FROM json_each(CAST(@connection_ids AS TEXT)));

-- name: DeleteConnectionDeployments :exec
DELETE FROM console_deployment WHERE connection_id IN (SELECT value FROM json_each(CAST(@connection_ids AS TEXT)));

-- name: DeleteConnections :exec
DELETE FROM console_connection WHERE id IN (SELECT value FROM json_each(CAST(@connection_ids AS TEXT)));

-- name: InsertActivity :exec
INSERT INTO console_activity(id, deployment_id, connection_id, namespace, recorded_at, projection)
VALUES (?1, ?2, ?3, ?4, ?5, ?6) ON CONFLICT DO NOTHING;

-- name: ListConnections :many
SELECT projection FROM console_connection ORDER BY id;

-- name: ListDeployments :many
SELECT projection FROM console_deployment ORDER BY id;

-- ListActivity applies authorized connection/namespace pairs before the page limit.
-- The JSON array contains objects with connection and namespace fields.
-- name: ListActivity :many
SELECT id, recorded_at, projection FROM console_activity
WHERE (connection_id, namespace) IN (
  SELECT json_extract(value, '$.connection'), json_extract(value, '$.namespace')
  FROM json_each(CAST(@scopes AS TEXT))
)
AND (CAST(sqlc.narg(after_at) AS INTEGER) IS NULL OR (recorded_at, id) < (CAST(sqlc.narg(after_at) AS INTEGER), CAST(sqlc.narg(after_id) AS TEXT)))
ORDER BY recorded_at DESC, id DESC
LIMIT @row_limit;

-- GetHistory returns one newest-first page of a deployment's observations recorded at or
-- after since, when set.
-- name: GetHistory :many
SELECT id, recorded_at, projection FROM console_observation
WHERE deployment_id = @deployment_id
AND (CAST(sqlc.narg(since) AS INTEGER) IS NULL OR recorded_at >= CAST(sqlc.narg(since) AS INTEGER))
AND (CAST(sqlc.narg(after_at) AS INTEGER) IS NULL OR (recorded_at, id) < (CAST(sqlc.narg(after_at) AS INTEGER), CAST(sqlc.narg(after_id) AS INTEGER)))
ORDER BY recorded_at DESC, id DESC
LIMIT @row_limit;

-- GetHistoryBaseline returns the newest observation recorded before a time window.
-- name: GetHistoryBaseline :one
SELECT id, recorded_at, projection FROM console_observation
WHERE deployment_id = @deployment_id AND recorded_at < @before
ORDER BY recorded_at DESC, id DESC
LIMIT 1;

-- name: PruneObservations :exec
DELETE FROM console_observation WHERE recorded_at < ?1;

-- name: PruneActivity :exec
DELETE FROM console_activity WHERE recorded_at < ?1;

-- name: GetSettings :one
SELECT revision, ciphertext FROM console_configuration WHERE id = true;

-- name: CreateSettings :execrows
INSERT INTO console_configuration(id, revision, ciphertext) VALUES (true, 1, ?1) ON CONFLICT DO NOTHING;

-- name: UpdateSettings :execrows
UPDATE console_configuration SET revision = revision + 1, ciphertext = ?1 WHERE id = true AND revision = ?2;

-- name: InsertSettingsChange :exec
INSERT INTO console_configuration_change(revision, actor, summary) VALUES (?1, ?2, ?3);

-- name: ListSettingsChanges :many
SELECT revision, actor, summary, recorded_at FROM console_configuration_change ORDER BY revision DESC LIMIT 100;

-- name: ListReaderCredentials :many
SELECT connection_id, revision, ciphertext FROM console_reader_credential ORDER BY connection_id;

-- name: CreateReaderCredential :execrows
INSERT INTO console_reader_credential(connection_id, revision, ciphertext) VALUES (?1, 1, ?2) ON CONFLICT DO NOTHING;

-- name: UpdateReaderCredential :execrows
UPDATE console_reader_credential SET revision = revision + 1, ciphertext = ?1 WHERE connection_id = ?2 AND revision = ?3;

-- name: DeleteRemovedCredentials :exec
DELETE FROM console_reader_credential WHERE connection_id NOT IN (SELECT value FROM json_each(CAST(?1 AS TEXT)));

-- name: LockSettingsRevision :one
SELECT revision FROM console_configuration WHERE id = true;

-- name: GetLocalCredential :one
SELECT account_id, revision, ciphertext FROM console_local_credential WHERE account_id = ?1;

-- name: CreateLocalCredential :execrows
INSERT INTO console_local_credential(account_id, revision, ciphertext) VALUES (?1, 1, ?2) ON CONFLICT DO NOTHING;

-- name: UpdateLocalCredential :execrows
UPDATE console_local_credential SET revision = revision + 1, ciphertext = ?1 WHERE account_id = ?2 AND revision = ?3;

-- name: DeleteRemovedLocalCredentials :exec
DELETE FROM console_local_credential WHERE account_id NOT IN (SELECT value FROM json_each(CAST(?1 AS TEXT)));
