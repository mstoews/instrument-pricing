-- Vendor data sources referenced by the ingestion adapters. The seed adds the same
-- rows for local runs; this makes them exist in every database, e.g. on Neon.
INSERT INTO data_source(id,name,is_sample) VALUES ('massive','Massive',false),('treasury','U.S. Department of the Treasury',false)
ON CONFLICT (id) DO NOTHING;
