-- name: CreateCapability :one
INSERT INTO capabilities (subscription_id, capability_type, status)
VALUES (@subscription_id, @capability_type, @status)
RETURNING *;

-- name: ListCapabilities :many
SELECT * FROM capabilities WHERE subscription_id = @subscription_id ORDER BY created_at;

-- name: DisableCapability :exec
UPDATE capabilities
SET status = 'disabled', disabled = true, disabled_reason = @disabled_reason, disabled_message = @disabled_message, disabled_at = now()
WHERE id = @id;

-- name: EnableCapability :exec
UPDATE capabilities
SET status = 'active', disabled = false, disabled_reason = NULL, disabled_message = '', disabled_at = NULL
WHERE id = @id;
