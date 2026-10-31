-- name: InsertInstrument :one
INSERT INTO instrument(instrument_type,currency,name) VALUES(sqlc.arg(instrument_type),sqlc.arg(currency),sqlc.arg(name)) RETURNING id::text AS id;
-- name: InsertIdentifier :exec
INSERT INTO instrument_identifier(instrument_id,scheme,value,venue) VALUES(sqlc.arg(instrument_id)::uuid,sqlc.arg(scheme),sqlc.arg(value),sqlc.arg(venue));
-- name: InsertEquity :exec
INSERT INTO equity(instrument_id,exchange,country,sector) VALUES(sqlc.arg(instrument_id)::uuid,sqlc.arg(exchange),sqlc.arg(country),sqlc.arg(sector));
-- name: InsertBond :exec
INSERT INTO bond(instrument_id,issuer,issue_date,maturity,coupon_rate,coupon_frequency,day_count,face_value,redemption)
VALUES(sqlc.arg(instrument_id)::uuid,sqlc.arg(issuer),sqlc.narg(issue_date)::date,sqlc.arg(maturity)::date,sqlc.arg(coupon_rate)::numeric,
 sqlc.arg(coupon_frequency)::smallint,sqlc.arg(day_count),sqlc.arg(face_value)::numeric,sqlc.arg(redemption)::numeric);
-- name: InsertBondCall :exec
INSERT INTO bond_call(instrument_id,call_date,call_price) VALUES(sqlc.arg(instrument_id)::uuid,sqlc.arg(call_date)::date,sqlc.arg(call_price)::numeric);
-- name: GetBond :one
SELECT i.id::text AS id,i.instrument_type,i.currency,i.name,b.issuer,coalesce(b.issue_date::text,'')::text AS issue_date,b.maturity::text AS maturity,
 b.coupon_rate::text AS coupon_rate,b.coupon_frequency,b.day_count,b.face_value::text AS face_value,b.redemption::text AS redemption
FROM bond b JOIN instrument i ON i.id=b.instrument_id WHERE b.instrument_id=sqlc.arg(id)::uuid;
-- name: BondCalls :many
SELECT call_date::text AS call_date,call_price::text AS call_price FROM bond_call WHERE instrument_id=sqlc.arg(id)::uuid ORDER BY call_date;
-- name: Identifiers :many
SELECT scheme,value,venue FROM instrument_identifier WHERE instrument_id=sqlc.arg(id)::uuid AND valid_from<=now() AND valid_to>now() ORDER BY scheme,value,venue;
-- name: PriceBars :many
SELECT instrument_id::text AS instrument_id,source_id,bar_date::text AS bar_date,open::text AS open,high::text AS high,low::text AS low,close::text AS close,
 coalesce(adj_close::text,'')::text AS adj_close,volume::text AS volume
FROM price_bar WHERE instrument_id=sqlc.arg(id)::uuid AND source_id=sqlc.arg(source_id) AND bar_date BETWEEN sqlc.arg(from_date)::date AND sqlc.arg(to_date)::date
ORDER BY bar_date LIMIT 20000;
-- name: LatestBarOnOrBefore :one
SELECT instrument_id::text AS instrument_id,source_id,bar_date::text AS bar_date,open::text AS open,high::text AS high,low::text AS low,close::text AS close,
 coalesce(adj_close::text,'')::text AS adj_close,volume::text AS volume
FROM price_bar WHERE instrument_id=sqlc.arg(id)::uuid AND source_id=sqlc.arg(source_id) AND bar_date<=sqlc.arg(on_date)::date ORDER BY bar_date DESC LIMIT 1;
-- name: InsertPriceBar :execrows
INSERT INTO price_bar(instrument_id,source_id,bar_date,open,high,low,close,adj_close,volume)
VALUES(sqlc.arg(instrument_id)::uuid,sqlc.arg(source_id),sqlc.arg(bar_date)::date,sqlc.arg(open)::numeric,sqlc.arg(high)::numeric,sqlc.arg(low)::numeric,
 sqlc.arg(close)::numeric,sqlc.narg(adj_close)::numeric,sqlc.arg(volume)::numeric)
ON CONFLICT(instrument_id,source_id,bar_date) DO NOTHING;
