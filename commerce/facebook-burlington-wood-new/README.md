# Burlington wood listings

Separate monitor for TJ, approved by Daniel on 2026-10-28. The generic Marketplace reference remains unchanged.

- Queries: firewood, free wood, wood, wood logs; title matches whole-word firewood or wood.
- Center: Burlington, Connecticut 06013, Facebook city ID 109423279083274. Live-verified center: 41.7692, -72.965.
- Radius: 20 miles (SDK rounds to 32 km), approximate listing city-center distances.
- Price: $0 through $1,000,000 USD; no keyword exclusions.
- Every 5 minutes; past-day listings, up to two newest-result pages per query.
- Silent first baseline, listing-ID deduplication across queries; alerts in Discord channel 1557018438587916389 through jarvis, no mentions.
- Health threshold: 3 failed callbacks. Anonymous Facebook access, no login credentials.

The pinned Shop SDK accepts only alphabetic city slugs. The local transport maps the dedicated burlingtonct search path to Burlington CT's verified numeric ID. Other requests, shared SDKs, and existing deployments remain unchanged.

The module uses the installed monitord SDK at /Users/jarvis/.monitord/lib. Pass this directory's absolute path to monitord test and deploy.

The transport expands CT/MA/NY/RI state abbreviations in place-lookup queries: live checks returned empty results for abbreviated Connecticut names and valid coordinates for full names. Unverifiable listings are excluded and remain eligible on later scans; a page entirely lost to verification errors fails the callback. Normal out-of-radius removals are not errors. Coverage is limited to Facebook's returned pages, not exhaustive inventory.
