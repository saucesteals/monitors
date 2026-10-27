# Solana token activity

Watch finalized, owner-attributed **net token changes for one mint in transactions touching one indexed Solana mainnet address**. Uses the SDK's managed QuickNode address source, not signature polling plus one RPC call per transaction. Requires authenticated QuickNode endpoints with both enhanced transaction subscriptions and indexed address history enabled.

## Configure

Set all three public keys in `config.go`:

- **Address:** the address used for provider-side transaction filtering and history. Use an owner wallet for transactions that mention that wallet, or one specific token account for transactions that mention that account.
- **Owner:** owner identity expected in `preTokenBalances`/`postTokenBalances`.
- **Mint:** exact SPL token mint; no symbol/name lookup or default decimal assumption.

All placeholders deliberately fail validation. To watch one token account, set Address to that account, Owner to its wallet owner and Mint to the account's mint. One deployment watches one indexed address. Use separate deployments for independent addresses; events can overlap across deployments if one transaction touches both.

Configure these exact monitord secret refs with URLs copied from the provider dashboard:

- `quicknode/solana-mainnet-http-url`
- `quicknode/solana-mainnet-websocket-url`

The URLs carry provider credentials; never commit them or derive one from the other. Required capabilities: HTTP `getGenesisHash`, `getSlot`, enhanced `getTransactionsForAddress` with full transaction/meta records, finalized commitment, inclusive slot filters, ascending sort and pagination; WebSocket `transactionSubscribe`/`transactionUnsubscribe` with account filters and full JSON transactions at finalized commitment. Basic Solana `logsSubscribe` or `getSignaturesForAddress` support alone is insufficient. Check plan/add-on entitlement with your provider; a public RPC endpoint is **not** a supported drop-in substitute. The SDK requests legacy/version-0 transactions, not future transaction versions.

Replace destination placeholders in `monitor.yaml` (default expiry: 30 days). Use an SDK with `catalog/quicknode/solana.AddressEvents`, then run a bounded `monitord test` with your configured secrets before deployment. This consumes provider requests but does not deliver alerts or persist monitord state.

## Alert and recovery behavior

The managed source establishes a **silent baseline** at the latest finalized address transaction (or current finalized slot for an empty address). There is no historical dump or activation event. New successful transactions are examined; zero net changes and failed transactions do not alert. Both live events and HTTP recovery use **finalized** commitment, accepting finalization latency instead of speculative notifications.

The provider applies the watched address filter. HTTP recovery reads complete ascending pages from the durable slot/transaction-index cursor. SDK journal/cursor changes and emitted events commit atomically. Stable IDs include the mainnet address/owner/mint scope and signature, so replay of the same occurrence coalesces. Replay depends on complete provider history; it cannot repair data a provider has pruned or never indexed.

Token amounts use arbitrary-precision integer arithmetic and exact mint decimals from the transaction. All owner-attributed entries for the selected mint are summed before/after, including accounts created or closed in the transaction. Missing metadata, absent token balance arrays, missing owner attribution for the selected mint, malformed raw amounts and inconsistent decimals fail closed rather than inventing zeroes or advancing past an uncertain result. An empty array is valid; an absent array is not.

**Before/after totals cover only the accounts touched in that transaction**, not every account owned by the wallet. An inflow/outflow is a transaction-local net change, not a transfer instruction decoder or proof of a purchase/sale. Token movement that nets to zero is silent. Changing token-account authority can also change owner-attributed totals without an ordinary transfer.

Use a **new deployment for a different Address, Owner or Mint**. The SDK rejects a changed address on an existing cursor, and typed state rejects changed attribution once an event has been handled. Do not edit state or clear only one checkpoint to repurpose a running watch. Changing endpoint credentials or polling interval preserves the existing cursor.

## Coverage limits

Watching a wallet address does **not** automatically watch all its token accounts: incoming transfers may mention only the recipient token account, not its owner. Watching one token account likewise does not discover other accounts, newly created accounts, or activity that never mentions it. This example does not enumerate accounts, track SOL balances, decode swaps, value holdings, attribute real-world owners, or claim complete wallet coverage. Extended/confidential token mechanics are covered only insofar as their spendable balances are represented in ordinary RPC token balance metadata.

## Verification

Built and vetted against monitord `v0.0.0-20261022022052-57d2a46c9343`. Provider connectivity, enhanced-method entitlements and end-to-end live/replay delivery were **not live-verified** for this public example. No private endpoint, owner or mint was used for verification.
