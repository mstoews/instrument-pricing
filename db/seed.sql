INSERT INTO data_source VALUES ('sample','Synthetic demonstration data',true),('massive','Massive',false),('finra','FINRA',false),('fred','FRED',false) ON CONFLICT DO NOTHING;
INSERT INTO instrument(id,instrument_type,currency,name) VALUES ('00000000-0000-0000-0000-000000000001','equity','USD','Apple Inc. (sample)') ON CONFLICT DO NOTHING;
INSERT INTO instrument_identifier(instrument_id,scheme,value,venue) VALUES ('00000000-0000-0000-0000-000000000001','ticker','AAPL','XNAS') ON CONFLICT DO NOTHING;
INSERT INTO equity VALUES ('00000000-0000-0000-0000-000000000001','XNAS','US') ON CONFLICT DO NOTHING;
INSERT INTO instrument(id,instrument_type,currency,name)
SELECT ('00000000-0000-0000-0000-'||lpad(n::text,12,'0'))::uuid,'option','USD','Sample AAPL option '||n FROM generate_series(2,7) n ON CONFLICT DO NOTHING;
INSERT INTO option_contract(instrument_id,underlying_id,expiration,strike,right_type,exercise_style)
SELECT ('00000000-0000-0000-0000-'||lpad(n::text,12,'0'))::uuid,'00000000-0000-0000-0000-000000000001','2027-01-15',190+((n-2)/2)*10,CASE WHEN n%2=0 THEN 'call' ELSE 'put' END,'european' FROM generate_series(2,7) n ON CONFLICT DO NOTHING;
INSERT INTO quote(instrument_id,source_id,observed_at,bid,ask) SELECT id,'sample','2026-09-15T12:00:00Z',CASE WHEN instrument_type='equity' THEN 199.9 ELSE 10 END,CASE WHEN instrument_type='equity' THEN 200.1 ELSE 10.5 END FROM instrument ON CONFLICT DO NOTHING;
INSERT INTO curve VALUES ('00000000-0000-0000-0000-000000000100','Sample USD zero curve','USD') ON CONFLICT DO NOTHING;
INSERT INTO curve_snapshot(id,curve_id,source_id,observed_at) VALUES ('00000000-0000-0000-0000-000000000101','00000000-0000-0000-0000-000000000100','sample','2026-09-15T12:00:00Z') ON CONFLICT DO NOTHING;
INSERT INTO curve_point VALUES ('00000000-0000-0000-0000-000000000101',30,0.04),('00000000-0000-0000-0000-000000000101',365,0.038) ON CONFLICT DO NOTHING;
