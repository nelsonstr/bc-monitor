package config

import (
	"blockchain-monitor/internal/models"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

// Config holds all configuration for the application
type Config struct {
	LogLevel   string
	MaxRetries int
	RetryDelay time.Duration
	HTTP       HTTPConfig
	Kafka      KafkaConfig
	Chains     map[models.BlockchainName]ChainConfig
}

// HTTPConfig holds HTTP client configuration
type HTTPConfig struct {
	Timeout time.Duration
}

// KafkaConfig holds Kafka configuration
type KafkaConfig struct {
	BrokerAddress string
	Topic         string
	BatchSize     int
	BatchTimeout  time.Duration
}

// ChainConfig holds configuration for each blockchain
type ChainConfig struct {
	RpcEndpoint string
	ApiKey      string
	RateLimit   float64
}

// Load loads configuration from environment variables
func Load() (*Config, error) {
	if err := godotenv.Load(); err != nil {
		// Not fatal, as env vars might be set externally
	}

	config := &Config{
		LogLevel:   getEnv("LOG_LEVEL", "info"),
		MaxRetries: getEnvAsInt("MAX_RETRIES", 1),
		RetryDelay: time.Duration(getEnvAsInt("RETRY_DELAY", 5)) * time.Second,
		HTTP: HTTPConfig{
			Timeout: time.Duration(getEnvAsInt("HTTP_TIMEOUT", 30)) * time.Second,
		},
		Kafka: KafkaConfig{
			BrokerAddress: getEnv("KAFKA_BROKER_ADDRESS", "localhost:9092"),
			Topic:         getEnv("KAFKA_TOPIC", "blockchain-transactions"),
			BatchSize:     getEnvAsInt("KAFKA_BATCH_SIZE", 10),
			BatchTimeout:  time.Duration(getEnvAsInt("KAFKA_BATCH_TIMEOUT", 5)) * time.Second,
		},
		Chains: make(map[models.BlockchainName]ChainConfig),
	}

	// Load chain configurations
	config.Chains[models.Bitcoin] = ChainConfig{
		RpcEndpoint: getEnv("BITCOIN_RPC_ENDPOINT", "https://svc.blockdaemon.com/bitcoin/mainnet/native"),
		ApiKey:      getEnv("BITCOIN_API_KEY", ""),
		RateLimit:   getEnvAsFloat("BITCOIN_RATE_LIMIT", 4),
	}

	config.Chains[models.Ethereum] = ChainConfig{
		RpcEndpoint: getEnv("ETHEREUM_RPC_ENDPOINT", "https://svc.blockdaemon.com/ethereum/mainnet/native"),
		ApiKey:      getEnv("ETHEREUM_API_KEY", ""),
		RateLimit:   getEnvAsFloat("ETHEREUM_RATE_LIMIT", 4),
	}

	config.Chains[models.Solana] = ChainConfig{
		RpcEndpoint: getEnv("SOLANA_RPC_ENDPOINT", "https://svc.blockdaemon.com/solana/mainnet/native"),
		ApiKey:      getEnv("SOLANA_API_KEY", ""),
		RateLimit:   getEnvAsFloat("SOLANA_RATE_LIMIT", 4),
	}

	return config, nil
}

// getEnv gets an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvAsInt gets an environment variable as int or returns a default value
func getEnvAsInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

// getEnvAsFloat gets an environment variable as float64 or returns a default value
func getEnvAsFloat(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if floatValue, err := strconv.ParseFloat(value, 64); err == nil {
			return floatValue
		}
	}
	return defaultValue
}
