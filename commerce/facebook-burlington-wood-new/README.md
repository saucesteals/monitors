# Burlington wood listings

Uses Shop's anonymous, stateless Facebook Marketplace search with explicit Burlington, Connecticut coordinates (41.7692, -72.965), a requested 20-mile radius, newest sort, and current/previous UTC day buckets.

- One `firewood` query and one native first-page GraphQL request every 30 seconds. No HTML bootstrap, JavaScript discovery, pagination, authentication token, or downstream relevance/radius filtering.
- Facebook may return accessories or outside-radius recommendations; radius is an upstream filter, not verified pickup distance. Missing location text stays missing.
- Existing listing-ID state is preserved. A source-configuration change silently baselines current results without forgetting prior IDs or emitting historical listings.
- Requires only `proxies/isps`; delivery remains in the existing wood thread through the named Discord account.
- Shop is pinned to merged revision 8a23fb309faf. The installed monitord SDK is the existing module replacement at `/Users/jarvis/.monitord/lib`.

Validate with `monitord test <absolute-source-path> --stored-state`. This sends no alerts and does not persist state. Deploy only clean committed source, without `--reset-state`.
