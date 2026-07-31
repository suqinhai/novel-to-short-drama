# Effective Input Resolver

`drama.resolve_effective_inputs(project_id, episode_id, stage)` is the
authoritative, read-only resolution boundary for production stages 05–10 and
17. It does not select rows by creation time. It follows lifecycle status,
`is_current`, project/source bindings, episode production bindings, candidate
current bindings, explicit lock/approval state, and version lineage.

The public read contract is `effective-input-resolution.v1`. Every response
contains all eleven input kinds, their `required`/`optional` requirement,
resolution state, exact IDs, explicit versions, content hash, source status,
missing inputs, blocking diagnostics, a semantic `context_hash`, and a complete
audit `resolution_hash`.

Supported states are:

- `resolved`: legal input that may be consumed.
- `missing`: no legal input exists; blocks only when required.
- `stale`: a formerly valid/current input no longer matches its authority
  chain; always blocks.
- `needs_review`: a draft, pending ledger, or unconfirmed candidate exists;
  always blocks.
- `blocked`: ambiguity, conflict, failed input, or incomplete locked profile
  set; always blocks.

## Consumer contract

Workflows call `drama.claim_effective_inputs(...)` before generation. Effective
projects may proceed only when `status=ready`. Historical projects keep
`input_resolution_mode=legacy`; their claim records diagnostics but allows the
existing compatibility path. Projects created after migration 18 default to
`effective` and receive a current system editing-template binding.

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
