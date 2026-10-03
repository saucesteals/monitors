# Restaurant cancellations

Catch an exact dinner time reopening—not a generic restaurant update.

## Configure

```go
Shop: "fourseasonshotel-kyoto-sushionodera",
Label: "Sushi Ginza Onodera · Kyoto", Timezone: "Asia/Tokyo",
Times: []string{"2026-12-11T18:00", "2026-12-11T19:30"},
Adults: 2, Children: 0, Infants: 0,
MenuID: "6943ecd1ba31f5c7efe0a180", ServiceCategory: "",
GroupOrder: true, Interval: 2 * time.Minute,
```

## How it behaves

Uses TableCheck's availability endpoint with the same party, menu, category, and local time as its booking form. No reservation is submitted.

Use the shop slug from `/shops/SLUG/reserve`. Menu IDs and service-category IDs come from that shop's reservation form—not the visible menu names. Match `GroupOrder` to the form's `data-is-group-order`; non-group orders send the adult count as quantity. Leave optional identifiers empty only if that shop permits it. Some venues require additional fields and need their adapter extended.

Every requested time has its own silent baseline and re-alerts after becoming unavailable and reopening. Past times are skipped. Known unavailable responses rearm the edge; generic failures and unknown responses preserve the checkpoint. `monitor.yaml` expires after 90 days; adjust it to your date.

## Recipes

### One exact seating

```go
Times: []string{"2026-12-11T19:30"},
Adults: 4,
```

### Another restaurant

Change `Shop`, `Label`, `Timezone`, `MenuID`, `ServiceCategory`, and `Image` together. The photograph is optional; the reservation parameters must match the new venue's form.

## Run

Set your Discord account and channel in `monitor.yaml`, then have your agent configure and run this monitor with monitord.
