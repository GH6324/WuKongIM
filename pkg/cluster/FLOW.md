---
scope: subtree
summary: Composes Controller state, Slot Multi-Raft metadata, typed node RPC, routing, and replicated Channel runtimes behind Node.
---

# Cluster Runtime Flow

## Responsibility

`pkg/cluster` is the reusable cluster composition root. `Node` owns lifecycle,
readiness, public facade delegation, route publication, and bounded snapshots;
focused subpackages own Controller adaptation, immutable routing, typed node
RPC, Slot reconciliation/proposal, Channel hosting, and observation loops.

All deployments, including a single-node cluster, use these semantics.

## Boundaries

- `control` adapts Controller state, writes, Raft transport, and snapshots;
  `routing` publishes hash-Slot authority; `slots` and `propose` own Slot
  Multi-Raft lifecycle and proposals; `channels` hosts Channel runtimes; `net`
  transports typed node RPC; `observe` runs low-frequency reporting.
- `Node` delegates validated intents. Manager business policy, drain safety,
  access DTOs, and response shaping remain in `internal`.
- Typed RPC routes opaque upper-layer DTOs by registered service. Cluster may
  fence maintenance and ownership but must not absorb delivery or Manager
  business logic.
- Controller, Slot, Channel, transport, and storage implementations remain
  behind public facades and neutral errors.

## Main Flows

1. Lifecycle starts transport and Controller, installs control routes, reconciles
   Slots/Channels and exposes readiness. Stop rejects work and reverses ownership.
2. Slot proposals and metadata facades resolve one immutable route snapshot,
   group Channel- or UID-owned work by physical Slot, execute locally or
   forward, and recheck leadership. Person-directory prepare joins UID
   membership/runtime metadata before publishing directory-ready.
3. Channel append resolves or creates Slot-owned runtime metadata, applies it
   monotonically to the selected runtime, and appends locally or forwards to
   the exact leader while background control/task convergence stays bounded.
4. Conversation hydration batch-reads lifecycle and runtime routes by physical
   Slot, preserves alignment and item errors, and groups heads by exact Leader;
   cold quorum reads require current-authority recovery before read retry.
5. `LocalControlSnapshot` exposes the latest fully Node-applied control state;
   revision-fenced management adapters may use `LocalControllerSnapshot` to read
   Controller-visible state without waiting for runtime task reconciliation.
6. Controller-backed management mutations, including Slot leader-transfer task
   creation, preserve semantic CAS errors and task identity through typed RPC;
   task-result RPC carries executor progress and terminal observations.

## Invariants and Failure Semantics

- Offline generation seals reject incomplete imports and mismatched bootstrap
  configuration before native startup.
- Event sequence reads route to the Slot leader and include durable projections.
- Channel RPC codec v8 carries full durable message protocol fields and command
  flags. Decoders retain v5-v7 compatibility; requests or responses containing
  fields an older codec cannot represent must fail instead of discarding them.
- Route authority is `(HashSlot, SlotID, LeaderNodeID, LeaderTerm,
  ConfigEpoch, RouteRevision)` from one immutable publication. Local
  `AuthorityEpoch` is diagnostic only and never a distributed fence.
- Desired or preferred ownership never substitutes for an observed leader.
  Missing, stale, incomplete, duplicate, or mismatched authority evidence
  fails readiness or the foreground operation closed.
- Slot Raft defaults to a 50 ms local tick, two-tick heartbeat, and 40-tick
  election floor. Heartbeats run every 100 ms and elections start after at
  least two seconds, while proposal replication remains event-driven.
- The data-plane lease expires when the node cannot publish healthy readiness;
  local Channel leaders then reject new writes without discarding admitted work.
- Slot and Channel metadata are authoritative at their current owners. Caches,
  hints, compatibility codecs, and local replicas cannot roll back a newer
  generation or make migration decisions from stale state.
- Runtime-meta creation uses one supervised owner per logical Slot: duplicate
  identities coalesce, unique work is bounded and canonical-sorted, placement
  comes from one current revision, and uncertain proposals retry only rows an
  authoritative reread proves missing.
- UID-owned membership fanout and person-directory batches have fixed
  concurrency. Directory-ready can never hide missing UID membership or
  missing append runtime metadata.
- Repair scans retain Slot/page position across bounded ticks. Cold candidate
  probes load current authoritative local-replica metadata before reading progress.
- Lifecycle, fanout, retries, scans, repairs, tasks and diagnostics stay bounded.
- Maintenance closes business admission before storage replacement and keeps
  only explicitly allowed restore RPC available. Backup and restore retain
  cluster routing and exact authority fences.

## Read First

- [Public API](api.go)
- [Node ownership](node.go)
- [Lifecycle](node_lifecycle.go)
- [Routing publication](routing/router.go)
- [Channel hosting](channels/service.go)

## Update Triggers

Update this file when `Node` lifecycle or readiness changes, subtree ownership
moves, route identity or publication changes, Slot or Channel authority flow
changes, a typed RPC gains policy, or maintenance/backup semantics change.
