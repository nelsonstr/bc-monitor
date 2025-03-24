package main

import (
	"blockchain-monitor/internal/interfaces"
	"blockchain-monitor/internal/logger"
	"blockchain-monitor/internal/models"
)

func addAddressesToMonitor(monitors map[models.BlockchainName]interfaces.BlockchainMonitor) {

	user := models.User{
		Id: "a4b21045-ea18-42f0-bfe0-798ed7f7a6cb",
		Addresess: map[models.BlockchainName][]string{
			models.Ethereum: {"0x95222290DD7278Aa3Ddd389Cc1E1d165CC4BAfe5" /*"0x00000000219ab540356cBB839Cbe05303d7705Fa", "0x060822eA7575AC6A063349C7a226005283556639", "0x7a250d5630B4cF539739dF2C5dAcb4c659F2488D"*/},
			models.Bitcoin:  {"bc1qamgjuxaywqls56h7rg7afga3m6rgqwfkew688k", "bc1qxy2kgdygjrsqtzq2n0yrf2493p83kkfjhx0wlh", "bc1qxy2kgdygjrsqtzq2n0yrf2493p83kkfjhx0wlh"},
			models.Solana:   {"oQPnhXAbLbMuKHESaGrbXT17CyvWCpLyERSJA9HCYd7" /*, "oQPnhXAbLbMuKHESaGrbXT17CyvWCpLyERSJA9HCYd7"*/},
		},
	}
	for bc, add := range user.Addresess {
		for _, addr := range add {
			if monitors[bc] == nil {
				continue
			}
			if err := monitors[bc].AddAddress(addr); err != nil {
				logger.GetLogger().Error().
					Err(err).
					Str("chain", bc.String()).
					Str("address", addr).
					Msg("Error adding address to monitor")
			}
		}
	}

}
