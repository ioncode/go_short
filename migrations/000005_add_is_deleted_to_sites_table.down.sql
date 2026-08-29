ALTER TABLE sites 
DROP COLUMN is_deleted;

DROP INDEX IF EXISTS idx_sites_user_active;