-- 0007_reservations (down) — drop the reservations table and its sweep index.
DROP INDEX IF EXISTS reservations_sweep_idx;
DROP TABLE IF EXISTS reservations;
