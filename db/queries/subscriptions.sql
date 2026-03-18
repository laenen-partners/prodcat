-- name: CreateSubscription :one
INSERT INTO subscriptions (entity_id, entity_type, product_id, signing_rule, required_count)
VALUES (@entity_id, @entity_type, @product_id, @signing_rule, @required_count)
RETURNING *;

-- name: GetSubscription :one
SELECT * FROM subscriptions WHERE id = @id;

-- name: ListSubscriptions :many
SELECT * FROM subscriptions
WHERE (@entity_id::text IS NULL OR entity_id = @entity_id)
  AND (@customer_id::text IS NULL OR id IN (SELECT subscription_id FROM parties WHERE customer_id = @customer_id AND removed_at IS NULL))
  AND (@status::text IS NULL OR status = @status::subscription_status)
ORDER BY created_at DESC
LIMIT CASE WHEN @result_limit::int > 0 THEN @result_limit ELSE 100 END
OFFSET @result_offset;

-- name: ActivateSubscription :exec
UPDATE subscriptions
SET status = 'active', external_ref = @external_ref, activated_at = now()
WHERE id = @id;

-- name: CancelSubscription :exec
UPDATE subscriptions
SET status = 'canceled', canceled_at = now()
WHERE id = @id;

-- name: DisableSubscription :exec
UPDATE subscriptions
SET disabled = true, disabled_reason = @disabled_reason, disabled_message = @disabled_message, disabled_at = now()
WHERE id = @id;

-- name: EnableSubscription :exec
UPDATE subscriptions
SET disabled = false, disabled_reason = NULL, disabled_message = '', disabled_at = NULL
WHERE id = @id;

-- name: UpdateSigningAuthority :exec
UPDATE subscriptions
SET signing_rule = @signing_rule, required_count = @required_count
WHERE id = @id;
