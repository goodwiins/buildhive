-- name: CreateBuild :one
INSERT INTO builds (project_id, builder_id, status) VALUES ($1, $2, 'pending') RETURNING *;

-- name: UpdateBuildStatus :exec
UPDATE builds SET status = $2, finished_at = $3, cache_hit = $4, image_ref = $5 WHERE id = $1;

-- name: StartBuild :exec
UPDATE builds SET status = 'running', started_at = NOW() WHERE id = $1;

-- name: ListBuildsByProject :many
SELECT * FROM builds WHERE project_id = $1 ORDER BY created_at DESC LIMIT 50;

-- name: GetBuild :one
SELECT * FROM builds WHERE id = $1;

-- name: InsertBuildLog :exec
INSERT INTO build_logs (build_id, line) VALUES ($1, $2);

-- name: GetBuildLogs :many
SELECT * FROM build_logs WHERE build_id = $1 ORDER BY id ASC;
