DROP TABLE price_bar, bond_call;
ALTER TABLE equity DROP COLUMN sector;
ALTER TABLE bond DROP COLUMN issue_date, DROP COLUMN coupon_frequency, DROP COLUMN day_count, DROP COLUMN redemption;
