-- Security master: bond terms needed for analytics, call schedules, equity sector, daily price history.
-- coupon_rate and yields are decimal fractions (0.045 = 4.5%); bond prices and redemption are per 100 of face.
ALTER TABLE bond
 ADD COLUMN issue_date date,
 ADD COLUMN coupon_frequency smallint NOT NULL DEFAULT 2 CHECK (coupon_frequency IN (0,1,2,4,12)),
 ADD COLUMN day_count text NOT NULL DEFAULT '30/360' CHECK (day_count IN ('30/360','ACT/360','ACT/365F','ACT/ACT')),
 ADD COLUMN redemption numeric(20,8) NOT NULL DEFAULT 100 CHECK (redemption>0),
 ADD CHECK (issue_date IS NULL OR issue_date < maturity),
 ADD CHECK (coupon_frequency>0 OR coupon_rate=0);
CREATE TABLE bond_call (instrument_id uuid NOT NULL REFERENCES bond(instrument_id), call_date date NOT NULL,
 call_price numeric(20,8) NOT NULL CHECK(call_price>0), PRIMARY KEY(instrument_id,call_date));
ALTER TABLE equity ADD COLUMN sector text NOT NULL DEFAULT '';
-- Daily bars. Equities: OHLC in currency units; bonds: clean price per 100. adj_close is vendor-adjusted (splits/dividends) when supplied.
-- Append-only by (instrument, date, source): replay keeps the first payload, like quotes.
CREATE TABLE price_bar (instrument_id uuid NOT NULL REFERENCES instrument(id), source_id text NOT NULL REFERENCES data_source(id), bar_date date NOT NULL,
 open numeric(20,8) NOT NULL, high numeric(20,8) NOT NULL, low numeric(20,8) NOT NULL, close numeric(20,8) NOT NULL,
 adj_close numeric(20,8) CHECK (adj_close>0), volume numeric(24,4) NOT NULL DEFAULT 0 CHECK (volume>=0),
 received_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(instrument_id,source_id,bar_date),
 CHECK (low>=0 AND low<=open AND low<=close AND open<=high AND close<=high));
