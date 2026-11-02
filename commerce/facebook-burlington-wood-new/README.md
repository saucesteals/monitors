# Burlington wood listings

Separate monitor for TJ, approved by Daniel on 2026-10-28. The generic Marketplace reference remains unchanged.

- Native query: firewood. No local title, keyword, or price rejection after search.
- Center: Burlington, Connecticut 06013, Facebook city ID 109423279083274. Live-verified center: 41.7692, -72.965.
- Radius: request 20 miles (SDK rounds to 32 km) from Facebook. No independent city lookup or distance verification; Facebook may include nearby/out-of-radius recommendations.
- Price: $0 through $1,000,000 USD; no keyword exclusions.
- Every 30 seconds; native current/previous UTC-day filter, newest-first. Fetch only the first page of each query, even when Facebook reports more pages. Four-minute callback timeout; no extra retries.
- Uses Facebook's opt-in `SearchFirstPage` API: one search GraphQL request per warm scan. Bootstrap HTML is fetched on first use and after its one-hour cache expiry. No script downloads, pagination, or geocoding calls. Listings themselves are never cached. A non-rate-limit GraphQL error invalidates metadata for the next scheduled scan, without retrying the failed request.
- Silent first baseline and query-scope changes, listing-ID deduplication across queries; alerts in Discord wood thread 1565146342962106560 through jarvis, no mentions.
- Dedupe is operator-editable `state.seen`, with `state.scope` identifying the query configuration. First rollout imports the original checkpoint `source:9783fb1da603646ddc83`, preserves its IDs, and silently adds current results. The legacy checkpoint is left intact for rollback.
- Health threshold: 3 failed callbacks. Anonymous Facebook access, no login credentials.

The pinned Shop SDK accepts only alphabetic city slugs. The local transport maps the dedicated burlingtonct search path to Burlington CT's verified numeric ID. Other requests, shared SDKs, and existing deployments remain unchanged.

The module uses the installed monitord SDK at /Users/jarvis/.monitord/lib. Pass this directory's absolute path to monitord test and deploy.

Location text comes directly from each listing. Missing text is shown as "Not listed", never replaced with the search city. Alerts label the radius as a Facebook search filter rather than a verified pickup distance. This intentionally favors low request volume over exhaustive coverage: older/deeper results and synonyms outside this query can be missed. A scope change silently baselines the new first-page view and retains all existing seen IDs.

## Simplification — 2026-11-02

Daniel explicitly relaxed earlier exact-coverage/location requirements. The six-query exhaustive scan was disproportionate for a five-minute new-listing watch. Initially compared firewood plus free logs: firewood returned 24 listings; free logs returned one duplicate plus an unrelated antenna and thermometer. Dropped free logs after inspecting those actual results. The retained firewood feed still includes accessories and farther-away recommendations, and is not exhaustive. The shared Shop `Search` API and other deployments retain their existing behavior; only this monitor opts into first-page/native-location mode.

## Historical query audit — 2026-10-28 (superseded coverage goal)

Compared 33 query variants over 107 returned native pages, with the same center, radius, newest sort, price range and listing-age constraint. Broad `wood` returned mostly furniture; `free wood` was heavily polluted by service spam. Quotes did not establish phrase matching, `firewood OR logs` missed known firewood results, and `firewood -rack -splitter` returned no local products. Final holdout queries `tree trunks`, `wood chunks` and `firewood rounds` added no local results in this snapshot. Do not assume SQL/Google-style search operators work.

The selected six-query union returned 18 unique listings in the verification scan, including ten identifiable firewood/raw-tree-wood offers and one slab-round listing. It recovered `1112425284710763` (a $200 seasoned oak tree pile, confirmed in the listing description; buried on page four of broad wood) and `1130635686439867` ($5 hardwood bundles, rejected by the former whole-word title regex). These are sampled coverage findings, not a claim of exhaustive Marketplace recall. Accessories, wanted ads and some unrelated results remain; they are not hidden with downstream relevance filters. Quoted variants and broader synonyms were measured, not added blindly.

## Genuine delivery verification

After deployment completes its silent baseline, pause this deployment, read state through `monitord state get`, and remove exactly one current listing ID from `seen` without changing `scope` or other IDs. Use `monitord state set` with the modified JSON, then resume. The next normal scan must emit that listing through its ordinary transaction/outbox/destination. Verify the Discord message and delivered outbox entry, then run another scan and verify no duplicate. Never edit SQLite directly or substitute a direct renderer/send call for this test. Choose a baseline-only listing with no existing durable event so event-ID deduplication does not mask the test.
