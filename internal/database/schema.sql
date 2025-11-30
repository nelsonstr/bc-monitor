-- Database schema for blockchain monitor

-- Users table
CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Watched addresses table
CREATE TABLE IF NOT EXISTS watched_addresses (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    address VARCHAR(255) NOT NULL,
    blockchain VARCHAR(50) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(user_id, address, blockchain)
);

-- Transaction history table
CREATE TABLE IF NOT EXISTS transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tx_hash VARCHAR(255) NOT NULL,
    blockchain VARCHAR(50) NOT NULL,
    from_address VARCHAR(255),
    to_address VARCHAR(255),
    amount VARCHAR(255),
    fees VARCHAR(255),
    timestamp TIMESTAMP WITH TIME ZONE NOT NULL,
    explorer_url TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(tx_hash, blockchain)
);

-- Indexes for performance
CREATE INDEX IF NOT EXISTS idx_watched_addresses_user_id ON watched_addresses(user_id);
CREATE INDEX IF NOT EXISTS idx_watched_addresses_address ON watched_addresses(address);
CREATE INDEX IF NOT EXISTS idx_transactions_tx_hash ON transactions(tx_hash);
CREATE INDEX IF NOT EXISTS idx_transactions_blockchain ON transactions(blockchain);
CREATE INDEX IF NOT EXISTS idx_transactions_timestamp ON transactions(timestamp);