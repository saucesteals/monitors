# Marketplace finds

Catch underpriced local listings before they disappear.

## Configure

```go
City: "austin", Radius: "25mi",
Queries: []string{"Herman Miller Aeron", "Herman Miller Mirra"},
Include: `(?i)\b(aeron|mirra)\b`,
Exclude: `(?i)\b(parts|repair|wanted|replica)\b`,
MinCents: 10000, MaxCents: 45000, Currency: "USD",
Days: 1, Pages: 2, Interval: 2 * time.Minute,
```

## How it behaves

Searches anonymous Marketplace feeds, merges overlapping queries by listing ID, and alerts once per new matching listing. Price limits use cents. Matching is title-based. `Days` limits listing age; `Pages` bounds each query's newest-result window. Radius is approximate because Facebook exposes city centers, not seller coordinates.

A first scan is silent. Listings observed outside your filters stay seen; this is a new-listing monitor, not a price-change monitor. A source warning that listings were discarded for missing locations fails the scan instead of recording a false empty baseline. IDs are retained, so checkpoint storage grows with observed listings.

## Recipes

### Camera hunt

```go
City: "chicago", Radius: "15mi",
Queries: []string{"Fujifilm X100VI", "Fujifilm X100V"},
Include: `(?i)\bx100(vi|v)\b`,
Exclude: `(?i)\b(case|strap|rental|wanted)\b`,
MinCents: 50000, MaxCents: 150000,
```

### Vintage audio

```go
Queries: []string{"Technics SL-1200", "Technics SL-1210"},
Include: `(?i)technics`, Exclude: `(?i)parts|repair`,
MinCents: 10000, MaxCents: 50000,
```

## Dependency

From the scaffold's Go module, add the Shop SDK:

```sh
go get github.com/saucesteals/shop@v0.0.0-20261002000718-1a7a960631f5
```

## Run

Set your Discord account and channel in `monitor.yaml`, then have your agent configure and run this monitor with monitord.
