# RSS & Atom

New posts from a blog, newsroom, podcast, or any RSS2.0 / Atom feed.

## Configure

```go
URL:           "https://blog.cloudflare.com/rss/",
TitleContains: "",
Category:      "",
Interval:      10 * time.Minute,
```

Title matching is case-insensitive. `Category` matches an exact category or Atom tag; leave either empty to accept all entries.

## Behavior

- Reads RSS GUIDs or Atom IDs, with RSS links as an identity fallback.
- Normalizes links and extracts text and images from feed content.
- Baselines silently, then emits new entry identities—not edits or feed reordering.
- Groups durable writes in bounded batches so larger feeds can make progress.

Uses the feed’s retained entry window; it does not crawl historical archives. A malformed entry fails the scan without advancing the checkpoint.

The example is a public Cloudflare Blog article with its supplied artwork.

## Run

Set your Discord account and channel in `monitor.yaml`, then have your agent configure and run this monitor with monitord.
