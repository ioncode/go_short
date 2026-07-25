-- migrate:options transaction:false

CREATE UNIQUE INDEX CONCURRENTLY idx_sites_url_new on sites(url);
DROP INDEX IF EXISTS idx_sites_url;
ALTER INDEX idx_sites_url_new RENAME TO idx_sites_url;