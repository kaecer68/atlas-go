-- 000029_mutation_briefs_unique_window_id.down.sql
--
-- Drops only the unique index added by the up migration. The rows are left
-- untouched (a down migration must never lose data), and the pre-000015
-- non-unique index idx_mutation_briefs_window_id was never removed by the up
-- migration, so the table keeps its lookup path.
DROP INDEX IF EXISTS ux_mutation_briefs_window_id;
