package main

import (
	"blockchain-monitor/internal/interfaces"
	"blockchain-monitor/internal/logger"
	"blockchain-monitor/internal/models"
)

func addAddressesToMonitor(monitors map[models.BlockchainName]interfaces.BlockchainMonitor) {

	user := models.User{
		ID: "a4b21045-ea18-42f0-bfe0-798ed7f7a6cb",
		Addresses: map[models.BlockchainName][]string{
			models.Ethereum: {"0x95222290DD7278Aa3Ddd389Cc1E1d165CC4BAfe5"},
			models.Solana: {
				"5guD4Uz462GT4Y4gEuqyGsHZ59JGxFN4a3rF6KWguMcJ",
				"oQPnhXAbLbMuKHESaGrbXT17CyvWCpLyERSJA9HCYd7"},

			models.Bitcoin: {
				"bc1qryhgpmfv03qjhhp2dj8nw8g4ewg08jzmgy3cyx",
				"bc1qamgjuxaywqls56h7rg7afga3m6rgqwfkew688k",
				"bc1qxy2kgdygjrsqtzq2n0yrf2493p83kkfjhx0wlh",
				"bc1qgdjqv0av3q56jvd82tkdjpy7gdp9ut8tlqmgrpmv24sq90ecnvqqjwvw97",
				"bc1qxvay4an52gcghxq5lavact7r6qe9l4laedsazz8fj2ee2cy47tlqff4aj4",
				"34xp4vRoCGJym3xR7yCVPFHoCNxv4Twseo"},
		},
	}

	for bc, add := range user.Addresses {
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
