# Best Buy local pickup

Watch one US Best Buy SKU for local pickup restocks. Optional quantity-change alerts also report increases, decreases and sellouts. No account or API key is required by this adapter.

## Configure

Edit `config.go`:

```go
SKU:                   "REPLACE_WITH_SKU",
ZIPCode:               "10001",
RadiusMiles:           25,
StoreIDs:              []string{},
Interval:              5 * time.Minute,
UseProxy:              false,
NotifyQuantityChanges: false,
```

- **SKU:** numeric SKU from the product page, not its model number or newer alphanumeric URL identifier. The placeholder intentionally fails validation.
- **ZIPCode:** five-digit US search ZIP. `10001` is an illustrative New York location.
- **RadiusMiles:** filters distances returned by Best Buy; does not expand server search coverage. Supports greater than zero through 250 miles.
- **StoreIDs:** empty watches all returned stores inside the radius. Restrict with IDs from `ispu.locations[].id`. Every explicit store must appear with inventory inside the radius or the entire poll fails.
- **Interval:** one minute through 24 hours. Default five minutes; `monitor.yaml` expires after 30 days.
- **UseProxy:** optional; configure `proxies/bestbuy` as a JSON array of proxy URLs when enabled. The SDK provides browser-compatible HTTP transport; clients are created once at startup and closed at shutdown.
- **NotifyQuantityChanges:** false emits only confirmed zero → positive restocks. True emits every change between observed quantities, including positive → zero.

Replace the account/channel placeholders in `monitor.yaml`, then run `monitord test` before deployment. Testing makes a real pickup lookup but does not save state or deliver notifications. This example has not been live-verified against Best Buy; build/vet checks do not establish endpoint compatibility.

## Source and coverage

The adapter POSTs a single-item request to Best Buy's website `productfulfillment/c/api/2.0/storeAvailability` endpoint. It joins `ispu.items[].locations[].locationId` to `ispu.locations[].id` and reads `availability.availablePickupQuantity`. ZIP and SKU are sent to Best Buy on each poll. No checkout, reservation, login, shipping availability or purchase actions are performed.

This is an undocumented website endpoint, not the public developer API. Best Buy may block automated requests or change the schema. Responses must include exactly the requested SKU, nonempty pickup metadata and inventory, valid quantities, and 1–128 selected stores. Zero eligible stores is treated as an incomplete search, not a sellout. Unknown/missing quantities, HTTP errors and malformed responses never overwrite inventory. A quantity is an observation, not guaranteed reservable inventory.

## Durable behavior

The first complete successful response establishes a **silent baseline**, including stores already in stock. Known stores alert only on configured quantity transitions. New stores and stores reappearing after omission start silently; omission itself never implies zero stock. Use explicit `StoreIDs` when disappearing coverage should instead fail the entire poll.

Quantities and an increasing event sequence live in a scope-specific checkpoint; alerts and the next snapshot commit atomically. SKU, ZIP, radius and store selection define that scope. Changing those values starts a new baseline; reordering store IDs, changing cadence/transport or toggling quantity-change alerts does not. Returning to a previously used scope resumes its retained checkpoint.

Failures retain observations and persist exponential backoff of 2–32 minutes, at least the configured interval and any longer `Retry-After`. Successful responses clear backoff. No tight retry loop; poll health remains failed during backoff.
