DELETE FROM data_source WHERE id='treasury' AND NOT EXISTS (SELECT 1 FROM curve_snapshot WHERE source_id='treasury');
