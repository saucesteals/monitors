# Amazon offers

Watch the offer you actually want—not just an “in stock” badge.

## Configure

```go
ASINs: []string{"B0F3GWXLTS"},
SellerID: "ATVPDKIKX0DER",
ShipsFrom: []string{"Amazon", "Amazon.com"},
MaxCents: 49900, Currency: "USD", Condition: "",
ShopConfig: "", MaxPages: 5, Interval: 2 * time.Minute,
```

## How it behaves

Scans paginated Amazon offers through Shop. Seller IDs are exact; shipper names are case-insensitive exact matches. The default watches Nintendo Switch 2 offers sold and shipped by Amazon at $499 or less.

Each ASIN baselines silently. Alerts when a qualifying offer returns or its lowest qualifying price drops. Unavailable and over-budget observations rearm the edge. Errors and incomplete pagination preserve the prior checkpoint. Product photography is fetched before the event transaction.

`Condition: ""` accepts unclassified offers; Amazon sometimes omits condition even for its own offer. Set `"new"`, `"used_like_new"`, `"used_good"`, `"used_fair"`, or `"refurbished"` to require an explicit source classification. Unknown conditions then do not match.

Uses the normal Shop configuration by default. If Amazon requests authentication, run `shop login amazon`; for a separate profile, use `shop login amazon --config PATH` and set `ShopConfig` to that path. This monitor only reads products and offers.

## Recipes

### Any seller, shipped by Amazon

```go
SellerID: "",
ShipsFrom: []string{"Amazon", "Amazon.com"},
```

### Used camera deal

```go
ASINs: []string{"REPLACE_ASIN"},
SellerID: "", ShipsFrom: nil,
Condition: "used_like_new", MaxCents: 120000,
```

### Several drops

Add ASINs to `ASINs`; each keeps an independent availability and price checkpoint. `MaxCents: 0` removes the price ceiling.

## Dependency

From the scaffold's Go module, add the Shop SDK:

```sh
go get github.com/saucesteals/shop@v0.0.0-20261002000718-1a7a960631f5
```

## Run

Set your Discord account and channel in `monitor.yaml`, then have your agent configure and run this monitor with monitord.
