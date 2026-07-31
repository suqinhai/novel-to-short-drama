'use strict'

// Idempotently wires the authoritative resolver into the production workflows.
// It intentionally preserves provider configuration and only adds local
// PostgreSQL preflight/provenance nodes plus prompt-context references.

const fs = require('node:fs')
const path = require('node:path')

const root = path.resolve(__dirname, '..')
const postgresCredentials = {
  postgres: { id: 'REPLACE_WITH_POSTGRES_CREDENTIAL_ID', name: 'Short Drama PostgreSQL' },
}

const specs = [
  {
    file: '05-episode-script.json', prefix: '05', stage: 'episode_script',
    upstream: 'Validate and Normalize', downstream: 'Idempotency Task Gate',
    failure: 'Failure Response', success: 'Save Script Scenes Dialogues Review',
    successDownstream: 'Success Response', responseField: 'output_data',
  },
  {
    file: '06-storyboard-design.json', prefix: '06', stage: 'storyboard_design',
    upstream: 'Validate and Normalize', downstream: 'Idempotency Task Gate',
    failure: 'Failure Response', success: 'Save Storyboard Shots Review Usage',
    successDownstream: 'Success Response', responseField: 'output_data',
  },
  {
    file: '07-visual-assets.json', prefix: '07', stage: 'visual_assets',
    upstream: 'Validate and Normalize', downstream: 'Idempotency Task Gate',
    failure: 'Failure Response', success: 'Finalize Visual Asset Batch',
    successDownstream: 'Final Response', responseField: 'response',
  },
  {
    file: '08-storyboard-images.json', prefix: '08', stage: 'storyboard_images',
    upstream: 'Validate and Normalize', downstream: 'Idempotency Task Gate',
    failure: 'Failure Response', success: 'Finalize Frame Batch',
    successDownstream: 'Final Response', responseField: 'response',
  },
  {
    file: '09-image-to-video.json', prefix: '09', stage: 'image_to_video',
    upstream: 'Restore Video Request', downstream: 'Load Approved Images and Video State',
    failure: 'Normalize Video Workflow Failure', success: 'Finalize Video Workflow',
    successDownstream: 'Video Workflow Response', responseField: 'response',
  },
  {
    file: '10-voice-audio.json', prefix: '10', stage: 'voice_audio',
    upstream: 'Restore Voice Audio Request', downstream: 'Load Approved Script Dialogues and Voices',
    failure: 'Voice Audio Failure Response', success: 'Build BGM SFX Plan and Aggregate Completion',
    successDownstream: 'Voice Audio Success Response', responseField: 'output_data',
  },
  {
    file: '17-post-production-creative-workbench.json', prefix: '17', stage: 'post_production',
    upstream: 'Normalize Versioned Request', downstream: 'Load Unified Upstream Context',
    failure: null, success: 'Create Template Timeline Version',
    successDownstream: 'Return Phase 5 Result', responseField: 'template_response',
    explicitTimeline: true,
  },
]

function mainTarget(connections, nodeName) {
  return connections[nodeName]?.main?.[0]?.[0]?.node
}

function reconnect(workflow, from, oldTarget, newTarget) {
  const outputs = workflow.connections[from]?.main || []
  let replaced = false
  for (const output of outputs) {
    for (const edge of output || []) {
      if (edge.node === oldTarget) {
        edge.node = newTarget
        replaced = true
      }
    }
  }
  if (!replaced && mainTarget(workflow.connections, from) !== newTarget) {
    throw new Error(`${workflow.name}: cannot reconnect ${from} -> ${oldTarget}`)
  }
}

function addNode(workflow, node) {
  const existing = workflow.nodes.find(item => item.id === node.id)
  if (existing) Object.assign(existing, node)
  else workflow.nodes.push(node)
}

function injectPreflight(workflow, spec) {
  const upstream = workflow.nodes.find(node => node.name === spec.upstream)
  const downstream = workflow.nodes.find(node => node.name === spec.downstream)
  if (!upstream || !downstream) throw new Error(`${spec.file}: preflight anchors missing`)
  const resolveName = 'Resolve Effective Inputs'
  const guardName = 'Enforce Effective Inputs'
  const resolveID = `${spec.prefix}-effective-input-resolve`
  const guardID = `${spec.prefix}-effective-input-guard`
  const x = Math.round((upstream.position[0] + downstream.position[0]) / 2)
  const y = upstream.position[1]
  addNode(workflow, {
    parameters: {
      operation: 'executeQuery',
      query: "SELECT drama.claim_effective_inputs($1,NULLIF($2,''),$3,$4,$5) effective_inputs;",
      options: {
        queryReplacement: `={{[$json.project_id,$json.episode_id||'','${spec.stage}',$json.trace_id,Number($json.generation_version||1)]}}`,
      },
    },
    id: resolveID, name: resolveName, type: 'n8n-nodes-base.postgres',
    typeVersion: 2.6, position: [x - 70, y], credentials: postgresCredentials,
    onError: spec.failure ? 'continueErrorOutput' : undefined,
  })
  const guardCode = `const request=$('${spec.upstream}').first().json,resolution=$json.effective_inputs;if(!resolution?.allowed){const error=new Error('EFFECTIVE_INPUTS_BLOCKED: '+JSON.stringify({resolution_id:resolution?.resolution_id,status:resolution?.status,missing:resolution?.missing||[],blockers:resolution?.blockers||[]}));error.name='EFFECTIVE_INPUTS_BLOCKED';throw error;}const effective_context=resolution.context||{};return [{json:{...request,effective_inputs:resolution,effective_context,payload:{...(request.payload||{}),effective_inputs:{resolution_id:resolution.resolution_id,context_hash:resolution.context_hash,resolution_hash:resolution.resolution_hash,context:effective_context}}}}];`
  addNode(workflow, {
    parameters: { jsCode: guardCode },
    id: guardID, name: guardName, type: 'n8n-nodes-base.code',
    typeVersion: 2, position: [x + 70, y],
    onError: spec.failure ? 'continueErrorOutput' : undefined,
  })
  reconnect(workflow, spec.upstream, spec.downstream, resolveName)
  workflow.connections[resolveName] = {
    main: spec.failure
      ? [[{ node: guardName, type: 'main', index: 0 }], [{ node: spec.failure, type: 'main', index: 0 }]]
      : [[{ node: guardName, type: 'main', index: 0 }]],
  }
  workflow.connections[guardName] = {
    main: spec.failure
      ? [[{ node: spec.downstream, type: 'main', index: 0 }], [{ node: spec.failure, type: 'main', index: 0 }]]
      : [[{ node: spec.downstream, type: 'main', index: 0 }]],
  }
}

function injectProvenance(workflow, spec) {
  const success = workflow.nodes.find(node => node.name === spec.success)
  const downstream = workflow.nodes.find(node => node.name === spec.successDownstream)
  if (!success || !downstream) throw new Error(`${spec.file}: provenance anchors missing`)
  const name = 'Record Effective Input Provenance'
  const id = `${spec.prefix}-effective-input-provenance`
  const explicit = spec.explicitTimeline
    ? `$('Create Template Timeline Version').item.json.data?.timeline_id||$('Create Template Timeline Version').item.json.timeline_id||''`
    : `''`
  const original = spec.explicitTimeline
    ? `$('Create Template Timeline Version').item.json`
    : `$json.${spec.responseField}`
  addNode(workflow, {
    parameters: {
      operation: 'executeQuery',
      query: `SELECT $1::jsonb ${spec.responseField},drama.record_effective_input_outputs($2,$3,NULLIF($4,'')) effective_input_provenance;`,
      options: {
        queryReplacement: `={{[JSON.stringify(${original}),$('Enforce Effective Inputs').first().json.trace_id,'${spec.stage}',${explicit}]}}`,
      },
    },
    id, name, type: 'n8n-nodes-base.postgres', typeVersion: 2.6,
    position: [Math.round((success.position[0] + downstream.position[0]) / 2), success.position[1]],
    credentials: postgresCredentials,
    onError: spec.failure ? 'continueErrorOutput' : undefined,
  })
  reconnect(workflow, spec.success, spec.successDownstream, name)
  workflow.connections[name] = {
    main: spec.failure
      ? [[{ node: spec.successDownstream, type: 'main', index: 0 }], [{ node: spec.failure, type: 'main', index: 0 }]]
      : [[{ node: spec.successDownstream, type: 'main', index: 0 }]],
  }
}

function injectContextConsumption(workflow, spec) {
  const byID = id => workflow.nodes.find(node => node.id === id)
  if (spec.prefix === '05') {
    const ai = byID('05-ai')
    const contextField = 'effective_inputs:$json.effective_context'
    while (ai.parameters.jsonBody.includes(`${contextField},${contextField}`)) {
      ai.parameters.jsonBody = ai.parameters.jsonBody.replace(
        `${contextField},${contextField}`, contextField,
      )
    }
    if (!ai.parameters.jsonBody.includes(contextField)) {
      ai.parameters.jsonBody = ai.parameters.jsonBody.replace(
        'rewrite_scene_id:$json.rewrite_scene_id',
        `rewrite_scene_id:$json.rewrite_scene_id,${contextField}`,
      )
    }
  }
  if (spec.prefix === '06') {
    const ai = byID('06-ai')
    const contextField = 'effective_inputs:$json.effective_context'
    while (ai.parameters.jsonBody.includes(`${contextField},${contextField}`)) {
      ai.parameters.jsonBody = ai.parameters.jsonBody.replace(
        `${contextField},${contextField}`, contextField,
      )
    }
    if (!ai.parameters.jsonBody.includes(contextField)) {
      ai.parameters.jsonBody = ai.parameters.jsonBody.replace(
        'shot_id:$json.shot_id',
        `shot_id:$json.shot_id,${contextField}`,
      )
    }
  }
  if (spec.prefix === '07') {
    const prepare = byID('07-prepare')
    prepare.parameters.jsCode = prepare.parameters.jsCode.replaceAll(
      'requests:requests.map((q,n)=>({...q,request_index:n}))',
      "requests:requests.map((q,n)=>({...q,request_index:n,prompt:[q.prompt,'权威输入约束：'+JSON.stringify(r.effective_context||{})].join('。'),effective_input_context:r.effective_context||{}}))",
    )
  }
  if (spec.prefix === '08') {
    const prepare = byID('08-prepare')
    const contextEntry = "'权威输入约束：'+JSON.stringify(r.effective_context||{})"
    while (prepare.parameters.jsCode.includes(`${contextEntry},${contextEntry}`)) {
      prepare.parameters.jsCode = prepare.parameters.jsCode.replace(
        `${contextEntry},${contextEntry}`, contextEntry,
      )
    }
    if (!prepare.parameters.jsCode.includes(contextEntry)) {
      prepare.parameters.jsCode = prepare.parameters.jsCode.replace(
        "'高质量短剧关键帧，无文字无水印'",
        `${contextEntry},'高质量短剧关键帧，无文字无水印'`,
      )
    }
  }
  if (spec.prefix === '09') {
    const build = byID('09-build')
    const promptContext = "'权威输入约束：'+JSON.stringify(r.effective_context||{})"
    const payloadContext = 'effective_input_context:r.effective_context||{},effective_input_context_hash:r.effective_inputs?.context_hash||null'
    while (build.parameters.jsCode.includes(`${promptContext},${promptContext}`)) {
      build.parameters.jsCode = build.parameters.jsCode.replace(
        `${promptContext},${promptContext}`, promptContext,
      )
    }
    while (build.parameters.jsCode.includes(`${payloadContext},${payloadContext}`)) {
      build.parameters.jsCode = build.parameters.jsCode.replace(
        `${payloadContext},${payloadContext}`, payloadContext,
      )
    }
    if (!build.parameters.jsCode.includes(promptContext)) {
      build.parameters.jsCode = build.parameters.jsCode.replace(
        "'保持人物、服装、场景与参考帧一致；无字幕、无文字、无水印、无Logo'",
        `${promptContext},'保持人物、服装、场景与参考帧一致；无字幕、无文字、无水印、无Logo'`,
      )
    }
    if (!build.parameters.jsCode.includes(payloadContext)) {
      build.parameters.jsCode = build.parameters.jsCode.replace(
        'mock_async:Boolean(p.mock_async),simulate:p.simulate||null',
        `${payloadContext},mock_async:Boolean(p.mock_async),simulate:p.simulate||null`,
      )
    }
  }
  if (spec.prefix === '10') {
    const build = byID('10-build-items')
    const ttsContext = 'effective_input_context:x.effective_context||{},effective_input_context_hash:x.effective_inputs?.context_hash||null,performance_bible_constraints:x.effective_context?.performance_bible||{},continuity_constraints:x.effective_context?.continuity_ledger||{}'
    while (build.parameters.jsCode.includes(`${ttsContext},${ttsContext}`)) {
      build.parameters.jsCode = build.parameters.jsCode.replace(
        `${ttsContext},${ttsContext}`, ttsContext,
      )
    }
    if (!build.parameters.jsCode.includes(ttsContext)) {
      build.parameters.jsCode = build.parameters.jsCode.replace(
        'volume:Number(d.voice_volume??1)',
        `volume:Number(d.voice_volume??1),${ttsContext}`,
      )
    }
  }
  if (spec.prefix === '17') {
    const normalize = byID('17-normalize')
    normalize.parameters.jsCode = normalize.parameters.jsCode.replace(
      'actor,dialogue_timings:',
      "actor,trace_id:clean(i.trace_id||('trace_phase17_'+require('crypto').randomUUID())),generation_version:Math.max(1,Number(i.generation_version||1)),dialogue_timings:",
    )
    const result = byID('17-result')
    result.parameters.jsCode = result.parameters.jsCode.replace(
      'timeline_version:$json.data||$json',
      "timeline_version:$json.template_response?.data||$json.template_response,effective_input_provenance:$json.effective_input_provenance,effective_input_context:$('Enforce Effective Inputs').first().json.effective_context",
    )
  }
}

for (const spec of specs) {
  const file = path.join(root, 'workflows', spec.file)
  const workflow = JSON.parse(fs.readFileSync(file, 'utf8').replace(/^\uFEFF/, ''))
  injectPreflight(workflow, spec)
  injectProvenance(workflow, spec)
  injectContextConsumption(workflow, spec)
  fs.writeFileSync(file, `${JSON.stringify(workflow, null, 2)}\n`)
}

process.stdout.write(`PASS injected Effective Input Resolver into ${specs.length} workflows\n`)
