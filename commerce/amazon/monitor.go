package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/saucesteals/monitord"
	"github.com/saucesteals/shop"
	_ "github.com/saucesteals/shop/provider/amazon"
	"time"
)

type State struct{}
type Snapshot struct {
	Eligible bool
	Cents    int64
	Sequence uint64
}

type monitor struct{ store shop.Store }

func main() {
	monitord.Run(&monitor{})
}

func (*monitor) Info() monitord.Info {
	return monitord.Info{Name: "amazon", Description: "Qualifying Amazon offers return or get cheaper"}
}

func (m *monitor) Plan() monitord.Plan[State] {
	return monitord.Every(settings.Interval, m.check, monitord.WithTimeout(2*time.Minute))
}

func (m *monitor) Start(ctx context.Context, _ monitord.Environment) error {
	if err := validate(); err != nil {
		return err
	}
	client, err := shop.New(shop.Options{ConfigDir: settings.ShopConfig})
	if err != nil {
		return err
	}
	m.store, err = client.Store(ctx, "amazon")
	return err
}

func (m *monitor) check(ctx context.Context, s *monitord.Session[State]) error {
	raw, _ := json.Marshal(sourceIdentity())
	scope := fmt.Sprintf("%x", sha256.Sum256(raw))[:20]
	for _, asin := range settings.ASINs {
		offer, err := m.findOffer(ctx, asin)
		if err != nil {
			return err
		}
		key := "offer:" + scope + ":" + asin
		var old Snapshot
		found, err := s.Checkpoint(key, &old)
		if err != nil {
			return err
		}
		next := Snapshot{Sequence: old.Sequence}
		if offer != nil {
			next.Eligible = true
			next.Cents = offer.Price.Amount
		}
		notify := found && next.Eligible && (!old.Eligible || next.Cents < old.Cents)
		var event monitord.Event
		if notify {
			// Product enrichment is outside the transaction; failed enrichment retries the edge.
			product, err := m.store.Product(ctx, asin)
			if err != nil {
				return err
			}
			if product == nil || product.Title == "" {
				return fmt.Errorf("missing product metadata")
			}
			kind := "Offer available"
			if old.Eligible {
				kind = "Price dropped"
			}
			next.Sequence++
			event = offerEvent(*product, *offer, kind)
			event.ID = fmt.Sprintf("amazon:%s:%s:%d", scope, asin, next.Sequence)
		}
		if err := s.Commit(ctx, func(tx *monitord.Tx[State]) error {
			if notify {
				if err := tx.Emit(event); err != nil {
					return err
				}
			}
			return tx.Checkpoint(key, next)
		}); err != nil {
			return err
		}
	}
	return nil
}
