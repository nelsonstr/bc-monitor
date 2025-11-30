package database

import (
	"blockchain-monitor/internal/models"
	"database/sql"
	"time"
)

// User represents a user in the database
type User struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// WatchedAddress represents a watched address in the database
type WatchedAddress struct {
	ID         string                `json:"id"`
	UserID     string                `json:"user_id"`
	Address    string                `json:"address"`
	Blockchain models.BlockchainName `json:"blockchain"`
	CreatedAt  time.Time             `json:"created_at"`
}

// Transaction represents a transaction in the database
type Transaction struct {
	ID          string                `json:"id"`
	TxHash      string                `json:"tx_hash"`
	Blockchain  models.BlockchainName `json:"blockchain"`
	FromAddress sql.NullString        `json:"from_address"`
	ToAddress   sql.NullString        `json:"to_address"`
	Amount      sql.NullString        `json:"amount"`
	Fees        sql.NullString        `json:"fees"`
	Timestamp   time.Time             `json:"timestamp"`
	ExplorerURL sql.NullString        `json:"explorer_url"`
	CreatedAt   time.Time             `json:"created_at"`
}

// CreateUser creates a new user
func CreateUser() (*User, error) {
	var user User
	err := DB.QueryRow(`
		INSERT INTO users DEFAULT VALUES
		RETURNING id, created_at, updated_at
	`).Scan(&user.ID, &user.CreatedAt, &user.UpdatedAt)
	return &user, err
}

// GetUserByID retrieves a user by ID
func GetUserByID(id string) (*User, error) {
	var user User
	err := DB.QueryRow(`
		SELECT id, created_at, updated_at FROM users WHERE id = $1
	`, id).Scan(&user.ID, &user.CreatedAt, &user.UpdatedAt)
	return &user, err
}

// AddWatchedAddress adds a watched address for a user
func AddWatchedAddress(userID, address string, blockchain models.BlockchainName) (*WatchedAddress, error) {
	var wa WatchedAddress
	err := DB.QueryRow(`
		INSERT INTO watched_addresses (user_id, address, blockchain)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id, address, blockchain) DO NOTHING
		RETURNING id, user_id, address, blockchain, created_at
	`, userID, address, blockchain).Scan(&wa.ID, &wa.UserID, &wa.Address, &wa.Blockchain, &wa.CreatedAt)
	return &wa, err
}

// GetWatchedAddresses retrieves watched addresses for a user
func GetWatchedAddresses(userID string) ([]WatchedAddress, error) {
	rows, err := DB.Query(`
		SELECT id, user_id, address, blockchain, created_at
		FROM watched_addresses
		WHERE user_id = $1
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var addresses []WatchedAddress
	for rows.Next() {
		var wa WatchedAddress
		err := rows.Scan(&wa.ID, &wa.UserID, &wa.Address, &wa.Blockchain, &wa.CreatedAt)
		if err != nil {
			return nil, err
		}
		addresses = append(addresses, wa)
	}
	return addresses, rows.Err()
}

// SaveTransaction saves a transaction to the database
func SaveTransaction(event models.TransactionEvent) error {
	_, err := DB.Exec(`
		INSERT INTO transactions (tx_hash, blockchain, from_address, to_address, amount, fees, timestamp, explorer_url)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (tx_hash, blockchain) DO NOTHING
	`, event.TxHash, event.Chain, event.From, event.To, event.Amount, event.Fees, event.Timestamp, event.ExplorerURL)
	return err
}

// GetTransactions retrieves transactions for a user (based on watched addresses)
func GetTransactions(userID string, limit, offset int) ([]Transaction, error) {
	rows, err := DB.Query(`
		SELECT t.id, t.tx_hash, t.blockchain, t.from_address, t.to_address, t.amount, t.fees, t.timestamp, t.explorer_url, t.created_at
		FROM transactions t
		INNER JOIN watched_addresses wa ON (t.from_address = wa.address OR t.to_address = wa.address) AND t.blockchain = wa.blockchain::text
		WHERE wa.user_id = $1
		ORDER BY t.timestamp DESC
		LIMIT $2 OFFSET $3
	`, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var transactions []Transaction
	for rows.Next() {
		var tx Transaction
		err := rows.Scan(&tx.ID, &tx.TxHash, &tx.Blockchain, &tx.FromAddress, &tx.ToAddress, &tx.Amount, &tx.Fees, &tx.Timestamp, &tx.ExplorerURL, &tx.CreatedAt)
		if err != nil {
			return nil, err
		}
		transactions = append(transactions, tx)
	}
	return transactions, rows.Err()
}
