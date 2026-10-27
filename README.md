# monitors

Open-source monitors for restocks, reservations, shipments, wallet activity, and more. Built with [monitord](https://github.com/saucesteals/monitord).

## Using an agent

Share this collection with your agent, tell it what to watch, and choose where alerts should go. Your agent can read the monitor’s README, configure it, and run it with [monitord](https://github.com/saucesteals/monitord).

Every example includes source, configuration, and a guide to what it watches—and what it doesn’t. Start with a monitor below.

## Shopping

**Find your size. Catch the restock. Know when pickup opens.**

- **[Shopify drops](commerce/shopify/README.md)** — New products, restocks, and price drops filtered to what you want.
- **[Amazon](commerce/amazon/README.md)** — Offers that match your price, seller, and shipper rules.
- **[Marketplace](commerce/marketplace/README.md)** — Local finds by search, radius, and budget.
- **[Apple Store](commerce/apple-store/README.md)** — Pickup availability for selected products and stores.
- **[Best Buy](commerce/bestbuy/README.md)** — Local pickup stock near a ZIP code.
- **[Uniqlo](commerce/uniqlo/README.md)** — Online restocks for exact color and size combinations.

<a href="commerce/shopify/preview.png"><img src="commerce/shopify/preview.png" width="240" alt="Example Shopify drop alert" align="top"></a>
<a href="commerce/marketplace/preview.png"><img src="commerce/marketplace/preview.png" width="240" alt="Example Marketplace listing alert" align="top"></a>
<a href="commerce/amazon/preview.png"><img src="commerce/amazon/preview.png" width="240" alt="Example Amazon offer alert" align="top"></a>

## Travel & dining

**The seat, stay, or table you’ve been waiting for.**

- **[Airbnb](travel/airbnb/README.md)** — Matching stays for your dates, area, and guest count, checked against a full-stay quote.
- **[Award seats](travel/award-seats/README.md)** — Flight availability by cabin, seat count, and points budget.
- **[Global Entry](travel/global-entry/README.md)** — Earlier interview appointments before your deadline.
- **[OpenTable](dining/opentable/README.md)** — Restaurant openings for your date, party size, and time window.
- **[TableCheck](dining/tablecheck/README.md)** — Availability at your chosen restaurant and seating.

<a href="travel/global-entry/preview.png"><img src="travel/global-entry/preview.png" width="240" alt="Example Global Entry appointment alert" align="top"></a>
<a href="travel/award-seats/preview.png"><img src="travel/award-seats/preview.png" width="240" alt="Example award-seat availability alert" align="top"></a>
<a href="dining/tablecheck/preview.png"><img src="dining/tablecheck/preview.png" width="240" alt="Example TableCheck reservation alert" align="top"></a>

## Shipments

**One watch for all your saved packages.**

**[Shipment tracking →](commerce/shipments/README.md)**

Save packages with [Shop](https://github.com/saucesteals/shop); the monitor picks them up automatically. Get scan updates and meaningful delivery-estimate changes without replaying every old scan. Fair polling keeps a slow package from holding up the rest.

Filter by carrier, merchant, or label—or watch the whole collection. Supports UPS, USPS, FedEx, GOFO, and Yanwen through Shop.

## Wallet activity

**Follow on-chain activity with explicit coverage and recovery rules.**

- **[Ethereum wallet](crypto/ethereum-wallet/README.md)** — Direct wallet transactions, with replay and reorganization corrections.
- **[Solana token activity](crypto/solana-token-activity/README.md)** — Finalized net token changes for a selected address, owner, and mint.

Both require QuickNode. These are scoped activity monitors, not complete wallet trackers; each guide explains exactly what is covered.

## Feeds & security

**Keep up with releases, publishing, outages, and vulnerabilities.**

- **[GitHub releases](development/github-releases/README.md)** — New releases, optional prereleases, and tag filters.
- **[YouTube](media/youtube/README.md)** — Uploads from selected channels.
- **[RSS & Atom](media/rss/README.md)** — New articles filtered by title or category.
- **[Statuspage](infrastructure/statuspage/README.md)** — Component incidents and resolutions.
- **[CVE alerts](security/cve/README.md)** — Vulnerabilities filtered by product, keyword, reference, and severity.

<a href="development/github-releases/preview.png"><img src="development/github-releases/preview.png" width="180" alt="Example GitHub release alert" align="top"></a>
<a href="media/youtube/preview.png"><img src="media/youtube/preview.png" width="180" alt="Example YouTube upload alert" align="top"></a>
<a href="media/rss/preview.png"><img src="media/rss/preview.png" width="180" alt="Example RSS article alert" align="top"></a>
<a href="infrastructure/statuspage/preview.png"><img src="infrastructure/statuspage/preview.png" width="180" alt="Example Statuspage incident alert" align="top"></a>

---

## Bring your monitor

**All PRs are welcome. Any monitor, any category.**

Built something useful, niche, or just fun? [Open a PR](https://github.com/saucesteals/monitors/pulls). It doesn’t need to fit an existing category—this collection grows with what people want to watch. Improvements to existing monitors are welcome too.

Include the source, example configuration, and a short README so someone else can run it. Use placeholders for credentials and personal targets.
