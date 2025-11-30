package validation

import (
	"errors"
	"math/big"
	"regexp"
	"strings"
)

// ValidateAddress validates a blockchain address format
func ValidateAddress(address string, chain string) error {
	if address == "" {
		return errors.New("address cannot be empty")
	}

	// Basic length check
	if len(address) < 10 || len(address) > 100 {
		return errors.New("address length is invalid")
	}

	// Chain-specific validation
	switch strings.ToLower(chain) {
	case "bitcoin":
		return validateBitcoinAddress(address)
	case "ethereum":
		return validateEthereumAddress(address)
	case "solana":
		return validateSolanaAddress(address)
	default:
		// Generic validation for unknown chains
		if !regexp.MustCompile(`^[a-zA-Z0-9]{10,100}$`).MatchString(address) {
			return errors.New("address contains invalid characters")
		}
	}

	return nil
}

// validateBitcoinAddress validates Bitcoin address format
func validateBitcoinAddress(address string) error {
	// Basic Bitcoin address regex (simplified)
	bitcoinRegex := regexp.MustCompile(`^[13][a-km-zA-HJ-NP-Z1-9]{25,34}$|^bc1[a-z0-9]{39,59}$`)
	if !bitcoinRegex.MatchString(address) {
		return errors.New("invalid Bitcoin address format")
	}
	return nil
}

// validateEthereumAddress validates Ethereum address format
func validateEthereumAddress(address string) error {
	ethereumRegex := regexp.MustCompile(`^0x[a-fA-F0-9]{40}$`)
	if !ethereumRegex.MatchString(address) {
		return errors.New("invalid Ethereum address format")
	}
	return nil
}

// validateSolanaAddress validates Solana address format (base58)
func validateSolanaAddress(address string) error {
	solanaRegex := regexp.MustCompile(`^[1-9A-HJ-NP-Za-km-z]{32,44}$`)
	if !solanaRegex.MatchString(address) {
		return errors.New("invalid Solana address format")
	}
	return nil
}

// ValidateAmount validates amount is positive and within reasonable bounds
func ValidateAmount(amount *big.Float) error {
	if amount == nil {
		return errors.New("amount cannot be nil")
	}

	if amount.Sign() <= 0 {
		return errors.New("amount must be positive")
	}

	// Check for reasonable upper bound (e.g., 1 billion)
	maxAmount := big.NewFloat(1e9)
	if amount.Cmp(maxAmount) > 0 {
		return errors.New("amount exceeds maximum allowed value")
	}

	return nil
}

// ValidateTxHash validates transaction hash format
func ValidateTxHash(txHash string, chain string) error {
	if txHash == "" {
		return errors.New("transaction hash cannot be empty")
	}

	switch strings.ToLower(chain) {
	case "bitcoin":
		if len(txHash) != 64 || !regexp.MustCompile(`^[a-fA-F0-9]{64}$`).MatchString(txHash) {
			return errors.New("invalid Bitcoin transaction hash")
		}
	case "ethereum":
		if len(txHash) != 66 || !regexp.MustCompile(`^0x[a-fA-F0-9]{64}$`).MatchString(txHash) {
			return errors.New("invalid Ethereum transaction hash")
		}
	case "solana":
		if len(txHash) != 88 || !regexp.MustCompile(`^[1-9A-HJ-NP-Za-km-z]{88}$`).MatchString(txHash) {
			return errors.New("invalid Solana transaction hash")
		}
	default:
		if len(txHash) < 32 || len(txHash) > 100 {
			return errors.New("transaction hash length is invalid")
		}
	}

	return nil
}

// ValidateURL validates URL format
func ValidateURL(url string) error {
	if url == "" {
		return errors.New("URL cannot be empty")
	}

	urlRegex := regexp.MustCompile(`^https?://[^\s/$.?#].[^\s]*$`)
	if !urlRegex.MatchString(url) {
		return errors.New("invalid URL format")
	}

	return nil
}
