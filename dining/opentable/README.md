# OpenTable openings

Catch dinner times appearing or reopening within your window, for your party size.

## Configure

Edit `config.go` with your restaurant's numeric ID and `/r/SLUG` profile slug. Find the ID in the restaurant page's reservation link (`rid` or `restref`) or its `RestaurantsAvailability` request in browser developer tools. Use the restaurant's IANA timezone, not your computer's timezone.

```go
RestaurantID: 0, // Replace with the real ID; zero deliberately fails validation.
Slug: "example-bistro", Label: "Example Bistro",
Timezone: "America/New_York",
Dates: []string{"2026-12-12", "2026-12-13"},
PartySize: 2, StartTime: "18:00", EndTime: "20:30",
Interval: 5 * time.Minute,
```

Example Bistro is fictional. Replace the venue and dates before running; no real restaurant is watched by default.

## How it behaves

- Checks each requested date separately, with an inclusive restaurant-local time window. Supports 1–7 dates and parties of 1–20; a venue may impose tighter limits.
- The first successful observation per date is silent. Later newly available times emit one grouped alert per date. A time that disappears and returns alerts again; unchanged availability does not.
- Uses ordinary **Standard** reservation slots, grouped by time. Experiences, private dining, seat types, and table attributes are not monitored separately. A table-type change at the same time does not create another alert.
- Cookies and CSRF tokens are obtained from the public profile page, retained only in memory, and refreshed once on a 401/403. No account, copied cookies, or stored tokens are needed.
- A complete empty slot list rearms openings. HTTP failures, GraphQL errors, missing dates/restaurants, and malformed responses preserve the last observation. Stable event IDs and atomic event/checkpoint writes prevent duplicate delivery on retries.
- Past times are skipped. Changing the restaurant, timezone, party size, or time window creates a new silent baseline. Adding a date does not reset other dates.

This adapter targets `www.opentable.com` and its `NA` database region. It uses the website's undocumented persisted GraphQL query, which can change or be blocked. An unavailable query is an error, never evidence that a restaurant is full. The adapter does not solve challenges, make reservations, or hold tables. Open the alert link and confirm the current times on OpenTable.

## Recipes

### A single seating

```go
Dates: []string{"2026-12-12"},
PartySize: 4, StartTime: "19:00", EndTime: "19:00",
```

### Early dinner

```go
StartTime: "17:00", EndTime: "18:30",
```

Windows cannot cross midnight; use separate configurations for each local date/time window.

## Run

Set your Discord account and channel placeholders in `monitor.yaml`, then have your agent configure and run this monitor with monitord. It uses the SDK's `catalog/httpx` client and `github.com/saucesteals/fhttp`, like the collection's other browser-compatible adapters. Run `monitord test opentable` before deployment; it sends no notifications. The 90-day TTL in `monitor.yaml` should be adjusted to cover your chosen dates.
