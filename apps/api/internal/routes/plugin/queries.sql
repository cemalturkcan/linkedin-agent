-- name: Plugin :one
SELECT extension_id, version, capabilities, first_seen, last_seen, hellos
  FROM plugin WHERE id = 1;

-- name: SavePlugin :exec
INSERT INTO plugin (id, extension_id, version, capabilities, first_seen, last_seen, hellos)
VALUES (1, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET extension_id = excluded.extension_id,
  version = excluded.version, capabilities = excluded.capabilities,
  last_seen = excluded.last_seen, hellos = excluded.hellos;
