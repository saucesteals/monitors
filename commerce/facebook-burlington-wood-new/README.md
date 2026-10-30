# Burlington wood listings

Separate monitor for TJ, approved by Daniel on 2026-10-28. The generic Marketplace reference remains unchanged.

- Native queries: firewood, free firewood, seasoned wood, free logs, wood logs, log rounds. No local title, keyword, or price rejection after search; all returned, radius-verified listings are eligible.
- Center: Burlington, Connecticut 06013, Facebook city ID 109423279083274. Live-verified center: 41.7692, -72.965.
- Radius: 20 miles (SDK rounds to 32 km), approximate listing city-center distances.
- Price: $0 through $1,000,000 USD; no keyword exclusions.
- Every 5 minutes; native current/previous UTC-day filter, newest-first. Follow pages until Facebook reports no more results; a 10-page safety limit fails visibly instead of silently truncating. Four-minute callback timeout.
- Silent first baseline and query-scope changes, listing-ID deduplication across queries; alerts in Discord wood thread 1565146342962106560 through jarvis, no mentions.
- Dedupe is operator-editable `state.seen`, with `state.scope` identifying the query configuration. First rollout imports the original checkpoint `source:9783fb1da603646ddc83`, preserves its IDs, and silently adds current results. The legacy checkpoint is left intact for rollback.
- Health threshold: 3 failed callbacks. Anonymous Facebook access, no login credentials.

The pinned Shop SDK accepts only alphabetic city slugs. The local transport maps the dedicated burlingtonct search path to Burlington CT's verified numeric ID. Other requests, shared SDKs, and existing deployments remain unchanged.

The module uses the installed monitord SDK at /Users/jarvis/.monitord/lib. Pass this directory's absolute path to monitord test and deploy.

Shop resolves city-page display names first and requires an exact city-page ID; the transport no longer rewrites state names. Unverifiable listings are excluded and remain eligible on later scans. An empty verification-filtered page does not discard other valid pages; the callback fails if verification warnings leave the whole scan without valid listings. Normal out-of-radius removals are not errors. Coverage is limited to Facebook's returned pages, not exhaustive inventory.

## Query audit — 2026-10-28

Compared 33 query variants over 107 returned native pages, with the same center, radius, newest sort, price range and listing-age constraint. Broad `wood` returned mostly furniture; `free wood` was heavily polluted by service spam. Quotes did not establish phrase matching, `firewood OR logs` missed known firewood results, and `firewood -rack -splitter` returned no local products. Final holdout queries `tree trunks`, `wood chunks` and `firewood rounds` added no local results in this snapshot. Do not assume SQL/Google-style search operators work.

The selected six-query union returned 18 unique listings in the verification scan, including ten identifiable firewood/raw-tree-wood offers and one slab-round listing. It recovered `1112425284710763` (a $200 seasoned oak tree pile, confirmed in the listing description; buried on page four of broad wood) and `1130635686439867` ($5 hardwood bundles, rejected by the former whole-word title regex). These are sampled coverage findings, not a claim of exhaustive Marketplace recall. Accessories, wanted ads and some unrelated results remain; they are not hidden with downstream relevance filters. Quoted variants and broader synonyms were measured, not added blindly.

## Genuine delivery verification

After deployment completes its silent baseline, pause this deployment, read state through `monitord state get`, and remove exactly one current listing ID from `seen` without changing `scope` or other IDs. Use `monitord state set` with the modified JSON, then resume. The next normal scan must emit that listing through its ordinary transaction/outbox/destination. Verify the Discord message and delivered outbox entry, then run another scan and verify no duplicate. Never edit SQLite directly or substitute a direct renderer/send call for this test. Choose a baseline-only listing with no existing durable event so event-ID deduplication does not mask the test.
