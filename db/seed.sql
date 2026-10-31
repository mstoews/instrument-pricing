INSERT INTO data_source VALUES ('sample','Synthetic demonstration data',true),('massive','Massive',false),('finra','FINRA',false),('fred','FRED',false) ON CONFLICT DO NOTHING;
INSERT INTO instrument(id,instrument_type,currency,name) VALUES ('00000000-0000-0000-0000-000000000001','equity','USD','Apple Inc. (sample)') ON CONFLICT DO NOTHING;
INSERT INTO instrument_identifier(instrument_id,scheme,value,venue) VALUES ('00000000-0000-0000-0000-000000000001','ticker','AAPL','XNAS') ON CONFLICT DO NOTHING;
INSERT INTO equity VALUES ('00000000-0000-0000-0000-000000000001','XNAS','US') ON CONFLICT DO NOTHING;
INSERT INTO instrument(id,instrument_type,currency,name)
SELECT ('00000000-0000-0000-0000-'||lpad(n::text,12,'0'))::uuid,'option','USD','Sample AAPL option '||n FROM generate_series(2,7) n ON CONFLICT DO NOTHING;
INSERT INTO option_contract(instrument_id,underlying_id,expiration,strike,right_type,exercise_style)
SELECT ('00000000-0000-0000-0000-'||lpad(n::text,12,'0'))::uuid,'00000000-0000-0000-0000-000000000001','2027-01-15',190+((n-2)/2)*10,CASE WHEN n%2=0 THEN 'call' ELSE 'put' END,'european' FROM generate_series(2,7) n ON CONFLICT DO NOTHING;
INSERT INTO quote(instrument_id,source_id,observed_at,bid,ask) SELECT id,'sample','2026-09-15T12:00:00Z',CASE WHEN instrument_type='equity' THEN 199.9 ELSE 10 END,CASE WHEN instrument_type='equity' THEN 200.1 ELSE 10.5 END FROM instrument WHERE instrument_type IN ('equity','option') ON CONFLICT DO NOTHING;
INSERT INTO curve VALUES ('00000000-0000-0000-0000-000000000100','Sample USD zero curve','USD') ON CONFLICT DO NOTHING;
INSERT INTO curve_snapshot(id,curve_id,source_id,observed_at) VALUES ('00000000-0000-0000-0000-000000000101','00000000-0000-0000-0000-000000000100','sample','2026-09-15T12:00:00Z') ON CONFLICT DO NOTHING;
INSERT INTO curve_point VALUES ('00000000-0000-0000-0000-000000000101',30,0.04),('00000000-0000-0000-0000-000000000101',365,0.038) ON CONFLICT DO NOTHING;
-- Security master and price history samples (synthetic).
INSERT INTO instrument(id,instrument_type,currency,name) VALUES ('00000000-0000-0000-0000-000000000201','index','USD','Sample US equity index'),
 ('00000000-0000-0000-0000-000000000301','bond','USD','Sample Corp 4.25% 2031 (sample)') ON CONFLICT DO NOTHING;
INSERT INTO instrument_identifier(instrument_id,scheme,value,venue) VALUES ('00000000-0000-0000-0000-000000000201','ticker','SPX-SAMPLE',''),
 ('00000000-0000-0000-0000-000000000301','sample','CORP-4.25-2031','') ON CONFLICT DO NOTHING;
INSERT INTO bond(instrument_id,issuer,issue_date,maturity,coupon_rate,coupon_frequency,day_count,face_value,redemption)
VALUES ('00000000-0000-0000-0000-000000000301','Sample Corp','2021-05-15','2031-05-15',0.0425,2,'30/360',1000,100) ON CONFLICT DO NOTHING;
INSERT INTO bond_call VALUES ('00000000-0000-0000-0000-000000000301','2029-05-15',100) ON CONFLICT DO NOTHING;
INSERT INTO price_bar(instrument_id,source_id,bar_date,open,high,low,close,volume)
SELECT id,'sample',d,round(c*0.998,4),round(c*1.006,4),round(c*0.992,4),round(c,4),v FROM (
 SELECT x.id,s.d,s.n,x.v,(x.base+x.amp*sin(s.n/20.0)+x.drift*s.n+x.wiggle*sin(s.n*1.7))::numeric AS c FROM
 (SELECT d::date AS d,row_number() OVER (ORDER BY d) AS n FROM generate_series('2025-09-15'::date,'2026-09-15'::date,'1 day') d WHERE extract(isodow FROM d)<6) s
 CROSS JOIN (VALUES ('00000000-0000-0000-0000-000000000001'::uuid,180.0,12.0,0.08,1.5,50000000),
                    ('00000000-0000-0000-0000-000000000201'::uuid,5000.0,180.0,1.2,20.0,0),
                    ('00000000-0000-0000-0000-000000000301'::uuid,98.0,0.8,0.002,0.05,2000000)) x(id,base,amp,drift,wiggle,v)
) bars ON CONFLICT DO NOTHING;
