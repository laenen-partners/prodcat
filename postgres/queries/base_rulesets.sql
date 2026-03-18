-- name: CreateBaseRuleset :one
INSERT INTO base_rulesets (id, name, description, content, version)
VALUES (@id, @name, @description, @content, @version)
RETURNING *;

-- name: GetBaseRuleset :one
SELECT * FROM base_rulesets WHERE id = @id;

-- name: ListBaseRulesets :many
SELECT * FROM base_rulesets ORDER BY name;

-- name: UpdateBaseRuleset :one
UPDATE base_rulesets
SET name = @name,
    description = @description,
    content = @content,
    version = @version,
    updated_at = now()
WHERE id = @id
RETURNING *;
