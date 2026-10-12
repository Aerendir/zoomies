-- A person can hide a repair from Eli's Recent repairs without deleting it.
--
-- The row is also the ledger behind the daily budget, the deduplication of
-- webhook deliveries and the guard that stops Eli repairing its own repair
-- commit. Deleting it would reset the spend cap and re-open a repair loop, so
-- clearing only records when the history stopped showing it. Only the history
-- queries and the clear read or write this column.
--
-- NULL means shown, which is what every existing row keeps on upgrade.
ALTER TABLE eli_repairs ADD COLUMN cleared_at INTEGER;
