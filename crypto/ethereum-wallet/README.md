# Ethereum wallet activity

Watch **direct transactions whose `from` or `to` equals one Ethereum mainnet address**. Includes outgoing contract calls, contract creation, incoming top-level native transfers and failed attempts. Requires authenticated QuickNode HTTP and WebSocket endpoints; no explorer API or market-data service is used.

## Configure

Set `wallet` in `config.go` to a 20-byte `0x…` Ethereum address. The placeholder deliberately fails validation. Set the Discord account/channel placeholders in `monitor.yaml` (default expiry: 30 days).

Configure these exact monitord secret refs with URLs copied from your Ethereum mainnet endpoint dashboard:

- `quicknode/ethereum-mainnet-http-url`
- `quicknode/ethereum-mainnet-websocket-url`

The URLs contain the provider credential. Keep them out of source/config files; do not construct one URL by rewriting the other. Both endpoints must describe chain ID `0x1` and support `eth_chainId`, `eth_blockNumber`, `eth_getBlockByNumber` with full transactions, `eth_getTransactionReceipt`, and WebSocket `eth_subscribe`/`eth_unsubscribe` for `newHeads`. Historical block/receipt availability must cover any downtime you want to recover. A free/public JSON-RPC URL is not assumed to supply these capabilities or throughput.

Use an SDK with `catalog/quicknode/evm.Blocks` (verified build target below). Run `monitord test` for a bounded duration with your configured secrets before deployment. Testing subscribes and makes provider requests but does not deliver notifications or save state. Never point a test at someone else's endpoint credentials.

## Alert and recovery behavior

- The current head block is a **silent activation baseline**. Only later block heights alert, including after a restart; no activation message or historical dump.
- One notification per matching block includes the total matching count and details of up to 12 transactions. Extra matches are explicitly omitted, not lost from the count; follow the block link to inspect them. Only displayed matches require receipt requests.
- Amounts use integer/rational arithmetic with all 18 ETH decimal places. The displayed value is the transaction's requested native value, not a wallet balance delta. Failed transactions explicitly say the value did not transfer. Fees and internal calls are excluded.
- SDK-owned checkpoints atomically advance canonical block progress with the monitor update and event. HTTP replay repairs gaps; WebSocket heads reduce latency. Full blocks are scanned, so costs scale with chain traffic, not wallet traffic.
- Notifications are **head observations, not finalized transactions**. `Confirmations = 64` controls the SDK rollback/recovery journal, not a 64-block notification delay. A matching orphaned block emits a correction. A replacement block can produce a new event. A reorganization deeper than the retained journal fails rather than silently rewriting history; operator recovery is required.
- Recent announced block hashes are retained in typed state for corrections. Event/source identity includes the normalized wallet and Ethereum chain scope. Changing the wallet creates a new scope and silent baseline; cadence or endpoint credential changes do not. Prefer a separate deployment per wallet. Do not manually clear only the typed state while retaining source checkpoints.

## Limits

This is **not complete wallet activity**. It does not discover incoming ERC-20/ERC-721/ERC-1155 transfers from logs, internal native transfers, smart-account operations where the wallet is not the transaction envelope's sender/recipient, pending transactions, or account-wide balance changes. Outgoing calls may move tokens, but this adapter does not decode them. Activity is not proof of a trade or sale. For token logs, build a separate `evm.Events` monitor with explicit contract/topic filters.

Ethereum mainnet only. This example intentionally does not claim generic EVM, L2, or explorer compatibility. The source cannot retract an already delivered notification; it emits a correction. Events have block-hash identity, so replay coalesces the same occurrence.

## Verification

Built and vetted against monitord `v0.0.0-20261022022052-57d2a46c9343`. Provider connectivity, subscription entitlement, replay history and live reorganization behavior were **not live-verified** for this public example. No private endpoint or wallet was used for verification.
