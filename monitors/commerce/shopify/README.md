# Shopify

New products, restocks, and price drops on public Shopify storefronts.

## Configure

```go
Store:         "https://kith.com",
Label:         "Kith",
Collection:    "mens-footwear",
TitleContains: "J.L-A.L",
Handles:       []string{},
Sizes:         []string{"9", "9.5"},
Events:        []string{"new", "restock", "price-drop"},
Currency:      "USD",
ImageMode:     "image",
Interval:      5 * time.Minute,
```

Change `Store` to another Shopify origin. Leave collection, title, handles, or sizes empty to widen the watch. Sizes match an exact variant title or option value. Select any combination of event types. `ImageMode` accepts `image`, `thumbnail`, or `none`; images come from the product itself.

## Recipes

### The whole sneaker collection

```go
Collection: "mens-footwear",
TitleContains: "", Handles: nil, Sizes: nil,
Events: []string{"new", "restock"},
```

### Your size, one collab

```go
Handles: []string{"pu406687-01"},
Sizes: []string{"9", "9.5", "10"},
Events: []string{"restock"},
```

### Sale price on a watchlist

```go
Handles: []string{"pu406687-01", "pu406687-02"},
Events: []string{"price-drop"},
```

## Behavior

- Reads Shopify’s public paginated product catalog, optionally scoped to a collection.
- Baselines silently, then tracks each product and its variants independently.
- New products emit once. Known variants returning to stock share a restock card.
- Price drops compare against the last observed price, not the storefront’s compare-at price.
- A complete, validated scan precedes changes; blocked or incomplete catalog responses preserve progress.

The storefront must expose its public catalog. Set the currency to that storefront’s market. Images and polling cadence can change without resetting progress; changing targets or filters starts a new silent baseline.


## Run

Set your Discord account and channel in `monitor.yaml`, then have your agent configure and run this monitor with monitord.
