ALTER TABLE sites
ADD user_id uuid; 

CREATE INDEX idx_user_id on sites(user_id);