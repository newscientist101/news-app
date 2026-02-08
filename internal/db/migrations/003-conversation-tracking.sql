-- Add conversation tracking to jobs table

ALTER TABLE jobs ADD COLUMN current_conversation_id TEXT DEFAULT '';

-- Record execution of this migration
INSERT OR IGNORE INTO migrations (migration_number, migration_name)
VALUES (003, '003-conversation-tracking');
