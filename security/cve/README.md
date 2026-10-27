# CVE alerts

New and materially changed CVEs for a configurable software watchlist. Reads the public [NVD CVE API](https://nvd.nist.gov/developers/vulnerabilities); no account is required. The optional `nvd/api-key` secret can be supplied through monitord's secret resolver.

## Configure

The example watches **OpenSSH**, with a minimum CVSS of **7.0**, while also notifying about matching records that have not been scored yet. Edit `defaultWatchlist` in `config.go` before first deployment, or replace the watchlist through typed state afterward:

```json
{
  "watchlist": {
    "rules": [
      {
        "name": "OpenSSH",
        "products": [{"vendor": "openbsd", "product": "openssh"}],
        "keywords": ["OpenSSH"],
        "reference_prefixes": ["https://www.openssh.com"],
        "exclude_keywords": []
      }
    ],
    "min_cvss": 7,
    "include_unscored": true,
    "notify_updates": true
  }
}
```

Save that JSON as `watchlist.json`, then run `monitord state set cve watchlist.json`. State replacement replaces the whole watchlist, not just the edited rule. An absent or `null` watchlist uses the compiled defaults. An empty rule list is rejected when the monitor polls.

### Matching

- **Rules are ORed.** Within a rule, any product pair, keyword, or reference prefix can match. One CVE produces one notification listing all matching rules.
- **Products** are exact, case-insensitive vendor/product pairs from vulnerable CPE entries or NVD's affected-product data. Use the names actually present in the source; vendor aliases may require additional pairs.
- **Keywords** are case-insensitive substrings of the English description. Use distinctive phrases to avoid unrelated products with similar names.
- **Reference prefixes** match an exact host and a whole path prefix. For example, `https://github.com/example/project` matches that repository and its child paths, not `project-other` or an unrelated host. HTTP references to the same host/path also match. References are inspected as strings, never fetched.
- **Exclude keywords** veto that rule, even if one of its selectors matches. Another rule may still match.
- **Minimum CVSS** uses the highest reported base score across available CVSS versions and sources. Alerts name the selected score's version and source; this is not a claim that all scorers agree. `include_unscored` controls records without a score.

Product/CPE metadata is often incomplete at first publication. Combine precise product pairs with description and repository-reference selectors when possible. Matches are software-level notices, not installed-version assessments. There is no inferred fixed version or automatic upgrade.

## Behavior

- Polls hourly, with a two-minute indexing lag and a one-hour overlap. Matches are evaluated locally against **all modified records**, not a vendor-only server search.
- Silently baselines the preceding hour plus overlap on first use. This is a forward watch, **not a historical vulnerability inventory**. An older CVE can still alert when it is later modified or becomes a match.
- Saves the baseline cutoff before requesting data. A failed initial scan retries the same baseline instead of moving the cutoff forward and suppressing newly arriving records.
- Pages through each fixed modification window. Only a fully processed window advances the cursor; failures retain progress for retry. Partial-window observations and emitted events commit atomically, so retries do not repeat delivered revisions.
- Keeps one small checkpoint per observed matching CVE rather than an ever-growing single seen-ID blob. Checkpoints survive event retention and redeployment.
- With `notify_updates`, alerts on material changes to description, reported scores/vectors, products/ranges, reference URLs, or CISA KEV status. Timestamp-only changes, array reordering, duplicate entries, and routine NVD analysis-status changes are silent. A previously eligible CVE being rejected produces a withdrawal notice; a score downgrade or other change that removes eligibility produces one final update.
- With updates disabled, still alerts when a record first becomes eligible, including an unscored or below-threshold record later crossing the threshold. Rejected records never create new-match alerts.
- Uses a durable revision sequence so a change from A to B and back to A is not lost to content-hash deduplication.
- Selector or threshold/policy changes start a **new silent baseline for the entire watchlist**. Rename/reorder rules, change delivery, or change the polling interval without resetting progress. Old scopes remain available if a previous watchlist is restored.

Requests are spaced at least six seconds apart, even with an API key. HTTP 429/5xx responses receive bounded retries respecting `Retry-After`; other errors fail the poll. This allowance is shared with other clients using the same public IP or key. Persistent failures surface through monitord health.

Recovery processes at most one six-hour modification window per poll, rather than jumping to the present after downtime. Pages contain at most 100 records to bound large responses. NVD is a changing, paginated index, not a transactional change stream: overlap reduces indexing/pagination races but cannot guarantee recovery of revisions that never appear in that window. NVD publication/enrichment delay also affects alert latency. A failed or unusually large window is retried, never silently truncated; review monitor health if it cannot complete within the ten-minute callback limit.

Alerts include the CVE link, description, matching rules, highest reported CVSS, available affected ranges, and known-exploited status when NVD supplies it. CPE ranges are informational; platform conditions are not evaluated and a range boundary is not presented as proof of a fixed release.

## Run

Set your Discord account and channel in `monitor.yaml`, then have your agent configure and run this monitor with monitord. Keep deployment-specific watchlists and delivery configuration outside the public example.

`monitord test cve --duration 10m` runs one poll without saving state or sending notifications. A fresh test should finish with a silent baseline. Configure the monitor's polling durations in `config.go`; watchlist edits do not require a rebuild when supplied through state.

Source documentation: [NVD CVE API](https://nvd.nist.gov/developers/vulnerabilities) and [API rate limits](https://nvd.nist.gov/developers/start-here).
