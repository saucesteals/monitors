# OpenTable openings

Catch dinner times appearing or reopening within your window, for your party size.

## Configure

Edit `config.go` with your restaurant's numeric ID and canonical profile URL (both `/r/NAME` and older `/NAME` URLs are supported). Find the ID in the restaurant page's reservation link (`rid` or `restref`) or its `RestaurantsAvailability` request in browser developer tools. Use the restaurant's IANA timezone, not your computer's timezone.

```go
RestaurantID: 0, // Replace with the real ID; zero deliberately fails validation.
ProfileURL: "https://www.opentable.com/r/example-bistro",
Label: "Example Bistro",
Timezone: "America/New_York",
Dates: []string{"2026-12-12", "2026-12-13"},
PartySize: 2, StartTime: "18:00", EndTime: "20:30",
Interval: 5 * time.Minute, UseProxy: true,
```

Example Bistro is fictional. Replace the venue and dates before running; no real restaurant is watched by default.

## Proxies and sessions

Proxy mode is enabled by default. Supply the exact monitord secret `proxies/opentable` as a JSON array of `http`, `https`, or `socks5` proxy URLs. For example, configure this entry in the private `secrets/proxies.env` file under your monitord root:

```dotenv
opentable=["http://USERNAME:PASSWORD@proxy.example:8000"]
```

That URL is a placeholder. Keep real credentials out of source control and restrict the secret file to its owner (`chmod 600`). Use fixed-exit proxies or provider-issued sticky sessions: a provider that changes IP behind a single URL on every request cannot preserve an OpenTable session.

Each proxy has its own Mimic transport, native cookie jar, and CSRF token. The monitor uses the same Mimic dependency as monitord, explicitly configured for Chrome 147. The SDK’s current `httpx` helper pins Chrome 131 and exposes no fingerprint option; controlled comparisons rejected that profile and accepted Chrome 147. The profile and availability requests stay on that proxy, and a working session is reused across polls. Startup chooses a random pool entry; failures move through the pool. Transport failures, missing bootstrap tokens, HTTP 401/403/429, and server errors trigger bounded recovery with fresh cookies, CSRF, and transport. Each date gets at most three attempts (two with a single proxy), with a short delay between attempts. Other errors fail the check immediately. HTTP 409 reports that the persisted query may need updating instead of cycling through proxies.

There is **no direct fallback**. Set `UseProxy: false` explicitly if you want direct access; the proxy secret is then unnecessary. Missing or invalid proxy configuration fails startup. Errors never include proxy credentials or response bodies.

## How it behaves

- Checks each requested date separately, with an inclusive restaurant-local time window. Supports 1–7 dates and parties of 1–20; a venue may impose tighter limits.
- The first successful observation per date is silent. Later newly available times emit one grouped alert per date. A time that disappears and returns alerts again; unchanged availability does not.
- Uses ordinary **Standard** reservation slots, grouped by time. Experiences, private dining, seat types, and table attributes are not monitored separately. A table-type change at the same time does not create another alert.
- Cookies and CSRF tokens are obtained from the public profile page and retained only in memory. No account, copied browser cookies, or stored tokens are needed.
- A complete empty slot list rearms openings. HTTP failures, GraphQL errors, missing dates/restaurants, and malformed responses preserve the last observation. Stable event IDs and atomic event/checkpoint writes prevent duplicate delivery on retries.
- Past times are skipped. Changing the restaurant, timezone, party size, or time window creates a new silent baseline. Adding a date does not reset other dates.

This adapter targets `www.opentable.com` and its `NA` database region. It uses the website's undocumented persisted GraphQL query, which can change or be blocked. An unavailable query is an error, never evidence that a restaurant is full. Unavailable slot placeholders may omit time/type; available slots must include them. Proxy access is not a guarantee against site challenges. The current query/schema were checked against live responses, and three consecutive standalone worker checks passed in explicit direct mode. Proxies can still be independently rejected: sampled routes returned HTTP 403 with both the original client and this adapter. Validate your own proxy route before deployment; a correct browser fingerprint does not guarantee an accepted exit IP. The adapter does not solve challenges, make reservations, or hold tables. Open the alert link and confirm the current times on OpenTable.

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

Set your Discord account and channel placeholders in `monitor.yaml`, then have your agent configure and run this monitor with monitord. It uses `github.com/saucesteals/fhttp` and `github.com/saucesteals/mimic`, which are already dependencies of monitord. No browser process, copied browser cookies, or separate scraping service is required. Run `monitord test opentable` before deployment; it sends no notifications. The 90-day TTL in `monitor.yaml` should be adjusted to cover your chosen dates.
