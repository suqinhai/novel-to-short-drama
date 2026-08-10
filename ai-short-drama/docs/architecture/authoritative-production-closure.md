# Authoritative production closure (migration 24)

Migration 24 is the corrective closure for production steps 05–10 and 17. It
is additive and repeatable, preserves native/history rows, and makes the
following rules enforceable across the database, backend, CMS, and n8n.

## One read authority

`resolve_effective_inputs` wraps lifecycle checks and freezes one
`production-input-snapshot.v1`. The snapshot includes exact native identities,
current immutable entity-version overlays, confirmed candidate selections, and
the current project/source binding with complete provenance. `claim_effective_inputs` rejects every required
missing/stale/needs-review/blocked item for both new and legacy projects. The
seven production workflows consume only this snapshot.

## One formal write authority

IR/spec/plan/pacing/episode/scene/dialogue/action/shot/performance/continuity/
timeline/post-production edits use:

1. change-plan and diff/impact/rebuild preview;
2. explicit confirmation;
3. immutable successor `entity_version` in one transaction;
4. atomic current binding switch;
5. exact stale propagation and pending rebuild creation.

Stale-base execution is rejected. Repeated confirm/execute or request retries
return the existing records. A transaction failure leaves the former current
version and binding unchanged. Rollback/reapply creates another confirmed
successor rather than updating history. Former direct production mutation
routes return HTTP 410 and point clients to change plans.

CMS current reads and Resolver/n8n reads use the same current entity binding.
Native rows remain immutable compatibility identities; they are never treated
as a newer authority than the bound successor.

## Independent candidate evaluation

Generation and review use separate endpoints, keys, provider/model names, and
execution records. Review payloads are blind: they omit generation ordering and
generator identity. Audit rows keep status, start/end times, attempts, retry
metadata, failure reason, provider, and model. A failed/invalid generator or a
failed reviewer produces no successful candidate/score.

## Deployment and verification

Apply migrations 01–24 in order, then run every verify file. Migration 24 also
repairs the final `projects_current_stage_check`, so replaying older migrations
cannot remove `adaptation_planning`. The supported automated entry point is:

```powershell
node scripts/run-phase5-acceptance.js
```

It creates isolated fresh and legacy databases, applies the full sequence,
replays the sequence twice on the fresh database, runs verifies and integration
tests (including IR merge and Phase 21 provider/evaluation), and drops both
databases in `finally`.
