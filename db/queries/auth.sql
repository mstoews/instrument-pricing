-- name: UpsertUser :one
INSERT INTO app_user(email,password_hash,role) VALUES(sqlc.arg(email),sqlc.arg(password_hash),sqlc.arg(role))
ON CONFLICT(email) DO UPDATE SET password_hash=excluded.password_hash,role=excluded.role,disabled=false RETURNING id::text AS id;
-- name: InsertUser :one
INSERT INTO app_user(email,password_hash,role) VALUES(sqlc.arg(email),sqlc.arg(password_hash),sqlc.arg(role)) RETURNING id::text AS id;
-- name: UserByEmail :one
SELECT id::text AS id,email,password_hash,role,disabled FROM app_user WHERE email=sqlc.arg(email);
-- name: InsertSession :exec
INSERT INTO auth_session(user_id,family_id,access_hash,refresh_hash,access_expires_at,refresh_expires_at,user_agent)
VALUES(sqlc.arg(user_id)::uuid,sqlc.arg(family_id)::uuid,sqlc.arg(access_hash),sqlc.arg(refresh_hash),sqlc.arg(access_expires_at),sqlc.arg(refresh_expires_at),sqlc.arg(user_agent));
-- name: SessionByAccess :one
SELECT s.id::text AS session_id,s.family_id::text AS family_id,u.id::text AS user_id,u.email,u.role FROM auth_session s JOIN app_user u ON u.id=s.user_id
WHERE s.access_hash=sqlc.arg(access_hash) AND s.access_expires_at>now() AND s.rotated_at IS NULL AND s.revoked_at IS NULL AND NOT u.disabled;
-- name: SessionByRefreshForUpdate :one
SELECT s.id::text AS session_id,s.family_id::text AS family_id,s.user_id::text AS user_id,s.user_agent,
 (s.rotated_at IS NOT NULL OR s.revoked_at IS NOT NULL)::boolean AS used,(s.refresh_expires_at<=now())::boolean AS expired,u.disabled
FROM auth_session s JOIN app_user u ON u.id=s.user_id WHERE s.refresh_hash=sqlc.arg(refresh_hash) FOR UPDATE OF s;
-- name: RotateSession :exec
UPDATE auth_session SET rotated_at=now() WHERE id=sqlc.arg(id)::uuid;
-- name: RevokeFamily :exec
UPDATE auth_session SET revoked_at=now() WHERE family_id=sqlc.arg(family_id)::uuid AND revoked_at IS NULL;
-- name: UserByID :one
SELECT id::text AS id,email,role,disabled FROM app_user WHERE id=sqlc.arg(id)::uuid;
-- name: InsertAPIKey :one
INSERT INTO api_key(user_id,name,scope,key_hash) VALUES(sqlc.arg(user_id)::uuid,sqlc.arg(name),sqlc.arg(scope),sqlc.arg(key_hash))
RETURNING id::text AS id,created_at;
-- name: APIKeyByHash :one
SELECT k.id::text AS id,k.scope,k.last_used_at,u.id::text AS user_id,u.email FROM api_key k JOIN app_user u ON u.id=k.user_id
WHERE k.key_hash=sqlc.arg(key_hash) AND k.revoked_at IS NULL AND NOT u.disabled;
-- name: TouchAPIKey :exec
UPDATE api_key SET last_used_at=now() WHERE id=sqlc.arg(id)::uuid;
-- name: ListAPIKeys :many
SELECT k.id::text AS id,k.name,k.scope,u.email AS owner,k.created_at,k.last_used_at,k.revoked_at FROM api_key k JOIN app_user u ON u.id=k.user_id ORDER BY k.created_at,k.id;
-- name: RevokeAPIKey :execrows
UPDATE api_key SET revoked_at=now() WHERE id=sqlc.arg(id)::uuid AND revoked_at IS NULL;
