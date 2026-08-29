ALTER TABLE sites
ADD COLUMN is_deleted BOOLEAN NOT NULL DEFAULT false;

CREATE INDEX idx_sites_user_active 
ON sites(user_id) 
WHERE is_deleted = false;