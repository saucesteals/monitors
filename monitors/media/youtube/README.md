# YouTube

New uploads from any public YouTube channel, without an API key.

## Configure

```go
ChannelID:     "UCBJycsmduvYEL83R_U4JriQ",
TitleContains: "",
Interval:      5 * time.Minute,
```

Use the channel’s `UC…` ID. `TitleContains` is an optional case-insensitive filter.

## Behavior

- Reads YouTube’s official Atom channel feed.
- Normalizes the feed-level channel ID, which can omit `UC`, and verifies entry channel IDs.
- Baselines silently; later video IDs emit once even if titles or view counts change.
- Uses the published thumbnail, description, channel name, and video link.

The feed contains the latest15 uploads, so choose a cadence appropriate for the channel’s upload volume. Source errors preserve the checkpoint.

The example uses a public Marques Brownlee upload.

## Run

Set your Discord account and channel in `monitor.yaml`, then have your agent configure and run this monitor with monitord.
