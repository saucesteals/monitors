package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/saucesteals/monitord"
	"github.com/saucesteals/monitord/catalog/quicknode/solana"
)

// State guards against silently changing attribution on an existing cursor.
type State struct {
	Scope string `json:"scope"`
}

type tokenBalance struct {
	Owner  string `json:"owner"`
	Mint   string `json:"mint"`
	Amount struct {
		Raw      string `json:"amount"`
		Decimals *int   `json:"decimals"`
	} `json:"uiTokenAmount"`
}

type transactionRecord struct {
	Meta *struct {
		Err  json.RawMessage `json:"err"`
		Pre  []tokenBalance  `json:"preTokenBalances"`
		Post []tokenBalance  `json:"postTokenBalances"`
	} `json:"meta"`
}

type monitor struct{}

func main() {
	monitord.Run(monitor{})
}

func (monitor) Info() monitord.Info {
	return monitord.Info{
		Name:        "solana-token-activity",
		Description: "Finalized owner-attributed token changes in transactions touching one Solana address",
	}
}

func (monitor) Plan() monitord.Plan[State] {
	scope, err := configuration()
	if err != nil {
		return monitord.Continuous(func(context.Context, *monitord.Session[State]) error { return err })
	}
	source := solana.AddressEvents[State]{
		Name:                "solana-token-activity",
		Address:             settings.Address,
		ExpectedGenesisHash: solana.MainnetGenesisHash,
		LiveCommitment:      solana.Finalized,
		PollInterval:        10 * time.Second,
		HTTPSecret:          monitord.RequiredSecret("quicknode", "solana-mainnet-http-url"),
		WSSSecret:           monitord.RequiredSecret("quicknode", "solana-mainnet-websocket-url"),
		Handle: func(_ context.Context, _ *solana.Client, event solana.AddressEvent) (solana.AddressEventUpdate[State], error) {
			return handle(event, scope)
		},
	}

	return source.Plan()
}

func handle(event solana.AddressEvent, scope string) (solana.AddressEventUpdate[State], error) {
	var record transactionRecord
	if err := json.Unmarshal(event.Transaction, &record); err != nil {
		return nil, fmt.Errorf("decode transaction: %w", err)
	}
	if record.Meta == nil || len(record.Meta.Err) == 0 {
		return nil, fmt.Errorf("transaction metadata or outcome missing")
	}
	var notification *monitord.Event
	if strings.TrimSpace(string(record.Meta.Err)) == "null" {
		if record.Meta.Pre == nil || record.Meta.Post == nil {
			return nil, fmt.Errorf("transaction omitted token balance arrays")
		}
		decimals := -1
		before, err := balanceTotal(record.Meta.Pre, &decimals)
		if err != nil {
			return nil, err
		}
		after, err := balanceTotal(record.Meta.Post, &decimals)
		if err != nil {
			return nil, err
		}
		change := new(big.Int).Sub(after, before)
		if change.Sign() != 0 {
			direction := "inflow"
			if change.Sign() < 0 {
				direction = "outflow"
			}
			notification = &monitord.Event{
				ID:    scope + ":" + event.ID(),
				Title: "Solana token · " + direction,
				Body: fmt.Sprintf("Owner: %s\nMint: %s\nNet change: %s tokens\nOwner-attributed balances in accounts touched by this transaction: %s → %s\nFinalized slot: %d\nThis is a transaction-local net change, not the complete wallet balance or proof of a purchase/sale.",
					settings.Owner, settings.Mint, units(change, decimals), units(before, decimals), units(after, decimals), event.Slot),
				URL: "https://solscan.io/tx/" + string(event.Signature),
			}
		}
	}

	return func(tx *monitord.Tx[State]) error {
		if tx.State.Scope != "" && tx.State.Scope != scope {
			return fmt.Errorf("token watch configuration changed: use a new deployment for a new address, owner or mint")
		}
		tx.State.Scope = scope
		if notification != nil {
			return tx.Emit(*notification)
		}

		return nil
	}, nil
}

func balanceTotal(balances []tokenBalance, decimals *int) (*big.Int, error) {
	total := new(big.Int)
	for _, balance := range balances {
		if balance.Mint != string(settings.Mint) {
			continue
		}
		if balance.Owner == "" {
			return nil, fmt.Errorf("selected mint balance omitted owner attribution")
		}
		if balance.Owner != string(settings.Owner) {
			continue
		}
		if balance.Amount.Decimals == nil || *balance.Amount.Decimals < 0 || *balance.Amount.Decimals > 255 {
			return nil, fmt.Errorf("selected mint balance has invalid decimals")
		}
		if *decimals >= 0 && *decimals != *balance.Amount.Decimals {
			return nil, fmt.Errorf("selected mint decimals changed within transaction")
		}
		*decimals = *balance.Amount.Decimals
		if balance.Amount.Raw == "" || strings.IndexFunc(balance.Amount.Raw, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
			return nil, fmt.Errorf("selected mint balance has invalid raw amount")
		}
		amount, ok := new(big.Int).SetString(balance.Amount.Raw, 10)
		if !ok {
			return nil, fmt.Errorf("selected mint balance has invalid raw amount")
		}
		total.Add(total, amount)
	}

	return total, nil
}

func units(value *big.Int, decimals int) string {
	divisor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	amount := new(big.Rat).SetFrac(value, divisor).FloatString(decimals)
	if decimals > 0 {
		amount = strings.TrimRight(strings.TrimRight(amount, "0"), ".")
	}

	return amount
}
