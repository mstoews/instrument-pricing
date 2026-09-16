CREATE TABLE instrument (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), instrument_type text NOT NULL CHECK (instrument_type IN ('equity','option','bond','index','future','fx','cash')),
 currency text NOT NULL CHECK (currency ~ '^[A-Z]{3}$'), name text NOT NULL, active boolean NOT NULL DEFAULT true,
 valid_from timestamptz NOT NULL DEFAULT '-infinity', valid_to timestamptz NOT NULL DEFAULT 'infinity',
 created_at timestamptz NOT NULL DEFAULT now(), CHECK(valid_from < valid_to));
CREATE TABLE instrument_identifier (
 instrument_id uuid NOT NULL REFERENCES instrument(id), scheme text NOT NULL, value text NOT NULL, venue text NOT NULL DEFAULT '',
 valid_from timestamptz NOT NULL DEFAULT '-infinity', valid_to timestamptz NOT NULL DEFAULT 'infinity',
 PRIMARY KEY(scheme,value,venue,valid_from), CHECK(valid_from < valid_to));
CREATE INDEX identifier_instrument_idx ON instrument_identifier(instrument_id);
CREATE TABLE equity (instrument_id uuid PRIMARY KEY REFERENCES instrument(id), exchange text NOT NULL, country text NOT NULL);
CREATE TABLE option_contract (instrument_id uuid PRIMARY KEY REFERENCES instrument(id), underlying_id uuid NOT NULL REFERENCES equity(instrument_id),
 expiration date NOT NULL, strike numeric(20,8) NOT NULL CHECK(strike>0), right_type text NOT NULL CHECK(right_type IN ('call','put')),
 exercise_style text NOT NULL CHECK(exercise_style IN ('european','american')), multiplier numeric NOT NULL DEFAULT 100 CHECK(multiplier>0),
 UNIQUE(underlying_id,expiration,strike,right_type,exercise_style));
CREATE TABLE bond (instrument_id uuid PRIMARY KEY REFERENCES instrument(id), issuer text NOT NULL, maturity date NOT NULL,
 coupon_rate numeric NOT NULL CHECK(coupon_rate>=0), face_value numeric NOT NULL CHECK(face_value>0));
CREATE TABLE data_source (id text PRIMARY KEY, name text NOT NULL, is_sample boolean NOT NULL DEFAULT false);
CREATE TABLE quote (id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, instrument_id uuid NOT NULL REFERENCES instrument(id),
 source_id text NOT NULL REFERENCES data_source(id), observed_at timestamptz NOT NULL, received_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 bid numeric(20,8) NOT NULL CHECK(bid>=0), ask numeric(20,8) NOT NULL CHECK(ask>=bid),
 UNIQUE(instrument_id,source_id,observed_at));
CREATE INDEX quote_latest_idx ON quote(instrument_id,observed_at DESC,received_at DESC,id DESC);
CREATE TABLE trade (id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, instrument_id uuid NOT NULL REFERENCES instrument(id),
 source_id text NOT NULL REFERENCES data_source(id), external_id text NOT NULL, observed_at timestamptz NOT NULL,
 received_at timestamptz NOT NULL DEFAULT clock_timestamp(), price numeric(20,8) NOT NULL CHECK(price>=0), quantity numeric NOT NULL CHECK(quantity>0), UNIQUE(source_id,external_id));
CREATE TABLE curve (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), name text NOT NULL, currency text NOT NULL CHECK(currency ~ '^[A-Z]{3}$'), UNIQUE(name,currency));
CREATE TABLE curve_snapshot (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), curve_id uuid NOT NULL REFERENCES curve(id), source_id text NOT NULL REFERENCES data_source(id),
 observed_at timestamptz NOT NULL, received_at timestamptz NOT NULL DEFAULT clock_timestamp(), UNIQUE(curve_id,source_id,observed_at));
CREATE TABLE curve_point (snapshot_id uuid NOT NULL REFERENCES curve_snapshot(id), tenor_days integer NOT NULL CHECK(tenor_days>0), zero_rate numeric NOT NULL, PRIMARY KEY(snapshot_id,tenor_days));
