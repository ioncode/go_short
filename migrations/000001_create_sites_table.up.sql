CREATE TABLE sites (
    id SERIAL PRIMARY KEY,
    url VARCHAR(255) NOT NULL,
    short_url VARCHAR(255) NOT NULL
);

CREATE INDEX idx_sites_url on sites(url); 
CREATE INDEX idx_sites_short_url on sites(short_url); 