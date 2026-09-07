---
scope: package
summary: Owns entry-independent message permission policy, send orchestration, committed sync, and message-event projection contracts.
---

# Message Usecase Flow

## Responsibility

`internal/usecase/message` evaluates legacy-compatible SEND permission policy,
orchestrates pre-append hooks and person-directory establishment, delegates
allowed work to `channelappend`, reads committed message pages, and validates
message-event projection requests.

It exposes usecase DTOs and reasons shared by gateway and HTTP adapters without
depending on their frames, JSON, or concrete cluster runtimes.

## Boundaries

- Channel authority routing, message-ID allocation, durable append,
  idempotency, realtime `NoPersist`, and post-commit delivery belong to
  `internal/runtime/channelappend`.
- Optional synchronous before-send Webhook policy runs after plugins, independent
  of plugin skip controls. Its HTTP adapter cannot decide product failure policy.
- Permission stores return raw authoritative facts. This package owns policy
  order and reason precedence.
- Concrete Slot, Channel, cluster, gateway, and access types must not cross the
  package boundary; the import-boundary test enforces this.
- Message-event buffering and durable reduction are owned by the injected
  store; this usecase validates and normalizes requests.

## Main Flows

1. `SendBatchEach` uses an allocation-tight single-item path at cardinality one;
   larger batches coalesce equivalent raw permission reads. Both evaluate the
   same legacy policy, establish each accepted person directory once, run hooks
   in original order, and copy aligned `channelappend` results to original
   indexes.
2. Single and batch sync validate membership and visibility, canonicalize
   Channel IDs, and pass page intent plus an independent visibility floor to
   `PageReader`. It owns latest-page selection, scan bounds, bounded lookahead,
   filtering, ascending order, and `HasMore` for sync and plugin reads. The
   committed-record adapter executes routed scans; sync then clones payloads
   and optionally enriches stream messages with bounded event metadata.
3. Event append validates and canonicalizes its projection key, then delegates
   cache or durable projection behavior to `MessageEventStore`.

## Invariants and Failure Semantics

- System UID, system device, disband, group membership, denylist, allowlist,
  stranger, agent, and visitor rules retain their documented precedence.
  Equivalent reads may be coalesced; commands and outcomes may not.
- A missing submitter is route-not-ready. Permission, directory, or hook
  failures remain item-local; batch order and cardinality are preserved.
- Permission and directory concurrency, batch sync size, page limits, event
  enrichment, and observer data are bounded. No identity enters metric labels.
  Submitter deadline errors preserve the original cause while attaching only
  permission, pre-append, submitter, and pre-submit-budget timings for the
  entry adapter's single existing diagnostic record.
- Terminal source-Channel checks are authoritative and bypass stale permission
  cache state.
- Page preparation never rewrites a caller start sequence to enforce visibility;
  `PageReader` interprets latest intent and the floor together. Command filtering
  remains after the bounded scan and never triggers refill reads. Plugin reads
  retain their separate authorization and response contracts.
- Sync reads committed data only and never mutate membership. A new person
  conversation without membership returns an empty page without a Channel read;
  missing group membership and tombstones still fail validation. Single and batch
  reads map storage and routed Channel-not-found errors to empty pages after
  membership validation; other read failures remain errors.
- Stream-finish projection fails closed when authority movement loses required
  cache-only lanes; callers must replay deltas or provide a complete snapshot.

## Read First

- [Permission policy](permission.go)
- [Send orchestration](send.go)
- [Before-send Webhook policy](before_send.go)
- [Committed sync](sync.go)
- [Message-page policy and read seam](page_reader.go)

## Update Triggers

Update this file when permission precedence, batching or caching, hook order,
append delegation, sync visibility, event projection ownership, or the import
boundary changes.
