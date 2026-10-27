# Apple Store pickup

Watch exact Apple part numbers for pickup restocks at nearby stores. Works with products exposed by Apple's retail pickup endpoint—not just iPhones. No Apple account or API key required.

## Configure

Edit `config.go`:

```go
Storefront: "https://www.apple.com/shop",
Location:   "10001",
StoreIDs:   []string{},
Parts:      []string{"REPLACE_WITH_PART_NUMBER"},
Interval:   time.Minute,
UseProxy:   false,
```

- **Storefront:** the regional Apple shop URL, such as `https://www.apple.com/uk/shop`. The US adapter is live-verified; other storefronts must return the same pickup schema.
- **Location:** a postal code accepted by that storefront. `10001` is an illustrative New York search area.
- **Parts:** exact regional part numbers for each finish/storage/configuration. The placeholder intentionally fails validation until replaced. Supports 1–16 parts in one request; a marketing name or base model number is not enough.
- **StoreIDs:** empty means all stores Apple returns for the location—not every store nationally. Set explicit `R…` IDs to restrict coverage. These must appear in that location's response; the setting does not expand Apple's search radius.
- **Interval:** at least one minute. Default expiry is 30 days in `monitor.yaml`.
- **UseProxy:** optional. Declare `proxies/apple-store` as a JSON array of proxy URLs when enabled. Direct and proxy clients use the SDK's Mimic browser profile; no browser version is pinned here.

### Find part numbers and stores

On the chosen regional Apple Store, select a product configuration and check pickup availability for your location. In your browser's network inspector, find `/shop/retail/pickup-message` (with a country prefix where applicable):

- Copy each `parts.0`, `parts.1`, … query value into `Parts`.
- Copy `body.stores[].storeNumber` into `StoreIDs` if you want specific stores.
- Check that `partsAvailability` includes every chosen part and its expected product title.

Use part numbers from that storefront; discontinued or non-buyable products can return HTTP 200 with an application error. This monitor rejects that response rather than treating it as sold out. Configurable-to-order products or products needing additional pickup parameters are not supported by this simple adapter.

## Alert behavior

The first complete response establishes a **silent baseline**. An alert is emitted per store/part only when confirmed unavailable inventory becomes available. Continuous availability stays silent; a later sold-out → available transition alerts again. Product titles and pickup wording come from Apple, and the alert links to the regional product page.

Store/part identity, observations and event sequence are durable. Events and the next snapshot are committed atomically. Changing location, storefront, parts or selected stores starts a fresh baseline; reordering the lists or changing interval/proxy settings does not.

Responses must contain every selected part and every explicitly requested store, with consistent availability flags. HTTP errors, application errors, missing fields and ambiguous availability preserve the previous inventory. Failures use durable exponential backoff (2–32 minutes, or the configured interval if longer), honoring a longer `Retry-After`. Success resets backoff. No rapid retry loop.

With automatic store selection, newly returned or reappearing stores start silently. A store omitted from a response is not assumed to have sold out. Use explicit store IDs when missing coverage should instead fail the poll.

Stock is an observation, not a reservation. This does not monitor shipping estimates, prices or checkout, and it never reserves or purchases anything.

## Run

Replace the account/channel placeholders in `monitor.yaml`. Configure the part numbers, then run `monitord test` before deploying with monitord. The test performs a poll without saving state or delivering alerts. Set mentions in the delivery configuration, not the Go source.
