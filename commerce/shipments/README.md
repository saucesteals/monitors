# Shipment updates

One watch for your saved packages. Add a shipment to [Shop](https://github.com/saucesteals/shop), and this monitor picks it up automatically: changed scans, corrected history, and meaningful delivery-estimate changes. No setup announcement or replay of every old scan.

## Save packages with Shop

```sh
shop track add <tracking-number> --label "Desk equipment" --merchant "Example Store"
shop track list
shop track list --all
shop track remove <tracking-number>
```

`add` saves a record without contacting the carrier. A one-off `shop track <tracking-number>` lookup does **not** save it. No shopping login or store-provider import is needed for tracking.

For a separate collection, pass `--config /absolute/path/to/shop-profile` to these commands and set `ShopConfig` to that same absolute path. An empty `ShopConfig` uses Shop's default `~/.config/shop`. The daemon must run as a user that can read and write that collection. Use an explicit path, not `~` or an environment-variable expression in the Go string.

## Configure

Edit `config.go`. Defaults watch all saved shipments, regardless of merchant or carrier:

```go
ShopConfig:       "",
Interval:         10 * time.Minute,
LookupTimeout:    35 * time.Second,
MaxRefreshes:     8,
MaxObservations:  50,
ETAMinimumChange: time.Hour,
```

Optional selection filters combine with **AND**; entries within a list combine with **OR**:

- `Carriers: []tracking.Carrier{tracking.UPS, tracking.USPS}` — restrict by Shop's tracking-number detection. Available constants: `UPS`, `USPS`, `FedEx`, `GOFO`, `Yanwen`.
- `Merchants: []string{"Example Store"}` — case-insensitive exact matches, ignoring surrounding whitespace.
- `LabelContains: "desk"` — case-insensitive label substring.
- Empty filters accept everything. Merchant/label edits are picked up on subsequent polls; metadata edits alone do not notify.

Delivery-age settings control **carrier requests**, not the saved-snapshot observation queue:

- Default: refresh unresolved packages and deliveries dated today.
- `DeliveredSince: "2026-01-01"` — also refresh deliveries on/after this calendar date.
- `AllDeliveries: true` — keep refreshing all deliveries. Cannot be combined with `DeliveredSince`.

Age selection delegates to Shop's `tracking.Filter`: it uses the latest saved scan, never retrieval time. Unknown statuses/dates remain eligible. Offset-bearing dates use the host's local timezone; dates without offsets retain the carrier calendar date. No records are removed.

## What produces an alert

- Each shipment's first usable observation is silent. A successful fresh lookup is used when available; an existing saved snapshot can otherwise establish the baseline.
- Any change to scan history produces **one update**, not one notification per historical scan. That includes corrected or removed scans and history reordering.
- ETA appearance, disappearance, calendar-date changes, and changes in precision or window shape notify. Recognized time estimates must move by at least `ETAMinimumChange` (one hour by default); a shift in either window endpoint counts.
- Small ETA shifts are compared with the **last baseline/announced estimate**, not the previous poll, so gradual delays accumulate. A scan update also announces and anchors the current ETA. Set the threshold to zero to report any actual time shift.
- Date-only estimates, UPS carrier-local windows, and Shop's offset-bearing FedEx/Yanwen windows are understood. Unknown ETA formats use conservative textual comparison. Estimates are never guarantees.
- Fetch timestamps, upstream freshness metadata, tracking URLs, and local label/merchant edits do not create alerts.

Notifications contain the package label (or number), latest scan/time/location, current estimate, previous estimate when it changed significantly, merchant, and a tracking link. Order IDs and notes are not included; mentions are suppressed. Choose a destination appropriate for that information.

## Durable progress and queue limits

Each poll refreshes at most **8** eligible shipments and reconciles at most **50** saved snapshots by default. These are independent rotating queues, so archived packages do not delay the live carrier queue. Their checkpointed lexical cursors survive restarts, removals, and additions. Cursor advancement happens before I/O: a slow or failed item does not monopolize the head of the next batch. It gets another turn after the queue wraps.

Successful packages commit even when other refreshes fail; a partial failure still marks the callback unhealthy. An emitted event and its new observation checkpoint are atomic, with checkpoint-backed event sequences. Saved results left by an interrupted poll are reconciled on a later turn. Per-shipment deadlines bound network work, and the poll budget includes all configured lookups plus one minute for local work.

With a stable collection of `N` eligible packages, every package gets a refresh turn within `ceil(N / MaxRefreshes)` polls, absent storage/worker failures. The saved queue similarly cycles through **all selected records**, including old deliveries. This catches externally refreshed delivery snapshots after age filtering stops carrier requests. Changes are eventually observed, not necessarily in the next poll.

Baselines are scoped to the Shop configuration directory and shipment creation time. Changing polling/presentation/selection settings preserves existing baselines. Removing and re-adding a number creates a new silent baseline; selecting a previously unobserved package does too. Checkpoints for removed shipments are retained; this example does not garbage-collect them.

## Run

Use your normal monitord installation to scaffold `shipments`, then replace **all scaffold `.go` files** with this directory's Go files and copy `monitor.yaml`. Do not leave the generated `check.go` in place. From the installation's shared `monitors` Go module, add Shop:

```sh
go get github.com/saucesteals/shop@v0.0.0-20261002000718-1a7a960631f5
```

Set the placeholder Discord account/channel in `monitor.yaml` to your own destination. The included TTL is 30 days; extend or renew it if this should remain running longer.

```sh
monitord test shipments --duration 6m
monitord deploy shipments
monitord inspect shipments
```

**`monitord test` suppresses delivery and does not persist monitord state/checkpoints, but it is not read-only for Shop.** `Refresh` contacts tracking providers and writes successful snapshots to the selected Shop collection, including during tests. For a network-free smoke check, use a separate empty directory for `ShopConfig` in a scratch copy. An empty collection performs no carrier lookups and emits no events. A fresh dry-run cannot prove change notifications against an existing baseline.

## Carrier coverage and limits

Shop supports USPS through Stamps, standard UPS numbers, 12-/15-digit FedEx numbers, GOFO US waybills, and Yanwen Express waybills. FedEx depends on Deliveries for short-lived access tokens. Carrier detection is format-based, not proof that a shipment exists; unsupported formats do not fall back to another provider. Source services may cache results, rate-limit, change formats, or stop working. Successful retrieval does not prove fresh carrier data.

Network requests and observation processing are bounded per poll, but Shop's current `List` API has **no pagination**: it reads the entire local collection once per callback. Very large collections can therefore exceed the local-work budget. A malformed/unreadable saved record can fail that listing before any work starts. Per-item refresh/read failures do not block other items, but checkpoint/storage errors stop the callback. Carrier failures retry on a later queue turn; this example does not implement carrier-specific backoff or `Retry-After` handling.

Shop snapshot writes are atomic but not cross-process transactions. Prefer one refresher per collection; concurrent CLI/monitor refreshes can overwrite one another. Shop writes and monitord commits are separate transactions; the saved queue repairs the usual interrupted handoff, but an external writer can overwrite an intermediate snapshot before it is observed. This is change detection over snapshots, not a guaranteed archive of every carrier occurrence.

Build compatibility: monitord `v0.0.0-20261022022052-57d2a46c9343` and Shop `v0.0.0-20261002000718-1a7a960631f5`. Empty-collection validation does not verify authentic carrier responses or real notification delivery.
