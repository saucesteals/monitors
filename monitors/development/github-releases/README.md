# GitHub releases

Published releases from any GitHub repository.

## Configure

```go
Repository:         "cli/cli",
IncludePrereleases: false,
TagPrefix:          "",
Interval:           5 * time.Minute,
```

Use `owner/repository`, optionally include prereleases, or narrow releases by tag prefix. Public repositories need no credentials. The optional `github/token` secret enables private-repository access or a higher API allowance.

## Behavior

- Silently records existing releases on first run.
- Uses release IDs rather than tag order or version-string comparisons.
- Ignores drafts; publishing a draft becomes observable.
- Treats prerelease-to-stable promotion as a separate occurrence.
- Includes release notes, source-provided avatar, version, and downloadable asset count.

Polls the latest100 published release records. Release-note edits are not new releases. Event emission and seen IDs commit atomically.

The example is a public GitHub CLI release from `cli/cli`.

## Run

Set your Discord account and channel in `monitor.yaml`, then have your agent configure and run this monitor with monitord.
