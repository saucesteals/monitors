# Uniqlo online restocks

Watch a product's exact color/size pairs using Uniqlo's storefront commerce response. No account or API key is required. This is online inventory, not store pickup.

## Configure

Edit `config.go` and replace all placeholders:

- `Country` / `Language`: storefront path segments, default `us` / `en`. Only storefronts exposing the same v5 commerce schema are supported; regional compatibility is not guaranteed.
- `Product`: full product identifier, shaped like `E` + six digits + `-` + three digits. Copy from the public product URL, not a product title.
- `PriceGroup`: two-digit group from the product URL and commerce request. Defaults to `00`; verify for your product.
- `Variants`: 1–32 explicit `{Color: "…", Size: "…"}` pairs. Color codes are two digits, size codes three digits. Leading zeros matter. Display codes are not human-readable size names and must be checked against the selected product.
- `Interval`: five minutes by default; minimum one minute.
- `UseProxy`: false by default. When enabled, configure `proxies/uniqlo` as a JSON array of proxy URLs. The lifecycle-owned SDK HTTP client supports the normal browser profile and closes idle connections on shutdown.

On the chosen public product page, select the desired color and size. Inspect the browser network request ending in `/products/<product>/price-groups/<group>/l2s`. Verify `result.l2s` contains the selected `color.displayCode` and `size.displayCode`; its `l2Id` joins to `result.stocks`. The alert links back to that exact selection. Do not configure an arbitrary endpoint or copy account/session headers.

## Behavior

The first complete poll establishes a silent baseline. Each subsequently observed unavailable → available transition emits one event for that pair. Continued availability stays silent; repeated restocks can alert again. Availability means an explicitly present positive quantity and a nonempty status other than `STOCK_OUT`. This is not checkout validation: other status codes may have regional ordering restrictions, so confirm that rule for the storefront you use.

Missing variants, missing quantity/status, duplicate selected variants, malformed JSON, application errors and HTTP failures preserve the last good inventory snapshot. No missing field is interpreted as sold out. Failed polls use persisted exponential backoff (2–32 minutes, or the configured interval if longer), honoring longer `Retry-After` values. Success resets the backoff.

Checkpoints hold inventory and a monotonic event sequence. Updates and emitted events commit atomically. Product, storefront, price group or selected-pair changes start a new silent baseline; reordering pairs or changing interval/proxy settings does not. Stock changes entirely between polls cannot be recovered. Availability is not a reservation; this monitor never purchases anything.

## Run and verification

Replace the delivery account/channel placeholders in `monitor.yaml`, then run `monitord test` on your configured copy before deploying. That command contacts Uniqlo but does not persist state or deliver notifications. The default TTL is 30 days. No live storefront probe was performed while authoring this example; verify endpoint and response-schema compatibility before deployment.
