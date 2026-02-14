-- name: ListComponents :many
SELECT id, name, description, created_at FROM components ORDER BY name;

-- name: CreateComponent :execlastid
INSERT INTO components (name, description) VALUES (?, ?);

-- name: GetComponentByName :one
SELECT id, name, description, created_at FROM components WHERE name = ?;
