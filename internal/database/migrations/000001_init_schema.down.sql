DROP INDEX IF EXISTS idx_transactions_timestamp;
DROP INDEX IF EXISTS idx_transactions_blockchain;
DROP INDEX IF EXISTS idx_transactions_tx_hash;
DROP INDEX IF EXISTS idx_watched_addresses_address;
DROP INDEX IF EXISTS idx_watched_addresses_user_id;

DROP TABLE IF EXISTS transactions;
DROP TABLE IF EXISTS watched_addresses;
DROP TABLE IF EXISTS users;
