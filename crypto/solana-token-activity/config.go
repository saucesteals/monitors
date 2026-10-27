package main

import (
	"crypto/sha256"
	"fmt"

	"github.com/saucesteals/monitord/catalog/quicknode/solana"
)

type config struct {
	Address solana.PublicKey // Indexed address to watch: owner or one token account.
	Owner   solana.PublicKey // Owner used to attribute token balance entries.
	Mint    solana.PublicKey
}

var settings = config{
	Address: "REPLACE_WITH_WATCHED_ADDRESS",
	Owner:   "REPLACE_WITH_TOKEN_OWNER",
	Mint:    "REPLACE_WITH_TOKEN_MINT",
}

func configuration() (string, error) {
	for label, address := range map[string]solana.PublicKey{
		"address": settings.Address,
		"owner":   settings.Owner,
		"mint":    settings.Mint,
	} {
		if err := address.Validate(); err != nil {
			return "", fmt.Errorf("configure %s: %w", label, err)
		}
	}
	identity := "solana-mainnet-token-v1:" + string(settings.Address) + ":" + string(settings.Owner) + ":" + string(settings.Mint)

	return fmt.Sprintf("%x", sha256.Sum256([]byte(identity)))[:20], nil
}
