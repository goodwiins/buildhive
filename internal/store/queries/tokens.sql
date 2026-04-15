-- name: CreateToken :one
INSERT INTO api_tokens (project_id, token_hash, label) VALUES ($1, $2, $3) RETURNING *;

-- name: ListTokensByProject :many
SELECT * FROM api_tokens WHERE project_id = $1 ORDER BY created_at DESC;

-- name: GetTokenByHash :one
SELECT * FROM api_tokens WHERE token_hash = $1;

-- name: UpdateTokenLastUsed :exec
UPDATE api_tokens SET last_used_at = NOW() WHERE id = $1;

-- name: DeleteToken :exec
DELETE FROM api_tokens WHERE id = $1;
