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
INSERT INTO console_observation(deployment_id, connection_id, recorded_at, projection) VALUES ($1, $2, $3, $4);

-- name: ListRemovedConnections :many
SELECT id FROM console_connection WHERE NOT (id = ANY(@enrolled::text[])) ORDER BY id;

-- name: DeleteConnectionActivity :exec
DELETE FROM console_activity WHERE connection_id = ANY(@connection_ids::text[]);

-- name: DeleteConnectionObservations :exec
DELETE FROM console_observation WHERE connection_id = ANY(@connection_ids::text[]);

-- name: DeleteConnectionDeployments :exec
DELETE FROM console_deployment WHERE connection_id = ANY(@connection_ids::text[]);

-- name: DeleteConnections :exec
DELETE FROM console_connection WHERE id = ANY(@connection_ids::text[]);

-- name: InsertActivity :exec
INSERT INTO console_activity(id, deployment_id, connection_id, namespace, recorded_at, projection)
VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT DO NOTHING;

-- name: ListConnections :many
SELECT projection FROM console_connection ORDER BY id;

-- name: ListDeployments :many
SELECT projection FROM console_deployment ORDER BY id;

-- ListActivity returns one newest-first page of events in the authorized scope: element i
-- of connection_ids and namespaces is one authorized namespace. The scope applies before the
-- limit. A page after a cursor starts below its (recorded_at, id) position.
-- name: ListActivity :many
SELECT id, recorded_at, projection FROM console_activity
WHERE (connection_id, namespace) IN (
  SELECT (@connection_ids::text[])[i], (@namespaces::text[])[i]
  FROM generate_subscripts(@connection_ids::text[], 1) AS i
)
AND (sqlc.narg(after_at)::timestamptz IS NULL OR (recorded_at, id) < (sqlc.narg(after_at)::timestamptz, sqlc.narg(after_id)::text))
ORDER BY recorded_at DESC, id DESC
LIMIT @row_limit;

-- GetHistory returns one newest-first page of a deployment's observations recorded at or
-- after since, when set.
-- name: GetHistory :many
SELECT id, recorded_at, projection FROM console_observation
WHERE deployment_id = @deployment_id
AND (sqlc.narg(since)::timestamptz IS NULL OR recorded_at >= sqlc.narg(since)::timestamptz)
AND (sqlc.narg(after_at)::timestamptz IS NULL OR (recorded_at, id) < (sqlc.narg(after_at)::timestamptz, sqlc.narg(after_id)::bigint))
ORDER BY recorded_at DESC, id DESC
LIMIT @row_limit;

-- GetHistoryBaseline returns the newest observation recorded before a time window.
-- name: GetHistoryBaseline :one
SELECT id, recorded_at, projection FROM console_observation
WHERE deployment_id = @deployment_id AND recorded_at < @before
ORDER BY recorded_at DESC, id DESC
LIMIT 1;

-- Project the chart window in SQL instead of transferring every full observation.
-- name: GetCharts :many
SELECT recorded_at, jsonb_build_object(
  'health', projection->'health',
  'evidence', jsonb_build_object('collection', projection#>'{evidence,collection}'),
  'backup', CASE WHEN projection->'backup' IS NULL OR projection->'backup' = 'null'::jsonb THEN NULL
    ELSE jsonb_build_object('lastSuccessAt', projection#>'{backup,lastSuccessAt}',
      'consecutiveFailures', projection#>'{backup,consecutiveFailures}',
      'sizeBytes', projection#>'{backup,sizeBytes}') END
)::jsonb AS chart_projection
FROM console_observation
WHERE deployment_id = @deployment_id AND recorded_at >= @since AND recorded_at <= @until
ORDER BY recorded_at ASC, id ASC;

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

-- name: GetLocalCredential :one
SELECT account_id, revision, ciphertext FROM console_local_credential WHERE account_id = $1;

-- name: CreateLocalCredential :execrows
INSERT INTO console_local_credential(account_id, revision, ciphertext) VALUES ($1, 1, $2) ON CONFLICT DO NOTHING;

-- name: UpdateLocalCredential :execrows
UPDATE console_local_credential SET revision = revision + 1, ciphertext = $1 WHERE account_id = $2 AND revision = $3;

-- name: DeleteRemovedLocalCredentials :exec
DELETE FROM console_local_credential WHERE NOT (account_id = ANY($1::text[]));
