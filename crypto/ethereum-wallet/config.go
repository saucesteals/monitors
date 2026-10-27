package main

import (
	"crypto/sha256"
	"fmt"

	"github.com/saucesteals/monitord/catalog/quicknode/evm"
)

// This example intentionally targets Ethereum mainnet, not arbitrary EVM chains.
var wallet = "REPLACE_WITH_ETHEREUM_ADDRESS"

const confirmations = 64

func configuration() (evm.Address, string, error) {
	address, err := evm.ParseAddress(wallet)
	if err != nil {
		return "", "", fmt.Errorf("configure wallet: %w", err)
	}
	scope := fmt.Sprintf("%x", sha256.Sum256([]byte("ethereum-direct-v1:"+string(address))))[:20]

	return address, scope, nil
}
