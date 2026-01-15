# Blockchain Transaction Monitor


* handle edge cases like retry situations:
    * block reorganization
        * the orphans blocks must be drop as the transactions
          it should stay in persisted but not include in any balance change
        * create a monitor to identify reorgs
    * how to not lose any txs in a 1h downtime scenario
        * when the current application status is store
          on startup if must load  the address to monitor, and the last scanned block
        
    * any other scenarios you want to showcase
        * if the api start return x error we should implement a circuit breaker, it can be an api error or and application error
        * run load tests to check any memory issue or even use a profiler 


* Next steps

    * persist wallets to data
        * userID
        * wallet address
        * balance
        * block
        * parent block
        * transaction hash
    * add kafka queue to manage the user wallets to monitor
    * add prometheus metrics
    * add monitoring 
    * create unit tests



* Known issues:
    * Ethereum fees calculator not working with EIP-1559
    * blockdaemon websocket not working, we wss is using Chainstack.

# Mermaid diagram to illustrate the solution

##  Architecture Diagram

```mermaid
graph TB
    subgraph Blockchain Monitors
        SM[Solana Monitor]
        EM[Ethereum Monitor]
        BM[Bitcoin Monitor]
    end

    subgraph RPC Nodes
        SRPC[Solana RPC]
        ERPC[Ethereum RPC]
        BRPC[Bitcoin RPC]
    end

    subgraph Event Processing
        KP[Kafka Producer]
        KT[Kafka Topic]
    end

 

    SM --> SRPC
    EM --> ERPC
    BM --> BRPC

    SM --> KP
    EM --> KP
    BM --> KP

    KP --> KT
 
```

## Bitcoin Architecture Diagram

```mermaid
sequenceDiagram
    participant BM as Bitcoin Monitor
    participant RPC as Bitcoin RPC
    participant KP as Kafka Producer

    loop Every 1 minute
        BM->>RPC: Get latest block
        RPC-->>BM: Return latest block
        BM->>BM: Process transactions
        BM->>KP: Emit transaction events
    end
```

## Ethereum Architecture Diagram

```mermaid
sequenceDiagram
    participant EM as Ethereum Monitor
    participant RPC as Ethereum RPC
    participant KP as Kafka Producer

    loop Every 10 seconds
        EM->>RPC: Get latest block number
        RPC-->>EM: Return latest block number
        loop For each new block
            EM->>RPC: Get block details
            RPC-->>EM: Return block details
            EM->>EM: Process transactions
            EM->>KP: Emit transaction events
        end
    end
```
## Solan Architecture Diagram

```mermaid
sequenceDiagram
    participant SM as Solana Monitor
    participant WS as WebSocket
    participant RPC as Solana RPC
    participant KP as Kafka Producer

    SM->>WS: Subscribe to account changes
    loop Every account change
        WS->>SM: Notify account change
        SM->>RPC: Get transaction details
        RPC-->>SM: Return transaction details
        SM->>SM: Process transaction
        SM->>KP: Emit transaction event
    end
```


## interfaces

### Blockchain Monitors

```mermaid
classDiagram
    class BlockchainMonitor {
        <<interface>>
        +Start(ctx context.Context) error
        +Stop(ctx context.Context) error
        +AddAddress(address string) error
        +GetBlockHead() (uint64, error)
        +GetExplorerURL(txHash string) string
    }

 
    class SolanaMonitor {
        -wsConn *websocket.Conn
        -subscriptions map[string]int
        -balances map[string]*big.Int
        +Initialize() error
        +StartMonitoring(ctx context.Context) error
        -processAccountChange(change AccountChange)
    }

    class EthereumMonitor {
        -latestBlock uint64
        +Initialize() error
        +StartMonitoring(ctx context.Context) error
        -monitorBlocks(ctx context.Context, watchAddresses map[string]bool)
        -processBlock(blockNum uint64, watchAddresses map[string]bool) error
    }

    class BitcoinMonitor {
        -latestBlock uint64
        +Initialize() error
        +StartMonitoring(ctx context.Context) error
        -monitorBlocks(ctx context.Context)
        -processBlock(blockHash string) error
    }
 
 

    BlockchainMonitor <|.. SolanaMonitor
    BlockchainMonitor <|.. EthereumMonitor
    BlockchainMonitor <|.. BitcoinMonitor
 
```



## Microservice healthcheck endpoint

### ready

http://127.0.0.1:8888/readyz

### heath

http://127.0.0.1:8888/healthz


# developer tools

* golang
* git

## build

```shell
go build ./cmd/ -o cmd
```


## run

```shell
go run ./cmd/
```

## Address configurations

The addresses to monitor are configured at [addresses](cmd/add_addresses.go)

Example

```go
 User{
    ID: "a4b21045-ea18-42f0-bfe0-798ed7f7a6cb",
    Addresses: map[models.BlockchainName][]string{
        models.Ethereum: {"0x95222290DD7278Aa3Ddd389Cc1E1d165CC4BAfe5"},
        models.Solana: {
            "5guD4Uz462GT4Y4gEuqyGsHZ59JGxFN4a3rF6KWguMcJ",
            "oQPnhXAbLbMuKHESaGrbXT17CyvWCpLyERSJA9HCYd7"},

        models.Bitcoin: {"bc1qamgjuxaywqls56h7rg7afga3m6rgqwfkew688k",
            "bc1qxy2kgdygjrsqtzq2n0yrf2493p83kkfjhx0wlh",
            "bc1qxy2kgdygjrsqtzq2n0yrf2493p83kkfjhx0wlh"},
    },
}
```

---

[//]: # (## Mandatory task)

[//]: # (Given a list of Bitcoin, Ethereum, and Solana addresses associated to a `userId` &#40;assume 1 per chain for example&#41;)

[//]: # (create a microservice in Golang that monitors the blockchains for any transactions involving those addresses.)

[//]: # ()
[//]: # (In summary, the service should:)

[//]: # ()
[//]: # (1. Connect via RPC to the Bitcoin, Ethereum, and Solana blockchains using Blockdaemon &#40;feel free to use another provider if you prefer&#41;.)

[//]: # ()
[//]: # (2. Consume the appropriate information to detect all future transactions that involve the specified addresses.)

[//]: # ()
[//]: # (3. For the filtered transactions, process the payload and output the following information:)

[//]: # (- Source)

[//]: # (- Destination)

[//]: # (- Amount)

[//]: # (- Fees)

[//]: # ()
[//]: # (This output should be in the form of an event emitted by a Kafka producer.)

[//]: # ()
[//]: # (The service should be designed for scalability, capable of processing blocks in real time &#40;be mindful of Solana's speed!&#41;.)

[//]: # ()
[//]: # (## Bonus task)

[//]: # (Not mandatory, but appreciated:)

[//]: # ()
[//]: # (Bonus 1: Add a Mermaid diagram to illustrate your solution.)

[//]: # ()
[//]: # (Bonus 2: Explain &#40;no need to code&#41; how you would handle edge cases like retry situations, block reorganization, how to not lose any txs in a 1h downtime scenario, and any other scenarios you want to showcase.)

## RPC Docs

You can use any RPC provider you want. If you need an example or reference

[RPC solana](https://docs.blockdaemon.com/reference/how-to-access-solana-api)

[RPC ethereum](https://docs.blockdaemon.com/reference/how-to-access-ethereum-api)

[RPC bitcoin](https://docs.blockdaemon.com/reference/how-to-access-bitcoin-api)


 