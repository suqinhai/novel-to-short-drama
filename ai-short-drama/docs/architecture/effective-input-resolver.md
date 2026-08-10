# Effective Input Resolver

`drama.resolve_effective_inputs(project_id, episode_id, stage)` is the
authoritative, read-only resolution boundary for production stages 05–10 and
17. It does not select rows by creation time. It follows lifecycle status,
`is_current`, project/source bindings, episode production bindings, candidate
current bindings, explicit lock/approval state, and version lineage.

The public read contract is `effective-input-resolution.v1`. Every response
contains all twelve input kinds (including `production_snapshot`), their `required`/`optional` requirement,
resolution state, exact IDs, explicit versions, content hash, source status,
missing inputs, blocking diagnostics, a semantic `context_hash`, and a complete
audit `resolution_hash`.

Supported states are:

- `resolved`: legal input that may be consumed.
- `missing`: no legal input exists; blocks only when required.
- `stale`: a formerly valid/current input no longer matches its authority
  chain; blocks when it is required by the target stage. A stale confirmed
  candidate always blocks downstream consumption.
- `needs_review`: a draft, pending ledger, or unconfirmed candidate exists;
  blocks when required, and always blocks candidate consumption.
- `blocked`: ambiguity, conflict, failed input, or incomplete locked profile
  set; blocks when required by the target stage.

## Consumer contract

Workflows call `drama.claim_effective_inputs(...)` before generation. After
migration 24, every project may proceed only when `status=ready`; the stored
`compatibility_mode` is always false. Migration 24 backfills legacy projects to
`input_resolution_mode=effective`, and the claim function has no compatibility
allow-generation branch.

The resolver context is passed to:

- 05 script and 06 storyboard model messages;
- 07 visual-asset and 08 storyboard-image prompts;
- 09 image-to-video prompts and provider request payloads;
- 10 TTS performance/continuity constraints;
- 17 post-production normalization and timeline creation.

Successful generation calls `drama.record_effective_input_outputs(...)`.
Resolved input IDs and observed hashes are written to
`artifact_input_consumptions`; artifact-backed inputs also create
`artifact_dependencies`, and the full consumed set is captured in an
`artifact_provenance_events` event. A generation attempt without a persisted
output fails with `EFFECTIVE_INPUT_OUTPUT_NOT_FOUND`; a cache/skip path with no
generation is reported as `skipped_no_generation`.

## HTTP

- `GET /api/v1/projects/{projectID}/effective-inputs?stage={stage}`
- `GET /api/v1/projects/{projectID}/episodes/{episodeID}/effective-inputs?stage={stage}`

The endpoints are read-only. The OpenAPI contract is
`contracts/openapi/effective-input-api.v1.yaml`; the response schema is
`contracts/json-schema/effective-input-resolution.v1.json`.

## Stage keys

Numeric stages `05`, `06`, `07`, `08`, `09`, `10`, and `17` are accepted along
with canonical keys `episode_script`, `storyboard_design`, `visual_assets`,
`storyboard_images`, `image_to_video`, `voice_audio`, and `post_production`.

## Migration 24 authoritative snapshot

Migration 24 makes this resolver the only production read authority. Its v2
response includes an immutable `production_snapshot` with exact native
identities, current `entity_versions`, and confirmed candidate-selection
bindings. The current primary project/source binding is frozen in both the
payload and provenance. Every provenance entry contains `source_type`, `source_id`,
`version_id`, `binding_id`, `resolved_at`, and `selection_reason`. The 05–10
and 17 workflow loaders reject an absent or unresolved snapshot and never
supply old rows, defaults, or mock content.

Production candidate generation requires explicit, independent generator and
reviewer provider/model configuration. `deterministic_mock` is registered only
when `CANDIDATE_ENABLE_DETERMINISTIC_MOCK=true`; n8n mock branches additionally
require an explicit test request/project and `MOCK_MODE=true`. Provider error,
timeout, invalid JSON, or reviewer failure is recorded as a failed execution
and cannot create a successful candidate or score.

Candidate regeneration has one narrow remediation rule: it may proceed when
the only Resolver blocker is the stale or unconfirmed candidate selection it
is replacing. It still freezes the resolved production snapshot and cannot
bypass any missing, stale, or blocked required upstream. Stages 05–10 and 17
do not have this remediation exception.
