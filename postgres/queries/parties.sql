-- name: AddParty :one
INSERT INTO parties (subscription_id, customer_id, role)
VALUES (@subscription_id, @customer_id, @role)
RETURNING *;

-- name: ListParties :many
SELECT * FROM parties WHERE subscription_id = @subscription_id AND removed_at IS NULL ORDER BY added_at;

-- name: RemoveParty :exec
UPDATE parties SET removed_at = now() WHERE id = @id;
