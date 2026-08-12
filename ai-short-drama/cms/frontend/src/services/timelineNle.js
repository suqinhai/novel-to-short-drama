export const NLE_TRACKS = [
  { type: 'video', label: '视频', kind: 'video' },
  { type: 'dialogue', label: '对白', kind: 'audio' },
  { type: 'narration', label: '旁白', kind: 'audio' },
  { type: 'bgm', label: 'BGM', kind: 'audio' },
  { type: 'ambience', label: '环境声', kind: 'audio' },
  { type: 'sound_effect', label: 'SFX', kind: 'audio' },
  { type: 'subtitle', label: '字幕', kind: 'subtitle' },
]

export function orderedNLETracks(order = []) {
  const known = new Map(NLE_TRACKS.map(track => [track.type, track]))
  const result = []
  for (const type of Array.isArray(order) ? order : []) {
    if (known.has(type) && !result.some(item => item.type === type)) result.push(known.get(type))
  }
  for (const track of NLE_TRACKS) if (!result.some(item => item.type === track.type)) result.push(track)
  return result
}

export function moveTrack(order = [], type, direction) {
  const result = orderedNLETracks(order).map(track => track.type)
  const index = result.indexOf(type)
  const target = Math.max(0, Math.min(result.length - 1, index + Math.sign(direction || 0)))
  if (index < 0 || target === index) return result
  const [item] = result.splice(index, 1)
  result.splice(target, 0, item)
  return result
}

export function integerMS(value, fallback = 0) {
  const number = Number(value)
  return Number.isFinite(number) ? Math.max(0, Math.round(number)) : fallback
}

export function formatNLETimecode(value) {
  const total = integerMS(value)
  const hours = Math.floor(total / 3600000)
  const minutes = Math.floor((total % 3600000) / 60000)
  const seconds = Math.floor((total % 60000) / 1000)
  const milliseconds = total % 1000
  const base = `${String(minutes).padStart(2, '0')}:${String(seconds).padStart(2, '0')}.${String(milliseconds).padStart(3, '0')}`
  return hours ? `${String(hours).padStart(2, '0')}:${base}` : base
}

export function frameDurationMS(fps = 25) {
  const value = Number(fps)
  return Math.max(1, Math.round(1000 / (Number.isFinite(value) && value > 0 ? value : 25)))
}

export function stepPlayhead(currentMS, direction, mode = 'frame', fps = 25, durationMS = Infinity) {
  const step = mode === '100ms' ? 100 : frameDurationMS(fps)
  return Math.min(integerMS(durationMS, Number.MAX_SAFE_INTEGER), Math.max(0, integerMS(currentMS) + Math.sign(direction || 1) * step))
}

export function visibleTimelineWindow(scrollLeft, viewportWidth, pixelsPerSecond, durationMS, overscanMS = 5000) {
  const scale = Math.max(1, Number(pixelsPerSecond) || 1)
  const start = Math.max(0, Math.floor((Math.max(0, scrollLeft) / scale) * 1000) - overscanMS)
  const visibleEnd = Math.ceil(((Math.max(0, scrollLeft) + Math.max(1, viewportWidth)) / scale) * 1000) + overscanMS
  return { start_ms: start, end_ms: Math.max(start + 1, Math.min(integerMS(durationMS, visibleEnd), visibleEnd)) }
}

export function snapTimeMS(value, boundaries = [], thresholdMS = 80) {
  const target = integerMS(value)
  let result = target
  let distance = Math.max(0, Number(thresholdMS) || 0) + 1
  for (const boundary of boundaries) {
    const candidate = integerMS(boundary)
    const currentDistance = Math.abs(candidate - target)
    if (currentDistance < distance && currentDistance <= thresholdMS) {
      result = candidate
      distance = currentDistance
    }
  }
  return result
}

export function gesturePatch(item, mode, deltaMS, boundaries = [], thresholdMS = 80) {
  const delta = Math.round(Number(deltaMS) || 0)
  const start = integerMS(item.timeline_start_ms)
  const end = Math.max(start + 1, integerMS(item.timeline_end_ms))
  const sourceIn = integerMS(item.source_in_ms)
  const sourceOut = item.source_out_ms == null ? null : integerMS(item.source_out_ms)
  if (mode === 'move') {
    let nextStart = Math.max(0, start + delta)
    let nextEnd = nextStart + (end - start)
    const adjustments = boundaries.flatMap((boundary) => {
      const value = integerMS(boundary)
      return [value - nextStart, value - nextEnd]
    }).filter(adjustment => Math.abs(adjustment) <= thresholdMS)
    const adjustment = adjustments.sort((left, right) => Math.abs(left) - Math.abs(right))[0] || 0
    nextStart = Math.max(0, nextStart + adjustment)
    nextEnd = nextStart + (end - start)
    return { timeline_start_ms: nextStart, timeline_end_ms: nextEnd }
  }
  if (mode === 'trim-start') {
    const raw = Math.min(end - 1, Math.max(0, start + delta))
    const nextStart = Math.min(end - 1, snapTimeMS(raw, boundaries, thresholdMS))
    return { timeline_start_ms: nextStart, source_in_ms: Math.max(0, sourceIn + (nextStart - start)) }
  }
  if (mode === 'trim-end') {
    const raw = Math.max(start + 1, end + delta)
    const nextEnd = Math.max(start + 1, snapTimeMS(raw, boundaries, thresholdMS))
    const result = { timeline_end_ms: nextEnd }
    if (sourceOut != null) result.source_out_ms = Math.max(sourceIn + 1, sourceOut + (nextEnd - end))
    return result
  }
  return {}
}

export function itemBoundaries(items = [], excludedId = '') {
  return items.flatMap((item) => item.timeline_item_id === excludedId
    ? [] : [integerMS(item.timeline_start_ms), integerMS(item.timeline_end_ms)])
}

export function activeItemsAt(items = [], timeMS, trackTypes = []) {
  const allowed = new Set(trackTypes)
  const time = integerMS(timeMS)
  return items.filter((item) => (!allowed.size || allowed.has(item.track_type))
    && integerMS(item.timeline_start_ms) <= time && integerMS(item.timeline_end_ms) > time)
}

export function subtitlePreviewConfig(item, timelineConfig = {}) {
  const transform = item?.transform_config || {}
  const effect = item?.effect_config || {}
  const safeArea = transform.safe_area_enabled !== false
  const position = (value, fallback) => {
    const parsed = Number(value ?? fallback)
    const normalized = Number.isFinite(parsed) ? parsed : fallback
    return safeArea ? Math.max(10, Math.min(90, normalized)) : Math.max(0, Math.min(100, normalized))
  }
  return {
    text: item?.subtitle_text || transform.text || '',
    x: position(transform.position_x_pct ?? timelineConfig.position_x_pct, 50),
    y: position(transform.position_y_pct ?? timelineConfig.position_y_pct, 84),
    fontSize: Number(transform.font_size_px ?? timelineConfig.font_size_px ?? 28),
    color: effect.color || timelineConfig.color || '#ffffff',
    background: effect.background || timelineConfig.background || 'rgba(0,0,0,.64)',
    fontWeight: effect.font_weight || timelineConfig.font_weight || 700,
    safeArea,
    style: effect.subtitle_style || timelineConfig.style || 'clean',
  }
}

export function renderOutcome(previousCurrentID, draftTimelineID, status) {
  if (status === 'succeeded') return { current_timeline_id: draftTimelineID, draft_state: 'approved' }
  if (['failed', 'timeout', 'cancelled'].includes(status)) {
    return { current_timeline_id: previousCurrentID, draft_state: 'render_failed' }
  }
  return { current_timeline_id: previousCurrentID, draft_state: 'rendering' }
}
