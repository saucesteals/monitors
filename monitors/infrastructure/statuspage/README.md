# Statuspage

Incident updates from any public Atlassian Statuspage.

## Configure

```go
URL:           "https://www.githubstatus.com",
Components:    []string{"Actions"},
MinimumImpact: "none",
Interval:      time.Minute,
```

Use the status-page origin. An empty component list watches everything. Impact levels are `none`, `minor`, `major`, and `critical`.

## Behavior

- Reads the public `/api/v2/incidents.json` endpoint.
- Baselines existing updates silently.
- Emits each new incident-update ID once, including investigation and resolution.
- Shows affected components, impact, source text, and the incident permalink.

Filters use each incident’s component list and impact. The source’s retained incident window bounds recovery after extended downtime.

Examples are published investigation and resolution updates from GitHub Status.

## Run

Set your Discord account and channel in `monitor.yaml`, then have your agent configure and run this monitor with monitord.
