



1. How are the RPC endpoints for each blockchain (Bitcoin, Ethereum, Solana) configured? Are they stored in environment variables or a configuration file? 

- RPC endpoints: These are likely configured as properties of each monitor (e.g., RpcEndpoint in the EthereumMonitor struct).

2. Is there a mechanism in place to handle rate limiting or connection issues with the RPC providers?
- Rate limiting: Basic retry mechanisms are implemented (e.g., MaxRetries and RetryDelay in the monitor structs).

3. How is the Kafka producer configured and initialized? Are there any specific settings for ensuring message delivery and handling potential network issues?
- Kafka configuration: An EventEmitter interface is used, likely implemented as a Kafka producer in the emitters directory.

4. Is there a graceful shutdown mechanism implemented to ensure that no transactions are missed when the service is stopped?
- Watched addresses: These are passed to the monitors during initialization (e.g., Addresses field in monitor structs).

5. How are the watched addresses managed? Is there a way to dynamically add or remove addresses without restarting the service?
- Block reorganization: Not explicitly handled in the visible code, but could be implemented in the main monitoring loops.

7. Is there a strategy in place for handling potential blockchain reorganizations, especially for Bitcoin and Ethereum?
- Logging: Basic logging is implemented using the standard log package.

7. How is the service handling potential out-of-memory issues, particularly when processing large blocks or during high transaction volumes?
- Blockchain differences: Separate monitor implementations for each blockchain (Ethereum, Solana, and Bitcoin) handle their specific transaction formats.

8. Is there a logging strategy implemented? How are errors and important events logged and potentially alerted on?
- Error handling: Basic error logging is implemented, with some retry logic for failed operations.

9. Are there any unit tests or integration tests implemented for the monitors and the Kafka producer?
- Scalability: The monitors are designed to process blocks continuously, with separate goroutines for each blockchain.

10. How is the service handling potential differences in transaction formats or peculiarities between the three blockchains?
- Transaction processing: Each monitor implements its own processTransaction method to extract relevant information.



to do

1.
Graceful shutdown mechanism
2.
Dynamic address management
3.
Comprehensive error alerting system
4.
Detailed memory management strategies
5.
Unit or integration tests
6.
Detailed configuration management (e.g., for RPC endpoints, Kafka settings)

```shell
go mod tidy
```

```shell
go run ./...  
```

```shell
go get -u ./...
```



https://app.blockdaemon.com/noelllabs-WKOyk-a88X/api-suite/dashboard/overview


## ethereum
```shell

curl --location --request POST 'https://svc.blockdaemon.com/ethereum/sepolia/native' \
--header 'Authorization: Bearer <YOUR_API_KEY>' \
--header 'Content-Type: application/json' \
--data-raw '{"jsonrpc": "2.0", "method": "eth_blockNumber", "params": [], "id": 83}'


curl --location --request POST 'https://svc.blockdaemon.com/optimism/mainnet/native/http-rpc' \
--header 'Authorization: Bearer <YOUR_API_KEY>' \
--header 'Content-Type: application/json' \
--data-raw '{"jsonrpc": "2.0", "id": 1, "method": "eth_blockNumber", "params": []}'
copy_all
```

## Solana

```shell
curl --location --request POST 'https://svc.blockdaemon.com/solana/mainnet/native' \
--header 'Authorization: Bearer <YOUR_API_KEY>' \
--header 'Content-Type: application/json' \
--data-raw '{"method":"getBlockHeight","id":1,"jsonrpc":"2.0"}'
```

## BTC

```shell
curl --location --request POST 'https://svc.blockdaemon.com/bitcoin/mainnet/native' \
--header 'Authorization: Bearer <YOUR_API_KEY>' \
--header 'Content-Type: application/json' \
--data-raw '{"jsonrpc": "2.0", "id": "curltest", "method": "getbestblockhash", "params": []}'
```