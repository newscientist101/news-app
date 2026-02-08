-- Add index on articles.retrieved_at for date filtering performance

CREATE INDEX IF NOT EXISTS idx_articles_retrieved_at ON articles(retrieved_at);

-- Also add composite index for common query patterns
CREATE INDEX IF NOT EXISTS idx_articles_user_retrieved ON articles(user_id, retrieved_at DESC);

-- Record execution of this migration
INSERT OR IGNORE INTO migrations (migration_number, migration_name)
VALUES (007, '007-article-indexes');
