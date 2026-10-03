# Premium award seats

Find bookable-looking premium inventory with enough seats for your party.

## Configure

```go
Origin: "JFK", Destination: "LHR",
Dates: []string{"2026-12-08", "2026-12-09"},
Passengers: 2, MaxPoints: 85000, MaxStops: 0,
Cabins: []string{"BUSINESS", "FIRST"},
IgnoreSeatCounts: []int{}, UseProxy: false,
Interval: 5 * time.Minute,
```

## How it behaves

Queries Alaska's Atmos award search directly, one route/date at a time. Every segment must meet the cabin filter, so an economy long-haul with a premium connection does not qualify. Filters reported seats, points, and stops.

Each date starts silently. Alerts for new qualifying itineraries, returning inventory, lower points/cash, or more seats. Stable identity uses flight numbers, departure, and cabin—not changing price. A failed response preserves that date's checkpoint; successful dates can progress independently. Past dates are skipped.

`IgnoreSeatCounts` optionally suppresses known source-specific phantom inventory counts; no value is assumed globally. Results are search observations, not held seats. Direct requests use the SDK's browser-compatible HTTP client. Set `UseProxy: true` to use the declared `proxies/award-seats` JSON-array secret. `monitor.yaml` expires after 90 days.

## Recipes

### One stop, business only

```go
MaxStops: 1, Cabins: []string{"BUSINESS"},
Passengers: 1, MaxPoints: 70000,
```

### First-class space

```go
Cabins: []string{"FIRST"},
MaxPoints: 150000,
```

Change the IATA airports and dates in `config.go`; the adapter supports any route returned by Alaska's award search.

## Run

Set your Discord account and channel in `monitor.yaml`, then have your agent configure and run this monitor with monitord.
