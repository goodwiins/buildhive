-- name: CreateProject :one
INSERT INTO projects (name, slug) VALUES ($1, $2) RETURNING *;

-- name: GetProjectByID :one
SELECT * FROM projects WHERE id = $1;

-- name: GetProjectBySlug :one
SELECT * FROM projects WHERE slug = $1;

-- name: ListProjects :many
SELECT * FROM projects ORDER BY created_at DESC;

-- name: DeleteProject :exec
DELETE FROM projects WHERE id = $1;
