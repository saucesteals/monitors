# Marketplace finds

Catch underpriced local listings before they disappear.

## Configure

```go
City: "austin", Radius: "25mi",
Queries: []string{"Herman Miller Aeron", "Herman Miller Mirra"},
Include: `(?i)\b(aeron|mirra)\b`,
Exclude: `(?i)\b(parts|repair|wanted|replica)\b`,
MinCents: 10000, MaxCents: 45000, Currency: "USD",
Days: 1, Interval: 30 * time.Second,
```

## How it behaves

Searches anonymous Marketplace feeds, merges overlapping queries by listing ID, and alerts once per new matching listing. Price limits use cents. Matching is title-based. `Days` limits listing age. Each query fetches only its newest first page, with no pagination, script discovery, independent city lookups, or automatic retries. Facebook supplies the radius filter and may return out-of-radius recommendations; pickup distance is not verified. Deeper listings can be missed. Prefer a small set of distinct searches; blank and duplicate queries are rejected.

A first scan is silent. Listings observed outside your filters stay seen; this is a new-listing monitor, not a price-change monitor. Changing the search scope silently establishes a new baseline; the old checkpoint remains intact. This first-page migration also starts a new silent baseline, so existing listings are not replayed. IDs are retained, so checkpoint storage grows with observed listings.

One long-lived client caches bootstrap metadata for an hour, not listings. A cold query uses HTML plus GraphQL; warm queries use one fresh GraphQL request each. Non-rate-limit search errors invalidate metadata for the next scheduled scan without retrying. Title patterns are compiled once at startup; price and title matching remain local. Missing listing locations are shown as "Not listed".

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
go get github.com/saucesteals/shop@v0.0.0-20261102185038-100650d71faa
```

## Run

Set your Discord account and channel in `monitor.yaml`, then have your agent configure and run this monitor with monitord.
