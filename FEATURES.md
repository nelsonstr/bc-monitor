# Features

The bc-monitor application provides comprehensive blockchain transaction monitoring capabilities across multiple blockchains. Below is a detailed list of its key features and capabilities.

## Multi-Blockchain Support

- **Bitcoin Monitoring**: Tracks transactions on the Bitcoin network using RPC polling every minute
- **Ethereum Monitoring**: Monitors Ethereum (EVM-compatible) transactions with 10-second polling intervals
- **Polygon Monitoring**: Monitors Polygon (EVM-compatible) transactions with 10-second polling intervals
- **BSC Monitoring**: Monitors Binance Smart Chain (EVM-compatible) transactions with 10-second polling intervals
- **Solana Monitoring**: Real-time transaction tracking using WebSocket subscriptions for account changes

## Real-Time Monitoring

- **Adaptive Polling Strategies**: Different monitoring frequencies optimized for each blockchain's characteristics
- **WebSocket Integration**: Solana uses WebSocket connections for instant account change notifications
- **Continuous Block Processing**: Monitors new blocks and processes transactions involving watched addresses

## Event Emission

- **Kafka Integration**: Emits transaction events to configurable Kafka topics
- **Structured Event Data**: Includes source address, destination address, amount, fees, timestamp, and blockchain explorer URLs
- **Batch Processing**: Configurable batch sizes and timeouts for efficient event publishing

## Health Checks

- **Liveness Probe** (`/healthz`): Basic service availability check
- **Readiness Probe** (`/readyz`): Comprehensive health check including blockchain connectivity and last block status
- **Blockchain Status Tracking**: Monitors last processed block/slot for each blockchain every 10 seconds

## Configuration Management

- **Environment-Based Configuration**: All settings configurable via environment variables with sensible defaults
- **Per-Chain Configuration**: Separate RPC endpoints, API keys, and rate limits for each blockchain
- **Flexible Deployment**: Supports `.env` files and external environment variable injection

## Rate Limiting

- **Per-Chain Rate Limiting**: Configurable requests per second limits to respect API provider constraints
- **Token Bucket Algorithm**: Smooth rate limiting using Go's `golang.org/x/time/rate` package
- **Automatic Throttling**: Prevents API rate limit violations across all RPC calls

## Async Processing

- **Concurrent Monitors**: Each blockchain runs in its own goroutine for parallel processing
- **Non-Blocking Operations**: Event emission and health checks run asynchronously
- **Graceful Shutdown**: Proper context cancellation and cleanup on application termination

## Input Validation

- **Address Validation**: Chain-specific validation for Bitcoin, Ethereum, and Solana addresses
- **Transaction Hash Validation**: Format verification for transaction identifiers
- **Amount Validation**: Positive value checks with reasonable upper bounds
- **URL Validation**: Proper formatting for RPC endpoints and explorer URLs

## Retry Mechanisms

- **Configurable Retries**: Adjustable maximum retry attempts and delay intervals
- **Exponential Backoff**: Built-in retry logic with configurable delays
- **Error Handling**: Comprehensive error logging and graceful failure handling

## Logging and Observability

- **Structured Logging**: Uses Zerolog for consistent, structured log output
- **Configurable Log Levels**: Debug, info, warn, error, and fatal levels
- **Detailed Transaction Logging**: Logs all transaction details for monitoring and debugging

## Data Persistence

- **PostgreSQL Integration**: Stores monitored addresses and transaction history in a relational database
- **Address Management**: Persistent storage of wallet addresses with metadata and monitoring status
- **Transaction Archiving**: Comprehensive storage of transaction data including amounts, fees, timestamps, and blockchain details
- **Query Capabilities**: Efficient retrieval of historical transaction data for analysis and reporting

## Scalability Enhancements

- **Redis-Based Coordination**: Distributed coordination using Redis for managing multiple monitor instances
- **Instance Synchronization**: Ensures consistent monitoring across horizontally scaled deployments
- **Load Distribution**: Automatic distribution of monitoring tasks across available instances
- **Fault Tolerance**: Graceful handling of instance failures with automatic redistribution of workloads

## Resilience Features

- **Block Reorganization Handling**: Detects and handles blockchain reorganizations (orphaned blocks)
- **Downtime Recovery**: Persists last scanned block to resume monitoring after outages
- **Circuit Breaker Pattern**: Planned implementation for API failure scenarios
- **Memory Profiling**: Built-in support for performance monitoring and optimization

## Extensibility

- **Modular Architecture**: Clean interfaces for adding new blockchain monitors
- **Plugin-Based Design**: Easy to extend with new event emitters or monitoring strategies
- **Interface-Driven Development**: Well-defined contracts for blockchain monitors and event emitters

## Future Enhancements

- **Prometheus Metrics**: Comprehensive monitoring and alerting capabilities
- **Unit Test Coverage**: Extensive test suite for reliability assurance
- **Load Testing**: Performance validation under high transaction volumes

## Security

- **API Key Management**: Secure handling of blockchain provider API keys
- **Input Sanitization**: Validation prevents malformed data from causing issues
- **Error Isolation**: Failures in one blockchain monitor don't affect others

This feature set makes bc-monitor a robust, scalable solution for real-time blockchain transaction monitoring across multiple networks.
