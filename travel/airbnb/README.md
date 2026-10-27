# Airbnb matching stays

Watch one exact stay, adult guest count, rectangular map area, and optional minimum bedrooms, beds, and bathrooms. A result must have an affirmative **full-stay booking quote** and matching current listing details; search-card presence alone is not availability. This is a read-only, anonymous web adapter: no login, reservations, or holds.

## Configure

Edit `config.go` before running. The destination, dates and rectangle intentionally fail validation until replaced.

- `Destination`: the slug from a public `/s/<destination>/homes` URL, such as `City--Country`. Use an ASCII slug. The rectangle, not the destination label, defines the allowed geographic area.
- `CheckIn` / `CheckOut`: exact `YYYY-MM-DD` dates, 1–90 nights apart. Flexible dates are not supported.
- `Adults`: 1–16. This example does not model children, infants, or pets.
- `Area`: north/south latitude and east/west longitude. Coordinates must form a nonempty rectangle without crossing the antimeridian. Airbnb supplies approximate listing coordinates, not street addresses.
- `MinBedrooms`, `MinBeds`, `MinBathrooms`: optional minima; zero disables that filter. Half-unit increments are accepted. Missing room metadata for an enabled filter is an error, not an assumed match.
- `Currency`: the three-letter currency for quote display. Price text is displayed as supplied; there is no numeric price ceiling, price-drop alert, or inferred all-in total.
- `Interval`: 15 minutes to 24 hours; default one hour. Checks are serialized, with a ten-minute callback deadline and thirty-second request deadlines.
- `MaxPages`: 1–20 pages **per search**, default 10. `MaxListings`: 1–128 retained candidates, default 64. Start with a small area and restrictive filters.

Only the listed criteria are supported. Amenities, property type, accessibility, cancellation policies, and other URL filters are not configurable here. Search-generated parameters are checked to ensure dates, adults, area, currency and enabled room filters survived the website bootstrap. Available candidates are independently checked against current room counts and coordinates before they qualify.

Set destination placeholders and a suitable TTL in `monitor.yaml`. The supplied TTL is 30 days; choose one appropriate to the stay. Checks stop with an error on the UTC date of check-in. Same-day/local-timezone watches are intentionally not supported; retire or update the deployment before then.

## Access and query discovery

The lifecycle owns one reusable `catalog/httpx` client. Direct access is the default. For proxies, set `UseProxy: true` and supply `proxies/airbnb` as a JSON array of HTTP, HTTPS, or SOCKS5 URLs through monitord's secret resolver. The SDK rotates requests across the configured exits; no authenticated cookies or account credentials are used. Proxy mode has no direct-network fallback.

The adapter obtains the anonymous web API key and search variables from the public page. It discovers current persisted-query identifiers by reading the site's referenced JavaScript **as text, never executing it**. Each discovery inspects at most 48 assets and 32 MiB; each response is capped at 8 MiB. Identifiers expire after 24 hours and are cleared following failed observations. Missing optional asset URLs may be skipped on HTTP 404; required operation, search, quote, or detail failures remain errors. There is no hard-coded query hash or build-specific asset filename.

Airbnb's web operations are undocumented and can change or block automated access. HTTP errors, challenges, GraphQL errors, missing fields, changing pagination, exhausted discovery bounds, or timeouts fail the check. There is no immediate retry/rotation loop to get around an access failure; the next scheduled check tries again. Neither a browser-compatible client nor a proxy guarantees access.

## Alerts and checkpoints

- The first **complete successful observation** is a silent baseline, including a valid empty result. Afterward, newly discovered matching availability and previously unavailable/nonmatching candidates that qualify produce alerts.
- Every poll performs both exact-date and date-free discovery. The latter can find currently blocked inventory. All candidates from successful observations remain in the checkpoint and receive full-stay quote checks even if they disappear from ranked search results.
- A quoted `isAvailable: false` or a successfully verified filter mismatch rearms an opening. A missing result, failed quote, malformed response, or missing required detail does **not** mean sold out.
- Polls are all-or-nothing. Any required source failure leaves the entire prior checkpoint unchanged and emits nothing, even if other listings succeeded. A delisted or persistently broken candidate can therefore block progress; this conservative tradeoff avoids guessed closures and reopening alerts.
- Candidates are never automatically evicted. Exceeding either pagination or retained-listing bounds fails explicitly without a partial baseline. Narrow the criteria, increase the bounds within their limits, or intentionally reset the checkpoint after pausing if you accept losing the old baseline. Resetting always starts silently.
- Checkpoints and monotonically numbered events commit together. Event IDs include a hash of the watch criteria and a durable revision, never a clock timestamp. Restarting preserves the baseline; changing stay/search/filter/currency criteria starts a new one. Cadence, proxy mode and safety-bound changes do not reset it.
- This is not an exhaustive inventory census. Search ranking and Airbnb's own result caps can hide properties; pagination bounds cannot remove that limitation. Availability can change between checks or before opening an alert.

## Run and verification

Place this directory in your monitord sources, configure the settings and delivery destination, then run:

```sh
monitord test airbnb --duration 4m
```

The test does not persist observations or deliver alerts. With no previous checkpoint its first successful result should produce no events. To check without configuring a destination, use `deliveries: []` in a separate test copy; do not substitute a real destination into a shared example.

Verified on 2026-10-27 against monitord `v0.0.0-20261022022052-57d2a46c9343`:

- Isolated `go build ./...` and `go vet ./...` passed.
- A synthetic public-area read completed both search passes, current query discovery, a full-stay quote, and listing-detail filter validation.
- An isolated `monitord test` with no deliveries completed a silent baseline: one listing, zero events.
- A separate public search traversed three pages and returned 41 unique listings. Reducing the page bound to one rejected that search before accepting a partial result.

These checks are not guarantees of future access, exhaustive discovery, booking success, or proxy-mode coverage. Reopening delivery and failure recovery were reviewed in code, not exercised against live notifications.

## Layout

- `config.go`: settings, validation, and public URLs.
- `monitor.go`: lifecycle, silent baseline, and atomic checkpoint/event updates.
- `client.go`: HTTP bounds, bootstrap parsing, and query-artifact discovery.
- `search.go`: strict search constraints, pagination, and listing identities.
- `stay.go`: candidate retention, full-stay quotes, and detail-based eligibility.
