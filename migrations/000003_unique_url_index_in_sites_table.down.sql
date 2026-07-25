-- migrate:options transaction:false 
CREATE INDEX CONCURRENTLY idx_sites_url_old on sites(url);
DROP INDEX IF EXISTS idx_sites_url;
ALTER INDEX idx_sites_url_old RENAME TO idx_sites_url;