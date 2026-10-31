-- name: SearchInstruments :many
SELECT i.id::text AS id, i.instrument_type, i.currency, i.name FROM instrument i
WHERE i.active AND (i.name ILIKE '%' || sqlc.arg(term)::text || '%' OR EXISTS (
 SELECT 1 FROM instrument_identifier x WHERE x.instrument_id=i.id AND x.value ILIKE '%' || sqlc.arg(term)::text || '%' AND x.valid_from<=now() AND x.valid_to>now())) ORDER BY i.name,i.id LIMIT 50;
-- name: ResolveInstrument :many
SELECT i.id::text AS id,i.instrument_type,i.currency,i.name FROM instrument i JOIN instrument_identifier x ON x.instrument_id=i.id
WHERE x.scheme=sqlc.arg(scheme) AND x.value=sqlc.arg(value) AND x.venue=sqlc.arg(venue) AND x.valid_from<=now() AND x.valid_to>now() AND i.active ORDER BY i.id;
-- name: LatestQuote :one
SELECT instrument_id::text AS instrument_id,source_id,observed_at,received_at,bid::text AS bid,ask::text AS ask FROM quote WHERE instrument_id=sqlc.arg(id)::uuid ORDER BY observed_at DESC,received_at DESC,id DESC LIMIT 1;
-- name: Expirations :many
SELECT DISTINCT expiration::text AS expiration FROM option_contract WHERE underlying_id=sqlc.arg(id)::uuid ORDER BY expiration;
-- name: OptionChain :many
SELECT o.instrument_id::text AS instrument_id,o.expiration::text AS expiration,o.strike::text AS strike,o.right_type,o.exercise_style,
 coalesce(q.bid::text,'')::text AS bid,coalesce(q.ask::text,'')::text AS ask,coalesce(q.source_id,'') AS source_id
FROM option_contract o LEFT JOIN LATERAL (SELECT bid,ask,source_id FROM quote WHERE instrument_id=o.instrument_id ORDER BY observed_at DESC,received_at DESC,id DESC LIMIT 1) q ON true
WHERE o.underlying_id=sqlc.arg(id)::uuid AND o.expiration=sqlc.arg(expiration)::date ORDER BY o.strike,o.right_type;
-- name: Curves :many
SELECT id::text AS id,name,currency FROM curve WHERE currency=sqlc.arg(currency) ORDER BY name;
-- name: CurvePoints :many
SELECT p.tenor_days,p.zero_rate::text AS zero_rate,s.observed_at,s.source_id FROM curve_point p JOIN curve_snapshot s ON s.id=p.snapshot_id
WHERE s.id=(SELECT id FROM curve_snapshot WHERE curve_id=sqlc.arg(id)::uuid ORDER BY observed_at DESC,received_at DESC,id DESC LIMIT 1) ORDER BY p.tenor_days;
-- name: InsertQuote :exec
INSERT INTO quote(instrument_id,source_id,observed_at,bid,ask) VALUES(sqlc.arg(instrument_id)::uuid,sqlc.arg(source_id),sqlc.arg(observed_at)::timestamptz,sqlc.arg(bid)::numeric,sqlc.arg(ask)::numeric) ON CONFLICT(instrument_id,source_id,observed_at) DO NOTHING;
-- name: UpsertCurve :one
INSERT INTO curve(name,currency) VALUES(sqlc.arg(name),sqlc.arg(currency)) ON CONFLICT(name,currency) DO UPDATE SET name=excluded.name RETURNING id::text AS id;
-- name: InsertCurveSnapshot :many
INSERT INTO curve_snapshot(curve_id,source_id,observed_at) VALUES(sqlc.arg(curve_id)::uuid,sqlc.arg(source_id),sqlc.arg(observed_at)::timestamptz)
ON CONFLICT(curve_id,source_id,observed_at) DO NOTHING RETURNING id::text AS id;
-- name: InsertCurvePoint :exec
INSERT INTO curve_point(snapshot_id,tenor_days,zero_rate) VALUES(sqlc.arg(snapshot_id)::uuid,sqlc.arg(tenor_days),sqlc.arg(zero_rate)::numeric);
