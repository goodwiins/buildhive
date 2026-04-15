-- name: UpsertBuilder :one
INSERT INTO builders (name, address, arch)
VALUES ($1, $2, $3)
ON CONFLICT (name) DO UPDATE
  SET address = EXCLUDED.address, arch = EXCLUDED.arch, last_seen_at = NOW(), status = 'healthy'
RETURNING *;

-- name: UpdateBuilderHeartbeat :exec
UPDATE builders SET last_seen_at = NOW(), status = $2 WHERE name = $1;

-- name: ListBuilders :many
SELECT * FROM builders ORDER BY last_seen_at DESC;

-- name: GetHealthyBuilders :many
SELECT * FROM builders WHERE status = 'healthy'
  AND last_seen_at > NOW() - INTERVAL '30 seconds'
ORDER BY last_seen_at DESC;
