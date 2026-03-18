-- name: CreateArchetype :one
INSERT INTO archetypes (id, family_id, name, description, ruleset, base_ruleset_ids)
VALUES (@id, @family_id, @name, @description, @ruleset, @base_ruleset_ids)
RETURNING *;

-- name: GetArchetype :one
SELECT * FROM archetypes WHERE id = @id;

-- name: ListArchetypes :many
SELECT * FROM archetypes WHERE family_id = @family_id ORDER BY created_at;

-- name: UpdateArchetype :one
UPDATE archetypes
SET name = @name,
    description = @description,
    ruleset = @ruleset,
    base_ruleset_ids = @base_ruleset_ids,
    updated_at = now()
WHERE id = @id
RETURNING *;
