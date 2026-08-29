ALTER TABLE sites 
DROP COLUMN user_id;

DROP INDEX IF EXISTS idx_user_id;