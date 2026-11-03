# Marketplace finds

Catch underpriced local listings before they disappear.

## Configure

```go
Latitude: 30.2677, Longitude: -97.7475, Radius: "25mi",
Queries: []string{"Herman Miller Aeron", "Herman Miller Mirra"},
Include: `(?i)\b(aeron|mirra)\b`,
Exclude: `(?i)\b(parts|repair|wanted|replica)\b`,
MinCents: 10000, MaxCents: 45000, Currency: "USD",
Days: 1, Pages: 2, Interval: 2 * time.Minute,
```

## How it behaves

Searches anonymous Marketplace feeds, merges overlapping queries by listing ID, and alerts once per new matching listing. Price limits use cents. Matching is title-based. `Days` limits listing age; `Pages` bounds each query's newest-result window. Both coordinates are required. Search is stateless: no HTML bootstrap, city lookup or caches. Radius is requested from Facebook; outside-radius recommendations are not independently rejected.

A first scan is silent. Listings observed outside your filters stay seen; this is a new-listing monitor, not a price-change monitor. IDs are retained, so checkpoint storage grows with observed listings.

## Recipes

### Camera hunt

```go
Latitude: 41.8781, Longitude: -87.6298, Radius: "15mi",
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
go get github.com/saucesteals/shop@v0.0.0-20261103082107-8a23fb309faf
```

## Run

Set your Discord account and channel in `monitor.yaml`, then have your agent configure and run this monitor with monitord.
