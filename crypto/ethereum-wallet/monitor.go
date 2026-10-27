package main

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/saucesteals/monitord"
	"github.com/saucesteals/monitord/catalog/quicknode/evm"
)

// State retains the activation boundary and recent announced blocks for corrections.
type State struct {
	Scope         string              `json:"scope"`
	BaselineBlock uint64              `json:"baseline_block"`
	Recent        map[evm.Hash]uint64 `json:"recent"`
}

type monitor struct{}

func main() {
	monitord.Run(monitor{})
}

func (monitor) Info() monitord.Info {
	return monitord.Info{
		Name:        "ethereum-wallet",
		Description: "Direct Ethereum wallet transactions with durable replay and reorg corrections",
	}
}

func (monitor) Plan() monitord.Plan[State] {
	address, scope, err := configuration()
	if err != nil {
		return monitord.Continuous(func(context.Context, *monitord.Session[State]) error { return err })
	}
	source := evm.Blocks[State]{
		Name:              "ethereum-wallet-" + scope,
		ExpectedChainID:   "0x1",
		Confirmations:     confirmations,
		MaxRecentBlocks:   256,
		ReconcileInterval: 12 * time.Second,
		FetchConcurrency:  4,
		HTTPSecret:        monitord.RequiredSecret("quicknode", "ethereum-mainnet-http-url"),
		WSSSecret:         monitord.RequiredSecret("quicknode", "ethereum-mainnet-websocket-url"),
		Handle: func(ctx context.Context, client *evm.Client, state State, block evm.Block) (evm.BlockUpdate[State], error) {
			return handle(ctx, client, state, block, address, scope)
		},
	}

	return source.Plan()
}

func handle(ctx context.Context, client *evm.Client, state State, block evm.Block, address evm.Address, scope string) (evm.BlockUpdate[State], error) {
	// The managed source starts at the current head. Suppress that whole block,
	// including any replacement at the same height, as the activation baseline.
	if state.Scope != scope {
		return func(tx *monitord.Tx[State]) error {
			if tx.State.Scope != scope {
				*tx.State = State{
					Scope:         scope,
					BaselineBlock: block.Number,
					Recent:        make(map[evm.Hash]uint64),
				}
			}

			return nil
		}, nil
	}
	if block.Removed {
		return func(tx *monitord.Tx[State]) error {
			if _, announced := tx.State.Recent[block.Hash]; !announced {
				return nil
			}
			delete(tx.State.Recent, block.Hash)

			return tx.Emit(monitord.Event{
				ID:    scope + ":" + block.ID() + ":removed",
				Title: "Ethereum wallet activity removed by reorganization",
				Body:  fmt.Sprintf("Previously announced wallet activity in block %d (%s) is no longer canonical. It may reappear in a different block.", block.Number, block.Hash),
			})
		}, nil
	}
	var lines []string
	count := 0
	if block.Number > state.BaselineBlock {
		for _, transaction := range block.Transactions {
			incoming := transaction.To != nil && *transaction.To == address
			outgoing := transaction.From == address
			if !incoming && !outgoing {
				continue
			}
			count++
			// One bounded notification per block. Omitted transactions remain
			// discoverable via the block link; do not fetch irrelevant receipts.
			if len(lines) == 12 {
				continue
			}
			receipt, err := client.ReceiptFor(ctx, transaction)
			if err != nil {
				return nil, err
			}
			value, err := transaction.Value.BigInt()
			if err != nil || value.Sign() < 0 {
				return nil, fmt.Errorf("invalid transaction value for %s", transaction.Hash)
			}
			direction := "incoming"
			if outgoing {
				direction = "outgoing"
			}
			if outgoing && incoming {
				direction = "self"
			}
			status := "succeeded"
			if !receipt.Success {
				status = "FAILED; value not transferred"
			}
			lines = append(lines, fmt.Sprintf("%s · %s · requested value %s ETH · <https://etherscan.io/tx/%s>", direction, status, ether(value), transaction.Hash))
		}
	}

	return func(tx *monitord.Tx[State]) error {
		if tx.State.Scope != scope {
			return fmt.Errorf("wallet scope changed during block update")
		}
		for hash, number := range tx.State.Recent {
			if block.Number > confirmations && number < block.Number-confirmations {
				delete(tx.State.Recent, hash)
			}
		}
		if count == 0 || block.Number <= tx.State.BaselineBlock {
			return nil
		}
		if _, announced := tx.State.Recent[block.Hash]; announced {
			return nil
		}
		if tx.State.Recent == nil {
			tx.State.Recent = make(map[evm.Hash]uint64)
		}
		tx.State.Recent[block.Hash] = block.Number
		body := fmt.Sprintf("Wallet: %s\n%d direct transaction(s) in block %d; showing %d.\n%s\nObserved at the chain head, NOT finalized. Values exclude fees and internal calls; activity does not establish a purchase or sale.", address, count, block.Number, len(lines), strings.Join(lines, "\n"))

		return tx.Emit(monitord.Event{
			ID:    scope + ":" + block.ID(),
			Title: "Ethereum wallet · direct transaction activity",
			Body:  body,
			URL:   "https://etherscan.io/block/" + string(block.Hash),
		})
	}, nil
}

func ether(value *big.Int) string {
	divisor := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	amount := new(big.Rat).SetFrac(value, divisor).FloatString(18)

	return strings.TrimRight(strings.TrimRight(amount, "0"), ".")
}
