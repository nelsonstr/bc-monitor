-- Create blockchain_states table
CREATE TABLE IF NOT EXISTS blockchain_states (
    chain_name TEXT PRIMARY KEY,
    last_block_height BIGINT NOT NULL DEFAULT 0,
    last_block_hash TEXT,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Trigger to update updated_at
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';

CREATE TRIGGER update_blockchain_states_updated_at
    BEFORE UPDATE ON blockchain_states
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
