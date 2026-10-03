# Global Entry openings

An earlier interview, without refreshing the scheduler.

## Configure

```go
LocationID: 5140,
Label: "JFK International", Timezone: "America/New_York",
Before: "2027-01-01", Interval: time.Minute,
```

## How it behaves

Reads CBP's public interview availability. `LocationID` and `Timezone` must match the enrollment center; find both in the [CBP location directory](https://ttp.cbp.dhs.gov/schedulerapi/locations/?serviceName=Global%20Entry).

`Before` is an exclusive local date. Empty accepts any future opening. Alerts when availability returns or the earliest qualifying appointment moves earlier; later dates update the checkpoint silently. First scan is silent. The event includes the prior appointment when one existed.

No credentials required. The YAML expires after 720 hours; change the TTL for a longer search.

## Recipes

### Any future opening

```go
Before: "",
```

### Before a trip

```go
Before: "2026-12-01",
```

Run separate copies for different enrollment centers; use each center's own IANA timezone.

## Run

Set your Discord account and channel in `monitor.yaml`, then have your agent configure and run this monitor with monitord.
