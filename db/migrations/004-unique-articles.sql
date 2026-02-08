-- Add unique constraint on url per user to prevent duplicate articles
-- First, remove duplicates keeping the oldest one
DELETE FROM articles 
WHERE id NOT IN (
    SELECT MIN(id) 
    FROM articles 
    WHERE url != ''
    GROUP BY user_id, url
)
AND url != '';

-- Create unique index (allows multiple empty URLs)
CREATE UNIQUE INDEX idx_articles_user_url ON articles(user_id, url) WHERE url != '';
