-- name: CreateFamily :one
INSERT INTO families (id, family, name, description, ruleset, base_ruleset_ids)
VALUES (@id, @family, @name, @description, @ruleset, @base_ruleset_ids)
RETURNING *;

-- name: GetFamily :one
SELECT * FROM families WHERE id = @id;

-- name: ListFamilies :many
SELECT * FROM families ORDER BY created_at;

-- name: UpdateFamily :one
UPDATE families
SET name = @name,
    description = @description,
    ruleset = @ruleset,
    base_ruleset_ids = @base_ruleset_ids,
    updated_at = now()
WHERE id = @id
RETURNING *;
