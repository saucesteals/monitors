# OpenTable openings

Watch standard reservation times for one restaurant, with independent dates and party sizes. No account, booking, or table holds.

## Configure

Edit `config.go`: replace the fictional restaurant and zero ID, supply its canonical profile URL and IANA timezone, then choose dates, party sizes, and an inclusive local time window. Find the numeric ID in the page's reservation link (`rid` or `restref`). Windows cannot cross midnight.

```go
RestaurantID: 0, // Required: replace with a real restaurant ID.
ProfileURL:   "https://www.opentable.com/r/example-bistro",
Label:        "Example Bistro",
Timezone:     "America/New_York",
Dates:        []string{"2026-12-12"},
PartySizes:   []int{2, 4},
StartTime:    "18:00",
EndTime:      "20:30",
Interval:     time.Minute,
```

The example intentionally fails validation until configured. Each party size is queried separately; availability for two people does not imply availability for four. Supports 1–7 dates and unique party sizes from 1–20. Polls do not overlap; the interval is a target, not a latency guarantee.

Some profiles do not supply an anonymous CSRF token. If needed, set `BootstrapURL` to another canonical OpenTable profile that does. It only initializes the anonymous session; requests and booking links still target `RestaurantID` and `ProfileURL`. There is no hard-coded fallback restaurant.

## Proxy sessions

Supply `proxies/opentable` through monitord's secret resolver as a nonempty JSON array of HTTP, HTTPS, or SOCKS5 proxy URLs. Use the SDK's `catalog/httpx`; no direct-network fallback or copied browser cookies.

One selected exit owns the connection pool, cookies, and CSRF token across polls. A transport failure refreshes that entire session on the same exit once; repeated transport failures or HTTP 401/403/5xx rotate to another exit. Recovery has at most three attempts, including when only one proxy is configured. Each attempt has an eight-second timeout.

HTTP 429 respects `Retry-After` (one minute when absent) without rotating. HTTP 409 reports a potentially stale persisted query. Schema/GraphQL failures do not rotate. Redirected endpoints are rejected after the SDK request; configure canonical URLs rather than relying on redirect cookies. A proxy or browser profile does not guarantee access.

## Alerts and state

- First successful read for each date/party pair is silent. Later new or reopened times produce one grouped alert per pair.
- Successful empty results rearm openings. Failed or malformed reads never replace the previous observation.
- Slots are interpreted in restaurant-local wall time, with past times excluded. Only `Standard` slots count; experiences and table attributes are not separate watches.
- Checkpoints and event revisions commit atomically. Adding dates or party sizes leaves existing pairs alone; changing restaurant, timezone, or time window starts fresh baselines.
- Mentions follow the delivery configuration. There is no setup notification or venue-specific state.

## Layout and run

- `config.go`: settings and validation.
- `monitor.go`: lifecycle, polling, and durable opening detection.
- `client.go`: proxy-bound session, HTTP, and bounded recovery.
- `source.go`: persisted-query payload, response decoding, and alert formatting.

Set destination placeholders and a suitable TTL in `monitor.yaml`, then run `monitord test opentable --duration 2m` before deploying. A local test neither saves observations nor sends notifications. This replaces the earlier single-party configuration; migrate `Slug` to `ProfileURL` and `PartySize` to `PartySizes`. Checkpoint identity changes intentionally cause a fresh silent baseline on upgrade.

The adapter targets `www.opentable.com`, database region `NA`, and an undocumented web operation that can change. Missing data is an error, not proof that a restaurant is full. Open the alert link to confirm current availability yourself.
