package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

var ErrIRMergeBlocked = errors.New("IR merge proposal is not publishable")

func (s *Store) CreateIRMergeProposal(ctx context.Context, key string, input IRMergeProposalInput) (IRMergeProposal, bool, error) {
	input.BaseFullIRRevisionID = strings.TrimSpace(input.BaseFullIRRevisionID)
	input.IncrementalIRRevisionID = strings.TrimSpace(input.IncrementalIRRevisionID)
	input.CreatedBy = strings.TrimSpace(input.CreatedBy)
	if input.BaseFullIRRevisionID == "" || input.IncrementalIRRevisionID == "" || input.BaseFullIRRevisionID == input.IncrementalIRRevisionID {
		return IRMergeProposal{}, false, ErrConflict
	}
	tx, err := s.writer.Begin(ctx)
	if err != nil {
		return IRMergeProposal{}, false, err
	}
	defer tx.Rollback(ctx)

	var existingID, existingBaseID, existingIncrementalID string
	var existingCreatedBy *string
	err = tx.QueryRow(ctx, `SELECT proposal.ir_merge_proposal_id,proposal.base_full_ir_revision_id,
		proposal.incremental_ir_revision_id,proposal.created_by
		FROM drama.ir_merge_proposals proposal WHERE proposal.idempotency_key=$1`, key).
		Scan(&existingID, &existingBaseID, &existingIncrementalID, &existingCreatedBy)
	if err == nil {
		existingCreator := ""
		if existingCreatedBy != nil {
			existingCreator = *existingCreatedBy
		}
		if existingBaseID != input.BaseFullIRRevisionID || existingIncrementalID != input.IncrementalIRRevisionID || existingCreator != input.CreatedBy {
			return IRMergeProposal{}, false, ErrConflict
		}
		proposal, loadErr := loadIRMergeProposal(ctx, tx, existingID, "", "", "")
		return proposal, false, loadErr
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return IRMergeProposal{}, false, err
	}

	var workID, targetSourceVersionID, incrementalBaseID, baseStatus, baseScope, incrementalStatus, incrementalScope, sourceStatus string
	var baseCurrent bool
	var changedChapterIDs []string
	err = tx.QueryRow(ctx, `SELECT base.work_id,incremental.source_version_id,incremental.base_ir_revision_id,
		base.status,base.revision_scope,base.is_current,incremental.status,incremental.revision_scope,source.status,
		ARRAY(SELECT jsonb_array_elements_text(incremental.changed_chapter_ids))
		FROM drama.narrative_ir_revisions base
		JOIN drama.narrative_ir_revisions incremental ON incremental.work_id=base.work_id
		JOIN drama.source_versions source ON source.source_version_id=incremental.source_version_id
		WHERE base.ir_revision_id=$1 AND incremental.ir_revision_id=$2
		FOR SHARE OF base,incremental,source`, input.BaseFullIRRevisionID, input.IncrementalIRRevisionID).
		Scan(&workID, &targetSourceVersionID, &incrementalBaseID, &baseStatus, &baseScope, &baseCurrent,
			&incrementalStatus, &incrementalScope, &sourceStatus, &changedChapterIDs)
	if errors.Is(err, pgx.ErrNoRows) {
		return IRMergeProposal{}, false, ErrNotFound
	}
	if err != nil {
		return IRMergeProposal{}, false, err
	}
	if baseStatus != "published" || baseScope != "full" || !baseCurrent || incrementalStatus != "published" ||
		incrementalScope != "incremental" || sourceStatus != "published" || incrementalBaseID != input.BaseFullIRRevisionID || len(changedChapterIDs) == 0 {
		return IRMergeProposal{}, false, ErrConflict
	}
	var priorProposalID string
	err = tx.QueryRow(ctx, `SELECT ir_merge_proposal_id FROM drama.ir_merge_proposals
		WHERE base_full_ir_revision_id=$1 AND incremental_ir_revision_id=$2`, input.BaseFullIRRevisionID, input.IncrementalIRRevisionID).
		Scan(&priorProposalID)
	if err == nil {
		proposal, loadErr := loadIRMergeProposal(ctx, tx, priorProposalID, "", "", "")
		return proposal, false, loadErr
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return IRMergeProposal{}, false, err
	}

	proposalID, _ := newPublicID("irmp_")
	createdBy := any(nil)
	if input.CreatedBy != "" {
		createdBy = input.CreatedBy
	}
	if _, err = tx.Exec(ctx, `INSERT INTO drama.ir_merge_proposals(ir_merge_proposal_id,work_id,target_source_version_id,
		base_full_ir_revision_id,incremental_ir_revision_id,changed_chapter_ids,idempotency_key,created_by)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, proposalID, workID, targetSourceVersionID,
		input.BaseFullIRRevisionID, input.IncrementalIRRevisionID, mustJSON(changedChapterIDs), key, createdBy); err != nil {
		return IRMergeProposal{}, false, mapPGConflict(err)
	}
	if err = materializeIRMergeItems(ctx, tx, proposalID, input.BaseFullIRRevisionID, input.IncrementalIRRevisionID, changedChapterIDs); err != nil {
		return IRMergeProposal{}, false, err
	}
	preview, err := calculateIRMergeImpactPreview(ctx, tx, proposalID)
	if err != nil {
		return IRMergeProposal{}, false, err
	}
	if _, err = tx.Exec(ctx, `UPDATE drama.ir_merge_proposals SET impact_preview=$2 WHERE ir_merge_proposal_id=$1`,
		proposalID, mustJSON(preview)); err != nil {
		return IRMergeProposal{}, false, err
	}
	// Deferred counter refresh runs at commit; force it now so the returned
	// resource has the authoritative ready/unresolved state.
	if _, err = tx.Exec(ctx, `SET CONSTRAINTS trg_refresh_ir_merge_proposal_counts IMMEDIATE`); err != nil {
		return IRMergeProposal{}, false, err
	}
	proposal, err := loadIRMergeProposal(ctx, tx, proposalID, "", "", "")
	if err != nil {
		return IRMergeProposal{}, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return IRMergeProposal{}, false, mapPGConflict(err)
	}
	return proposal, true, nil
}

func materializeIRMergeItems(ctx context.Context, tx pgx.Tx, proposalID, baseID, incrementalID string, changedChapterIDs []string) error {
	queries := []string{entityMergeItemsSQL, factMergeItemsSQL, relationMergeItemsSQL, storyArcMergeItemsSQL}
	for _, query := range queries {
		if _, err := tx.Exec(ctx, query, proposalID, baseID, incrementalID, changedChapterIDs); err != nil {
			return err
		}
	}
	return nil
}

const entityMergeItemsSQL = `
WITH base AS (
  SELECT revision.*,logical.entity_type,span.start_utf8_byte,span.end_utf8_byte,span.start_codepoint,span.end_codepoint,
    span.start_paragraph,span.end_paragraph,span.excerpt_hash,span.evidence_text,span.locator_version,
    COALESCE((SELECT jsonb_agg(jsonb_build_object('alias',alias.alias,'alias_type',alias.alias_type)
      ORDER BY alias.alias,alias.alias_type) FROM drama.narrative_entity_aliases alias
      WHERE alias.entity_revision_id=revision.entity_revision_id),'[]'::jsonb) aliases
  FROM drama.narrative_entity_revisions revision JOIN drama.narrative_entities logical USING(entity_id)
  JOIN drama.source_spans span ON span.source_span_id=revision.primary_source_span_id
  WHERE revision.ir_revision_id=$2
), incoming AS (
  SELECT revision.*,logical.entity_type,span.start_utf8_byte,span.end_utf8_byte,span.start_codepoint,span.end_codepoint,
    span.start_paragraph,span.end_paragraph,span.excerpt_hash,span.evidence_text,span.locator_version,
    COALESCE((SELECT jsonb_agg(jsonb_build_object('alias',alias.alias,'alias_type',alias.alias_type)
      ORDER BY alias.alias,alias.alias_type) FROM drama.narrative_entity_aliases alias
      WHERE alias.entity_revision_id=revision.entity_revision_id),'[]'::jsonb) aliases
  FROM drama.narrative_entity_revisions revision JOIN drama.narrative_entities logical USING(entity_id)
  JOIN drama.source_spans span ON span.source_span_id=revision.primary_source_span_id
  WHERE revision.ir_revision_id=$3
), candidates AS (
  SELECT incoming.entity_id incoming_entity_id,candidate.entity_id candidate_entity_id,
    candidate.entity_revision_id candidate_revision_id,candidate.canonical_name candidate_name
  FROM incoming JOIN LATERAL (
    SELECT base.* FROM base WHERE base.entity_id<>incoming.entity_id AND base.entity_type=incoming.entity_type AND (
      lower(base.canonical_name)=lower(incoming.canonical_name) OR EXISTS(
        SELECT 1 FROM jsonb_array_elements(base.aliases) old_alias
        JOIN jsonb_array_elements(incoming.aliases) new_alias
          ON lower(old_alias->>'alias')=lower(new_alias->>'alias')))
    ORDER BY CASE WHEN lower(base.canonical_name)=lower(incoming.canonical_name) THEN 0 ELSE 1 END,base.entity_id LIMIT 1
  ) candidate ON true
), differences AS (
  SELECT COALESCE(incoming.entity_id,base.entity_id) logical_id,base.entity_revision_id before_revision_id,
    incoming.entity_revision_id after_revision_id,base.entity_type before_type,incoming.entity_type after_type,
    base.canonical_name before_name,incoming.canonical_name after_name,base.attributes before_attributes,
    incoming.attributes after_attributes,base.aliases before_aliases,incoming.aliases after_aliases,
    base.confidence before_confidence,incoming.confidence after_confidence,base.validation_status before_validation,
    incoming.validation_status after_validation,base.primary_source_span_id before_span_id,
    incoming.primary_source_span_id after_span_id,
    jsonb_build_object('source_span_id',base.primary_source_span_id,'chapter_id',base.chapter_id,
      'chapter_revision_id',base.primary_chapter_revision_id,'start_utf8_byte',base.start_utf8_byte,
      'end_utf8_byte',base.end_utf8_byte,'start_codepoint',base.start_codepoint,'end_codepoint',base.end_codepoint,
      'start_paragraph',base.start_paragraph,'end_paragraph',base.end_paragraph,'excerpt_hash',base.excerpt_hash,
      'evidence_text',base.evidence_text,'locator_version',base.locator_version) before_evidence,
    jsonb_build_object('source_span_id',incoming.primary_source_span_id,'chapter_id',incoming.chapter_id,
      'chapter_revision_id',incoming.primary_chapter_revision_id,'start_utf8_byte',incoming.start_utf8_byte,
      'end_utf8_byte',incoming.end_utf8_byte,'start_codepoint',incoming.start_codepoint,'end_codepoint',incoming.end_codepoint,
      'start_paragraph',incoming.start_paragraph,'end_paragraph',incoming.end_paragraph,'excerpt_hash',incoming.excerpt_hash,
      'evidence_text',incoming.evidence_text,'locator_version',incoming.locator_version) after_evidence,
    candidate.candidate_entity_id, candidate.candidate_revision_id,candidate.candidate_name,
    (base.entity_revision_id IS NOT NULL AND incoming.entity_revision_id IS NOT NULL AND
      (base.canonical_name<>incoming.canonical_name OR base.attributes<>incoming.attributes OR base.aliases<>incoming.aliases)) semantic_changed,
    (base.entity_revision_id IS NOT NULL AND incoming.entity_revision_id IS NOT NULL AND
      (base.primary_chapter_revision_id<>incoming.primary_chapter_revision_id OR base.start_utf8_byte<>incoming.start_utf8_byte OR
       base.end_utf8_byte<>incoming.end_utf8_byte OR base.start_codepoint<>incoming.start_codepoint OR
       base.end_codepoint<>incoming.end_codepoint OR base.excerpt_hash<>incoming.excerpt_hash)) span_changed
  FROM base FULL JOIN incoming USING(entity_id)
  LEFT JOIN candidates candidate ON candidate.incoming_entity_id=incoming.entity_id
  WHERE incoming.entity_revision_id IS NOT NULL OR (base.chapter_id=ANY($4::text[]) AND incoming.entity_revision_id IS NULL)
)
INSERT INTO drama.ir_merge_proposal_items(ir_merge_item_id,ir_merge_proposal_id,item_type,logical_id,change_type,
  before_revision_id,after_revision_id,before_value,after_value,before_evidence,after_evidence,
  semantic_changed,source_span_changed,confidence,conflict_code,conflict_message,resolution,resolution_status,
  canonicalization_required,canonical_entity_id)
SELECT 'irmi_'||substr(encode(digest($1||'|entity|'||logical_id,'sha256'),'hex'),1,32),$1,'entity',logical_id,
  CASE WHEN candidate_entity_id IS NOT NULL THEN 'conflict'
    WHEN before_revision_id IS NULL THEN 'added' WHEN after_revision_id IS NULL THEN 'deleted'
    WHEN semantic_changed THEN 'modified' WHEN span_changed THEN 'relocated' ELSE 'unchanged' END,
  COALESCE(before_revision_id,candidate_revision_id),after_revision_id,
  CASE WHEN before_revision_id IS NULL AND candidate_revision_id IS NULL THEN NULL ELSE jsonb_build_object(
    'entity_type',before_type,'entity_id',COALESCE(candidate_entity_id,logical_id),
    'canonical_name',COALESCE(before_name,candidate_name),'attributes',before_attributes,'aliases',before_aliases,
    'confidence',before_confidence,'validation_status',before_validation) END,
  CASE WHEN after_revision_id IS NULL THEN NULL ELSE jsonb_build_object('entity_type',after_type,'entity_id',logical_id,
    'canonical_name',after_name,'attributes',after_attributes,'aliases',after_aliases,
    'confidence',after_confidence,'validation_status',after_validation) END,
  before_evidence,after_evidence,
  CASE WHEN before_revision_id IS NULL OR after_revision_id IS NULL THEN true ELSE semantic_changed END,span_changed,
  LEAST(COALESCE(before_confidence,1),COALESCE(after_confidence,1)),
  CASE WHEN candidate_entity_id IS NOT NULL THEN 'ENTITY_IDENTITY_CONFIRMATION_REQUIRED'
    WHEN before_revision_id IS NOT NULL AND after_revision_id IS NOT NULL AND
      (before_name<>after_name OR before_aliases<>after_aliases) THEN 'ENTITY_ALIAS_CONFIRMATION_REQUIRED'
    WHEN LEAST(COALESCE(before_confidence,1),COALESCE(after_confidence,1))<0.75 OR
      COALESCE(after_validation,before_validation) IN ('needs_review','invalid') THEN 'LOW_CONFIDENCE' END,
  CASE WHEN candidate_entity_id IS NOT NULL THEN '同名或别名重叠的人物必须人工确认是同一实体还是不同实体'
    WHEN before_revision_id IS NOT NULL AND after_revision_id IS NOT NULL AND
      (before_name<>after_name OR before_aliases<>after_aliases) THEN '实体姓名或别名发生变化，必须人工确认 canonical identity'
    WHEN LEAST(COALESCE(before_confidence,1),COALESCE(after_confidence,1))<0.75 THEN '实体置信度低于自动合并阈值' END,
  CASE WHEN candidate_entity_id IS NOT NULL OR (before_revision_id IS NOT NULL AND after_revision_id IS NOT NULL AND
      (before_name<>after_name OR before_aliases<>after_aliases)) OR
      LEAST(COALESCE(before_confidence,1),COALESCE(after_confidence,1))<0.75 OR
      COALESCE(after_validation,before_validation) IN ('needs_review','invalid') THEN NULL
    WHEN after_revision_id IS NULL THEN 'delete_invalid' ELSE 'accept_new' END,
  CASE WHEN candidate_entity_id IS NOT NULL OR (before_revision_id IS NOT NULL AND after_revision_id IS NOT NULL AND
      (before_name<>after_name OR before_aliases<>after_aliases)) OR
      LEAST(COALESCE(before_confidence,1),COALESCE(after_confidence,1))<0.75 OR
      COALESCE(after_validation,before_validation) IN ('needs_review','invalid') THEN 'unresolved' ELSE 'resolved' END,
  candidate_entity_id IS NOT NULL OR (before_revision_id IS NOT NULL AND after_revision_id IS NOT NULL AND
    (before_name<>after_name OR before_aliases<>after_aliases)),candidate_entity_id
FROM differences
WHERE before_revision_id IS NULL OR after_revision_id IS NULL OR semantic_changed OR span_changed OR candidate_entity_id IS NOT NULL;`

const factMergeItemsSQL = `
WITH base AS (
  SELECT revision.*,logical.fact_kind,span.start_utf8_byte,span.end_utf8_byte,span.start_codepoint,span.end_codepoint,
    span.start_paragraph,span.end_paragraph,span.excerpt_hash,span.evidence_text,span.locator_version,
    event.event_revision_id,event.event_type,event.summary event_summary,event.narrative_order,event.temporal_expression,
    event.importance,state.state_change_id,state.state_dimension,state.before_state,state.after_state,state.sequence_number,
    timeline.timeline_fact_id,timeline.normalized_time,timeline.timeline_order,timeline.certainty,
    occurrence.foreshadow_occurrence_id,occurrence.foreshadow_thread_id,occurrence.lifecycle_stage,occurrence.occurrence_order
  FROM drama.narrative_fact_revisions revision JOIN drama.narrative_facts logical USING(fact_id)
  JOIN drama.source_spans span ON span.source_span_id=revision.primary_source_span_id
  LEFT JOIN drama.narrative_event_revisions event USING(fact_revision_id)
  LEFT JOIN drama.character_state_changes state USING(fact_revision_id)
  LEFT JOIN drama.timeline_facts timeline USING(fact_revision_id)
  LEFT JOIN drama.foreshadow_occurrences occurrence USING(fact_revision_id)
  WHERE revision.ir_revision_id=$2
), incoming AS (
  SELECT revision.*,logical.fact_kind,span.start_utf8_byte,span.end_utf8_byte,span.start_codepoint,span.end_codepoint,
    span.start_paragraph,span.end_paragraph,span.excerpt_hash,span.evidence_text,span.locator_version,
    event.event_revision_id,event.event_type,event.summary event_summary,event.narrative_order,event.temporal_expression,
    event.importance,state.state_change_id,state.state_dimension,state.before_state,state.after_state,state.sequence_number,
    timeline.timeline_fact_id,timeline.normalized_time,timeline.timeline_order,timeline.certainty,
    occurrence.foreshadow_occurrence_id,occurrence.foreshadow_thread_id,occurrence.lifecycle_stage,occurrence.occurrence_order
  FROM drama.narrative_fact_revisions revision JOIN drama.narrative_facts logical USING(fact_id)
  JOIN drama.source_spans span ON span.source_span_id=revision.primary_source_span_id
  LEFT JOIN drama.narrative_event_revisions event USING(fact_revision_id)
  LEFT JOIN drama.character_state_changes state USING(fact_revision_id)
  LEFT JOIN drama.timeline_facts timeline USING(fact_revision_id)
  LEFT JOIN drama.foreshadow_occurrences occurrence USING(fact_revision_id)
  WHERE revision.ir_revision_id=$3
), differences AS (
  SELECT COALESCE(incoming.fact_id,base.fact_id) logical_id,base.*,incoming.fact_revision_id new_fact_revision_id,
    incoming.fact_kind new_fact_kind,incoming.chapter_id new_chapter_id,incoming.primary_chapter_revision_id new_chapter_revision_id,
    incoming.primary_source_span_id new_source_span_id,incoming.canonical_fingerprint new_fingerprint,
    incoming.confidence new_confidence,incoming.payload new_payload,incoming.validation_status new_validation,
    incoming.start_utf8_byte new_start_utf8_byte,incoming.end_utf8_byte new_end_utf8_byte,
    incoming.start_codepoint new_start_codepoint,incoming.end_codepoint new_end_codepoint,
    incoming.start_paragraph new_start_paragraph,incoming.end_paragraph new_end_paragraph,
    incoming.excerpt_hash new_excerpt_hash,incoming.evidence_text new_evidence_text,incoming.locator_version new_locator_version,
    incoming.event_revision_id new_event_revision_id,incoming.event_type new_event_type,incoming.event_summary new_event_summary,
    incoming.narrative_order new_narrative_order,incoming.temporal_expression new_temporal_expression,
    incoming.importance new_importance,incoming.state_change_id new_state_change_id,incoming.state_dimension new_state_dimension,
    incoming.before_state new_before_state,incoming.after_state new_after_state,incoming.sequence_number new_sequence_number,
    incoming.timeline_fact_id new_timeline_fact_id,incoming.normalized_time new_normalized_time,
    incoming.timeline_order new_timeline_order,incoming.certainty new_certainty,
    incoming.foreshadow_occurrence_id new_foreshadow_occurrence_id,incoming.foreshadow_thread_id new_foreshadow_thread_id,
    incoming.lifecycle_stage new_lifecycle_stage,incoming.occurrence_order new_occurrence_order
  FROM base FULL JOIN incoming USING(fact_id)
  WHERE incoming.fact_revision_id IS NOT NULL OR (base.chapter_id=ANY($4::text[]) AND incoming.fact_revision_id IS NULL)
)
INSERT INTO drama.ir_merge_proposal_items(ir_merge_item_id,ir_merge_proposal_id,item_type,logical_id,change_type,
  before_revision_id,after_revision_id,before_value,after_value,before_evidence,after_evidence,semantic_fingerprint,
  semantic_changed,source_span_changed,confidence,conflict_code,conflict_message,resolution,resolution_status)
SELECT 'irmi_'||substr(encode(digest($1||'|fact|'||logical_id,'sha256'),'hex'),1,32),$1,
  CASE COALESCE(new_fact_kind,fact_kind) WHEN 'event' THEN 'event' WHEN 'character_state' THEN 'state'
    WHEN 'foreshadowing' THEN 'foreshadow' WHEN 'relationship' THEN 'relation' ELSE 'fact' END,
  logical_id,CASE WHEN fact_revision_id IS NULL THEN 'added' WHEN new_fact_revision_id IS NULL THEN 'deleted'
    WHEN canonical_fingerprint<>new_fingerprint THEN 'modified'
    WHEN primary_chapter_revision_id<>new_chapter_revision_id OR start_utf8_byte<>new_start_utf8_byte OR
      end_utf8_byte<>new_end_utf8_byte OR start_codepoint<>new_start_codepoint OR end_codepoint<>new_end_codepoint OR
      excerpt_hash<>new_excerpt_hash THEN 'relocated' ELSE 'unchanged' END,
  fact_revision_id,new_fact_revision_id,
  CASE WHEN fact_revision_id IS NULL THEN NULL ELSE jsonb_build_object('fact_kind',fact_kind,'fact_id',logical_id,
    'payload',payload,'confidence',confidence,'validation_status',validation_status,
    'event',CASE WHEN event_revision_id IS NULL THEN NULL ELSE jsonb_build_object('event_revision_id',event_revision_id,
      'event_type',event_type,'summary',event_summary,'narrative_order',narrative_order,
      'temporal_expression',temporal_expression,'importance',importance) END,
    'state',CASE WHEN state_change_id IS NULL THEN NULL ELSE jsonb_build_object('state_change_id',state_change_id,
      'state_dimension',state_dimension,'before_state',before_state,'after_state',after_state,'sequence_number',sequence_number) END,
    'timeline',CASE WHEN timeline_fact_id IS NULL THEN NULL ELSE jsonb_build_object('timeline_fact_id',timeline_fact_id,
      'normalized_time',normalized_time,'timeline_order',timeline_order,'certainty',certainty) END,
    'foreshadow',CASE WHEN foreshadow_occurrence_id IS NULL THEN NULL ELSE jsonb_build_object(
      'foreshadow_occurrence_id',foreshadow_occurrence_id,'foreshadow_thread_id',foreshadow_thread_id,
      'lifecycle_stage',lifecycle_stage,'occurrence_order',occurrence_order) END) END,
  CASE WHEN new_fact_revision_id IS NULL THEN NULL ELSE jsonb_build_object('fact_kind',new_fact_kind,'fact_id',logical_id,
    'payload',new_payload,'confidence',new_confidence,'validation_status',new_validation,
    'event',CASE WHEN new_event_revision_id IS NULL THEN NULL ELSE jsonb_build_object('event_revision_id',new_event_revision_id,
      'event_type',new_event_type,'summary',new_event_summary,'narrative_order',new_narrative_order,
      'temporal_expression',new_temporal_expression,'importance',new_importance) END,
    'state',CASE WHEN new_state_change_id IS NULL THEN NULL ELSE jsonb_build_object('state_change_id',new_state_change_id,
      'state_dimension',new_state_dimension,'before_state',new_before_state,'after_state',new_after_state,
      'sequence_number',new_sequence_number) END,
    'timeline',CASE WHEN new_timeline_fact_id IS NULL THEN NULL ELSE jsonb_build_object('timeline_fact_id',new_timeline_fact_id,
      'normalized_time',new_normalized_time,'timeline_order',new_timeline_order,'certainty',new_certainty) END,
    'foreshadow',CASE WHEN new_foreshadow_occurrence_id IS NULL THEN NULL ELSE jsonb_build_object(
      'foreshadow_occurrence_id',new_foreshadow_occurrence_id,'foreshadow_thread_id',new_foreshadow_thread_id,
      'lifecycle_stage',new_lifecycle_stage,'occurrence_order',new_occurrence_order) END) END,
  CASE WHEN fact_revision_id IS NULL THEN NULL ELSE jsonb_build_object('source_span_id',primary_source_span_id,
    'chapter_id',chapter_id,'chapter_revision_id',primary_chapter_revision_id,'start_utf8_byte',start_utf8_byte,
    'end_utf8_byte',end_utf8_byte,'start_codepoint',start_codepoint,'end_codepoint',end_codepoint,
    'start_paragraph',start_paragraph,'end_paragraph',end_paragraph,'excerpt_hash',excerpt_hash,
    'evidence_text',evidence_text,'locator_version',locator_version) END,
  CASE WHEN new_fact_revision_id IS NULL THEN NULL ELSE jsonb_build_object('source_span_id',new_source_span_id,
    'chapter_id',new_chapter_id,'chapter_revision_id',new_chapter_revision_id,'start_utf8_byte',new_start_utf8_byte,
    'end_utf8_byte',new_end_utf8_byte,'start_codepoint',new_start_codepoint,'end_codepoint',new_end_codepoint,
    'start_paragraph',new_start_paragraph,'end_paragraph',new_end_paragraph,'excerpt_hash',new_excerpt_hash,
    'evidence_text',new_evidence_text,'locator_version',new_locator_version) END,
  COALESCE(new_fingerprint,canonical_fingerprint),
  fact_revision_id IS NULL OR new_fact_revision_id IS NULL OR canonical_fingerprint<>new_fingerprint,
  fact_revision_id IS NOT NULL AND new_fact_revision_id IS NOT NULL AND
    (primary_chapter_revision_id<>new_chapter_revision_id OR start_utf8_byte<>new_start_utf8_byte OR
      end_utf8_byte<>new_end_utf8_byte OR start_codepoint<>new_start_codepoint OR end_codepoint<>new_end_codepoint OR
      excerpt_hash<>new_excerpt_hash),
  LEAST(COALESCE(confidence,1),COALESCE(new_confidence,1)),
  CASE WHEN LEAST(COALESCE(confidence,1),COALESCE(new_confidence,1))<0.75 OR
    COALESCE(new_validation,validation_status) IN ('needs_review','invalid','conflicting') THEN 'FACT_CONFLICT' END,
  CASE WHEN LEAST(COALESCE(confidence,1),COALESCE(new_confidence,1))<0.75 THEN '事实置信度低于自动合并阈值'
    WHEN COALESCE(new_validation,validation_status) IN ('needs_review','invalid','conflicting') THEN '事实验证状态要求人工裁决' END,
  CASE WHEN LEAST(COALESCE(confidence,1),COALESCE(new_confidence,1))<0.75 OR
    COALESCE(new_validation,validation_status) IN ('needs_review','invalid','conflicting') THEN NULL
    WHEN new_fact_revision_id IS NULL THEN 'delete_invalid' ELSE 'accept_new' END,
  CASE WHEN LEAST(COALESCE(confidence,1),COALESCE(new_confidence,1))<0.75 OR
    COALESCE(new_validation,validation_status) IN ('needs_review','invalid','conflicting') THEN 'unresolved' ELSE 'resolved' END
FROM differences
WHERE fact_revision_id IS NULL OR new_fact_revision_id IS NULL OR canonical_fingerprint<>new_fingerprint OR
  primary_chapter_revision_id<>new_chapter_revision_id OR start_utf8_byte<>new_start_utf8_byte OR
  end_utf8_byte<>new_end_utf8_byte OR start_codepoint<>new_start_codepoint OR end_codepoint<>new_end_codepoint OR
  excerpt_hash<>new_excerpt_hash;`

const relationMergeItemsSQL = `
WITH base AS (
  SELECT relation.*,from_fact.fact_id from_fact_id,to_fact.fact_id to_fact_id,span.chapter_id,span.chapter_revision_id,
    span.start_utf8_byte,span.end_utf8_byte,span.start_codepoint,span.end_codepoint,span.excerpt_hash,span.evidence_text,
    from_fact.fact_id||'|'||to_fact.fact_id||'|'||relation.relation_type logical_id
  FROM drama.event_relations relation
  JOIN drama.narrative_event_revisions from_event ON from_event.event_revision_id=relation.from_event_revision_id
  JOIN drama.narrative_fact_revisions from_fact ON from_fact.fact_revision_id=from_event.fact_revision_id
  JOIN drama.narrative_event_revisions to_event ON to_event.event_revision_id=relation.to_event_revision_id
  JOIN drama.narrative_fact_revisions to_fact ON to_fact.fact_revision_id=to_event.fact_revision_id
  JOIN drama.source_spans span USING(source_span_id) WHERE relation.ir_revision_id=$2
), incoming AS (
  SELECT relation.*,from_fact.fact_id from_fact_id,to_fact.fact_id to_fact_id,span.chapter_id,span.chapter_revision_id,
    span.start_utf8_byte,span.end_utf8_byte,span.start_codepoint,span.end_codepoint,span.excerpt_hash,span.evidence_text,
    from_fact.fact_id||'|'||to_fact.fact_id||'|'||relation.relation_type logical_id
  FROM drama.event_relations relation
  JOIN drama.narrative_event_revisions from_event ON from_event.event_revision_id=relation.from_event_revision_id
  JOIN drama.narrative_fact_revisions from_fact ON from_fact.fact_revision_id=from_event.fact_revision_id
  JOIN drama.narrative_event_revisions to_event ON to_event.event_revision_id=relation.to_event_revision_id
  JOIN drama.narrative_fact_revisions to_fact ON to_fact.fact_revision_id=to_event.fact_revision_id
  JOIN drama.source_spans span USING(source_span_id) WHERE relation.ir_revision_id=$3
), differences AS (
  SELECT COALESCE(incoming.logical_id,base.logical_id) logical_key,base.event_relation_id before_id,
    incoming.event_relation_id after_id,base.from_fact_id before_from,base.to_fact_id before_to,
    incoming.from_fact_id after_from,incoming.to_fact_id after_to,base.relation_type before_type,
    incoming.relation_type after_type,base.confidence before_confidence,incoming.confidence after_confidence,
    base.source_span_id before_span,incoming.source_span_id after_span,
    jsonb_build_object('source_span_id',base.source_span_id,'chapter_id',base.chapter_id,
      'chapter_revision_id',base.chapter_revision_id,'start_utf8_byte',base.start_utf8_byte,'end_utf8_byte',base.end_utf8_byte,
      'start_codepoint',base.start_codepoint,'end_codepoint',base.end_codepoint,'excerpt_hash',base.excerpt_hash,
      'evidence_text',base.evidence_text) before_evidence,
    jsonb_build_object('source_span_id',incoming.source_span_id,'chapter_id',incoming.chapter_id,
      'chapter_revision_id',incoming.chapter_revision_id,'start_utf8_byte',incoming.start_utf8_byte,'end_utf8_byte',incoming.end_utf8_byte,
      'start_codepoint',incoming.start_codepoint,'end_codepoint',incoming.end_codepoint,'excerpt_hash',incoming.excerpt_hash,
      'evidence_text',incoming.evidence_text) after_evidence,
    base.chapter_id before_chapter,incoming.chapter_id after_chapter
  FROM base FULL JOIN incoming USING(logical_id)
  WHERE incoming.event_relation_id IS NOT NULL OR (base.chapter_id=ANY($4::text[]) AND incoming.event_relation_id IS NULL)
)
INSERT INTO drama.ir_merge_proposal_items(ir_merge_item_id,ir_merge_proposal_id,item_type,logical_id,change_type,
  before_revision_id,after_revision_id,before_value,after_value,before_evidence,after_evidence,semantic_changed,
  source_span_changed,confidence,conflict_code,conflict_message,resolution,resolution_status)
SELECT 'irmi_'||substr(encode(digest($1||'|causal-relation|'||logical_key,'sha256'),'hex'),1,32),$1,'relation',
  'causal:'||logical_key,CASE WHEN before_id IS NULL THEN 'added' WHEN after_id IS NULL THEN 'deleted'
    WHEN before_span<>after_span THEN 'relocated' ELSE 'unchanged' END,before_id,after_id,
  CASE WHEN before_id IS NULL THEN NULL ELSE jsonb_build_object('relation_kind','causal','from_fact_id',before_from,
    'to_fact_id',before_to,'relation_type',before_type,'confidence',before_confidence) END,
  CASE WHEN after_id IS NULL THEN NULL ELSE jsonb_build_object('relation_kind','causal','from_fact_id',after_from,
    'to_fact_id',after_to,'relation_type',after_type,'confidence',after_confidence) END,
  before_evidence,after_evidence,before_id IS NULL OR after_id IS NULL,before_id IS NOT NULL AND after_id IS NOT NULL AND before_span<>after_span,
  LEAST(COALESCE(before_confidence,1),COALESCE(after_confidence,1)),
  CASE WHEN LEAST(COALESCE(before_confidence,1),COALESCE(after_confidence,1))<0.75 THEN 'RELATION_LOW_CONFIDENCE' END,
  CASE WHEN LEAST(COALESCE(before_confidence,1),COALESCE(after_confidence,1))<0.75 THEN '因果关系置信度低于自动合并阈值' END,
  CASE WHEN LEAST(COALESCE(before_confidence,1),COALESCE(after_confidence,1))<0.75 THEN NULL
    WHEN after_id IS NULL THEN 'delete_invalid' ELSE 'accept_new' END,
  CASE WHEN LEAST(COALESCE(before_confidence,1),COALESCE(after_confidence,1))<0.75 THEN 'unresolved' ELSE 'resolved' END
FROM differences WHERE before_id IS NULL OR after_id IS NULL OR before_span<>after_span;`

const storyArcMergeItemsSQL = `
WITH base AS (
  SELECT arc.*,span.start_utf8_byte,span.end_utf8_byte,span.start_codepoint,span.end_codepoint,span.excerpt_hash,span.evidence_text
  FROM drama.story_arc_revisions arc JOIN drama.source_spans span ON span.source_span_id=arc.primary_source_span_id
  WHERE arc.ir_revision_id=$2
), incoming AS (
  SELECT arc.*,span.start_utf8_byte,span.end_utf8_byte,span.start_codepoint,span.end_codepoint,span.excerpt_hash,span.evidence_text
  FROM drama.story_arc_revisions arc JOIN drama.source_spans span ON span.source_span_id=arc.primary_source_span_id
  WHERE arc.ir_revision_id=$3
), differences AS (
  SELECT COALESCE(incoming.story_arc_id,base.story_arc_id) logical_id,base.story_arc_revision_id before_id,
    incoming.story_arc_revision_id after_id,base.title before_title,incoming.title after_title,
    base.summary before_summary,incoming.summary after_summary,base.arc_type before_type,incoming.arc_type after_type,
    base.confidence before_confidence,incoming.confidence after_confidence,base.primary_source_span_id before_span,
    incoming.primary_source_span_id after_span,base.chapter_id before_chapter,incoming.chapter_id after_chapter,
    jsonb_build_object('source_span_id',base.primary_source_span_id,'chapter_id',base.chapter_id,
      'chapter_revision_id',base.primary_chapter_revision_id,'start_utf8_byte',base.start_utf8_byte,
      'end_utf8_byte',base.end_utf8_byte,'start_codepoint',base.start_codepoint,'end_codepoint',base.end_codepoint,
      'excerpt_hash',base.excerpt_hash,'evidence_text',base.evidence_text) before_evidence,
    jsonb_build_object('source_span_id',incoming.primary_source_span_id,'chapter_id',incoming.chapter_id,
      'chapter_revision_id',incoming.primary_chapter_revision_id,'start_utf8_byte',incoming.start_utf8_byte,
      'end_utf8_byte',incoming.end_utf8_byte,'start_codepoint',incoming.start_codepoint,'end_codepoint',incoming.end_codepoint,
      'excerpt_hash',incoming.excerpt_hash,'evidence_text',incoming.evidence_text) after_evidence
  FROM base FULL JOIN incoming USING(story_arc_id)
  WHERE incoming.story_arc_revision_id IS NOT NULL OR (base.chapter_id=ANY($4::text[]) AND incoming.story_arc_revision_id IS NULL)
)
INSERT INTO drama.ir_merge_proposal_items(ir_merge_item_id,ir_merge_proposal_id,item_type,logical_id,change_type,
  before_revision_id,after_revision_id,before_value,after_value,before_evidence,after_evidence,semantic_changed,
  source_span_changed,confidence,conflict_code,conflict_message,resolution,resolution_status)
SELECT 'irmi_'||substr(encode(digest($1||'|story-arc|'||logical_id,'sha256'),'hex'),1,32),$1,'story_arc',logical_id,
  CASE WHEN before_id IS NULL THEN 'added' WHEN after_id IS NULL THEN 'deleted'
    WHEN before_title<>after_title OR before_summary<>after_summary OR before_type<>after_type THEN 'modified'
    WHEN before_span<>after_span THEN 'relocated' ELSE 'unchanged' END,before_id,after_id,
  CASE WHEN before_id IS NULL THEN NULL ELSE jsonb_build_object('title',before_title,'summary',before_summary,
    'arc_type',before_type,'confidence',before_confidence) END,
  CASE WHEN after_id IS NULL THEN NULL ELSE jsonb_build_object('title',after_title,'summary',after_summary,
    'arc_type',after_type,'confidence',after_confidence) END,before_evidence,after_evidence,
  before_id IS NULL OR after_id IS NULL OR before_title<>after_title OR before_summary<>after_summary OR before_type<>after_type,
  before_id IS NOT NULL AND after_id IS NOT NULL AND before_span<>after_span,
  LEAST(COALESCE(before_confidence,1),COALESCE(after_confidence,1)),
  CASE WHEN LEAST(COALESCE(before_confidence,1),COALESCE(after_confidence,1))<0.75 THEN 'STORY_ARC_LOW_CONFIDENCE' END,
  CASE WHEN LEAST(COALESCE(before_confidence,1),COALESCE(after_confidence,1))<0.75 THEN '故事弧置信度低于自动合并阈值' END,
  CASE WHEN LEAST(COALESCE(before_confidence,1),COALESCE(after_confidence,1))<0.75 THEN NULL
    WHEN after_id IS NULL THEN 'delete_invalid' ELSE 'accept_new' END,
  CASE WHEN LEAST(COALESCE(before_confidence,1),COALESCE(after_confidence,1))<0.75 THEN 'unresolved' ELSE 'resolved' END
FROM differences WHERE before_id IS NULL OR after_id IS NULL OR before_title<>after_title OR before_summary<>after_summary OR
  before_type<>after_type OR before_span<>after_span;`

func calculateIRMergeImpactPreview(ctx context.Context, tx pgx.Tx, proposalID string) (IRMergeImpactPreview, error) {
	preview := IRMergeImpactPreview{AffectedArtifacts: []IRMergeImpactArtifact{}, AutoRebuild: false}
	if err := tx.QueryRow(ctx, `SELECT count(*) FILTER(WHERE semantic_changed),count(*) FILTER(WHERE NOT semantic_changed)
		FROM drama.ir_merge_proposal_items WHERE ir_merge_proposal_id=$1`, proposalID).
		Scan(&preview.SemanticChangeCount, &preview.RelocationOnlyCount); err != nil {
		return preview, err
	}
	rows, err := tx.Query(ctx, `WITH RECURSIVE direct AS (
		SELECT artifact.artifact_id,0 depth,jsonb_build_array(artifact.artifact_id) path
		FROM drama.artifacts artifact WHERE EXISTS(
		  SELECT 1 FROM drama.ir_merge_proposal_items item
		  LEFT JOIN drama.artifact_source_evidence evidence ON evidence.artifact_id=artifact.artifact_id
		  LEFT JOIN drama.episode_event_assignments assignment ON assignment.event_revision_id=item.before_value->'event'->>'event_revision_id'
		  LEFT JOIN drama.adaptation_episode_plans episode ON episode.adaptation_episode_plan_id=assignment.adaptation_episode_plan_id
		  WHERE item.ir_merge_proposal_id=$1 AND item.semantic_changed AND (
		    evidence.fact_revision_id=item.before_revision_id OR artifact.native_entity_id IN (
		      episode.adaptation_episode_plan_id,episode.adaptation_plan_id) OR
		    (item.item_type='story_arc' AND artifact.native_entity_id=item.before_revision_id)))
	), affected AS (
		SELECT * FROM direct UNION ALL
		SELECT downstream.artifact_id,affected.depth+1,affected.path||to_jsonb(downstream.artifact_id)
		FROM affected JOIN drama.artifact_dependencies dependency ON dependency.upstream_artifact_id=affected.artifact_id
		JOIN drama.artifacts downstream ON downstream.artifact_id=dependency.downstream_artifact_id
		WHERE affected.depth<30 AND NOT affected.path ? downstream.artifact_id
	), collapsed AS (SELECT artifact_id,min(depth) depth FROM affected GROUP BY artifact_id)
	SELECT artifact.artifact_id,artifact.artifact_type,artifact.native_entity_id,COALESCE(artifact.project_id,''),collapsed.depth
	FROM collapsed JOIN drama.artifacts artifact USING(artifact_id) ORDER BY collapsed.depth,artifact.artifact_type,artifact.artifact_id`, proposalID)
	if err != nil {
		return preview, err
	}
	defer rows.Close()
	for rows.Next() {
		var item IRMergeImpactArtifact
		if err := rows.Scan(&item.ArtifactID, &item.ArtifactType, &item.NativeEntityID, &item.ProjectID, &item.Depth); err != nil {
			return preview, err
		}
		preview.AffectedArtifacts = append(preview.AffectedArtifacts, item)
	}
	return preview, rows.Err()
}

func (s *Store) GetIRMergeProposal(ctx context.Context, proposalID, itemType, changeType, resolutionStatus string) (IRMergeProposal, error) {
	return loadIRMergeProposal(ctx, s.pool, proposalID, itemType, changeType, resolutionStatus)
}

type queryRower interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func loadIRMergeProposal(ctx context.Context, db queryRower, proposalID, itemType, changeType, resolutionStatus string) (IRMergeProposal, error) {
	var proposal IRMergeProposal
	var changedRaw, previewRaw []byte
	err := db.QueryRow(ctx, `SELECT ir_merge_proposal_id,work_id,target_source_version_id,base_full_ir_revision_id,
		incremental_ir_revision_id,published_full_ir_revision_id,status,resource_revision,confidence,conflict_count,
		unresolved_count,changed_chapter_ids,impact_preview,created_by,published_by,published_at,created_at,updated_at
		FROM drama.ir_merge_proposals WHERE ir_merge_proposal_id=$1`, proposalID).
		Scan(&proposal.IRMergeProposalID, &proposal.WorkID, &proposal.TargetSourceVersionID, &proposal.BaseFullIRRevisionID,
			&proposal.IncrementalIRRevisionID, &proposal.PublishedFullIRRevisionID, &proposal.Status, &proposal.ResourceRevision,
			&proposal.Confidence, &proposal.ConflictCount, &proposal.UnresolvedCount, &changedRaw, &previewRaw,
			&proposal.CreatedBy, &proposal.PublishedBy, &proposal.PublishedAt, &proposal.CreatedAt, &proposal.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return proposal, ErrNotFound
	}
	if err != nil {
		return proposal, err
	}
	_ = json.Unmarshal(changedRaw, &proposal.ChangedChapterIDs)
	_ = json.Unmarshal(previewRaw, &proposal.ImpactPreview)
	if proposal.ImpactPreview.AffectedArtifacts == nil {
		proposal.ImpactPreview.AffectedArtifacts = []IRMergeImpactArtifact{}
	}
	rows, err := db.Query(ctx, `SELECT ir_merge_item_id,item_type,logical_id,change_type,before_revision_id,after_revision_id,
		COALESCE(before_value,'null'::jsonb),COALESCE(after_value,'null'::jsonb),COALESCE(before_evidence,'null'::jsonb),
		COALESCE(after_evidence,'null'::jsonb),semantic_fingerprint,semantic_changed,source_span_changed,confidence,
		conflict_code,conflict_message,resolution,COALESCE(resolved_value,'null'::jsonb),resolution_status,
		canonicalization_required,canonicalization_confirmed,canonicalization_decision,canonical_entity_id,
		resolution_note,resolved_by,resolved_at
		FROM drama.ir_merge_proposal_items WHERE ir_merge_proposal_id=$1
		AND ($2='' OR item_type=$2) AND ($3='' OR change_type=$3) AND ($4='' OR resolution_status=$4)
		ORDER BY CASE item_type WHEN 'entity' THEN 1 WHEN 'fact' THEN 2 WHEN 'event' THEN 3 WHEN 'relation' THEN 4
		  WHEN 'state' THEN 5 WHEN 'foreshadow' THEN 6 ELSE 7 END,logical_id`, proposalID, itemType, changeType, resolutionStatus)
	if err != nil {
		return proposal, err
	}
	defer rows.Close()
	proposal.Items = []IRMergeProposalItem{}
	for rows.Next() {
		var item IRMergeProposalItem
		if err := rows.Scan(&item.IRMergeItemID, &item.ItemType, &item.LogicalID, &item.ChangeType,
			&item.BeforeRevisionID, &item.AfterRevisionID, &item.BeforeValue, &item.AfterValue,
			&item.BeforeEvidence, &item.AfterEvidence, &item.SemanticFingerprint, &item.SemanticChanged,
			&item.SourceSpanChanged, &item.Confidence, &item.ConflictCode, &item.ConflictMessage,
			&item.Resolution, &item.ResolvedValue, &item.ResolutionStatus, &item.CanonicalizationRequired,
			&item.CanonicalizationConfirmed, &item.CanonicalizationDecision, &item.CanonicalEntityID,
			&item.ResolutionNote, &item.ResolvedBy, &item.ResolvedAt); err != nil {
			return proposal, err
		}
		proposal.Items = append(proposal.Items, item)
	}
	return proposal, rows.Err()
}

func (s *Store) ResolveIRMergeItem(ctx context.Context, proposalID, itemID string, input IRMergeItemResolutionInput) (IRMergeProposalItem, error) {
	allowed := map[string]bool{"accept_new": true, "keep_old": true, "merge": true, "manual_edit": true, "delete_invalid": true}
	input.Resolution = strings.TrimSpace(input.Resolution)
	if !allowed[input.Resolution] {
		return IRMergeProposalItem{}, ErrConflict
	}
	if len(input.ResolvedValue) > 0 && !json.Valid(input.ResolvedValue) {
		return IRMergeProposalItem{}, ErrConflict
	}
	tx, err := s.writer.Begin(ctx)
	if err != nil {
		return IRMergeProposalItem{}, err
	}
	defer tx.Rollback(ctx)
	var status string
	var beforeID, afterID *string
	var canonicalRequired bool
	err = tx.QueryRow(ctx, `SELECT proposal.status,item.before_revision_id,item.after_revision_id,item.canonicalization_required
		FROM drama.ir_merge_proposals proposal JOIN drama.ir_merge_proposal_items item USING(ir_merge_proposal_id)
		WHERE proposal.ir_merge_proposal_id=$1 AND item.ir_merge_item_id=$2 FOR UPDATE OF proposal,item`, proposalID, itemID).
		Scan(&status, &beforeID, &afterID, &canonicalRequired)
	if errors.Is(err, pgx.ErrNoRows) {
		return IRMergeProposalItem{}, ErrNotFound
	}
	if err != nil {
		return IRMergeProposalItem{}, err
	}
	if status != "draft" && status != "ready" {
		return IRMergeProposalItem{}, ErrConflict
	}
	if input.Resolution == "accept_new" && afterID == nil || input.Resolution == "keep_old" && beforeID == nil {
		return IRMergeProposalItem{}, ErrConflict
	}
	resolvedStatus := "resolved"
	if input.Resolution == "merge" && len(input.ResolvedValue) == 0 {
		return IRMergeProposalItem{}, ErrConflict
	}
	if input.Resolution == "manual_edit" && len(input.ResolvedValue) == 0 {
		resolvedStatus = "needs_manual_edit"
	}
	canonicalDecision, canonicalID := any(nil), any(nil)
	canonicalConfirmed := false
	if canonicalRequired {
		if !input.CanonicalizationConfirmed || (input.CanonicalizationDecision != "same_entity" && input.CanonicalizationDecision != "distinct_entities") {
			return IRMergeProposalItem{}, fmt.Errorf("%w: canonicalization confirmation is required", ErrIRMergeBlocked)
		}
		if input.CanonicalizationDecision == "same_entity" && strings.TrimSpace(input.CanonicalEntityID) == "" {
			return IRMergeProposalItem{}, fmt.Errorf("%w: canonical_entity_id is required", ErrIRMergeBlocked)
		}
		canonicalConfirmed = true
		canonicalDecision = input.CanonicalizationDecision
		if input.CanonicalizationDecision == "same_entity" {
			canonicalID = strings.TrimSpace(input.CanonicalEntityID)
		} else if afterID != nil {
			var afterEntityID string
			if err := tx.QueryRow(ctx, `SELECT entity_id FROM drama.narrative_entity_revisions WHERE entity_revision_id=$1`, *afterID).Scan(&afterEntityID); err != nil {
				return IRMergeProposalItem{}, err
			}
			canonicalID = afterEntityID
		}
	}
	resolvedValue := any(nil)
	if len(input.ResolvedValue) > 0 {
		resolvedValue = input.ResolvedValue
	}
	resolvedBy, note := any(nil), any(nil)
	if strings.TrimSpace(input.ResolvedBy) != "" {
		resolvedBy = strings.TrimSpace(input.ResolvedBy)
	}
	if strings.TrimSpace(input.ResolutionNote) != "" {
		note = strings.TrimSpace(input.ResolutionNote)
	}
	if _, err = tx.Exec(ctx, `UPDATE drama.ir_merge_proposal_items SET resolution=$3,resolved_value=$4,
		resolution_status=$5,canonicalization_confirmed=$6,canonicalization_decision=$7,canonical_entity_id=$8,
		resolution_note=$9,resolved_by=$10,resolved_at=CURRENT_TIMESTAMP
		WHERE ir_merge_proposal_id=$1 AND ir_merge_item_id=$2`, proposalID, itemID, input.Resolution, resolvedValue,
		resolvedStatus, canonicalConfirmed, canonicalDecision, canonicalID, note, resolvedBy); err != nil {
		return IRMergeProposalItem{}, err
	}
	if _, err = tx.Exec(ctx, `SET CONSTRAINTS trg_refresh_ir_merge_proposal_counts IMMEDIATE`); err != nil {
		return IRMergeProposalItem{}, err
	}
	proposal, err := loadIRMergeProposal(ctx, tx, proposalID, "", "", "")
	if err != nil {
		return IRMergeProposalItem{}, err
	}
	var result IRMergeProposalItem
	for _, item := range proposal.Items {
		if item.IRMergeItemID == itemID {
			result = item
			break
		}
	}
	if result.IRMergeItemID == "" {
		return IRMergeProposalItem{}, ErrNotFound
	}
	if err = tx.Commit(ctx); err != nil {
		return IRMergeProposalItem{}, err
	}
	return result, nil
}

func (s *Store) PublishIRMergeProposal(ctx context.Context, proposalID, key string, input PublishIRMergeInput) (PublishIRMergeResult, error) {
	if !input.Confirmed {
		return PublishIRMergeResult{}, ErrIRMergeBlocked
	}
	tx, err := s.writer.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return PublishIRMergeResult{}, err
	}
	defer tx.Rollback(ctx)

	var workID, targetSourceID, baseIRID, incrementalIRID, status string
	var publishedIRID, publishKey *string
	var unresolved int
	var changedChapterIDs []string
	err = tx.QueryRow(ctx, `SELECT work_id,target_source_version_id,base_full_ir_revision_id,incremental_ir_revision_id,
		status,unresolved_count,published_full_ir_revision_id,publish_idempotency_key,
		ARRAY(SELECT jsonb_array_elements_text(changed_chapter_ids))
		FROM drama.ir_merge_proposals WHERE ir_merge_proposal_id=$1 FOR UPDATE`, proposalID).
		Scan(&workID, &targetSourceID, &baseIRID, &incrementalIRID, &status, &unresolved, &publishedIRID, &publishKey, &changedChapterIDs)
	if errors.Is(err, pgx.ErrNoRows) {
		return PublishIRMergeResult{}, ErrNotFound
	}
	if err != nil {
		return PublishIRMergeResult{}, err
	}
	if status == "published" {
		if publishKey == nil || *publishKey != key || publishedIRID == nil {
			return PublishIRMergeResult{}, ErrConflict
		}
		return loadPublishedIRMergeResult(ctx, tx, proposalID, *publishedIRID)
	}
	var blocking int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM drama.ir_merge_proposal_items WHERE ir_merge_proposal_id=$1 AND (
		resolution_status<>'resolved' OR resolution IS NULL OR
		(canonicalization_required AND NOT canonicalization_confirmed))`, proposalID).Scan(&blocking); err != nil {
		return PublishIRMergeResult{}, err
	}
	if status != "ready" || unresolved != 0 || blocking != 0 {
		return PublishIRMergeResult{}, fmt.Errorf("%w: %d unresolved conflict(s)", ErrIRMergeBlocked, blocking)
	}
	var sourceStatus, baseStatus, baseScope, incrementalStatus, incrementalScope, incrementalBase string
	var baseCurrent bool
	err = tx.QueryRow(ctx, `SELECT source.status,base.status,base.revision_scope,base.is_current,incremental.status,
		incremental.revision_scope,incremental.base_ir_revision_id
		FROM drama.source_versions source
		JOIN drama.narrative_ir_revisions base ON base.ir_revision_id=$2
		JOIN drama.narrative_ir_revisions incremental ON incremental.ir_revision_id=$3
		WHERE source.source_version_id=$1 FOR SHARE OF source,base,incremental`, targetSourceID, baseIRID, incrementalIRID).
		Scan(&sourceStatus, &baseStatus, &baseScope, &baseCurrent, &incrementalStatus, &incrementalScope, &incrementalBase)
	if err != nil {
		return PublishIRMergeResult{}, err
	}
	if sourceStatus != "published" || baseStatus != "published" || baseScope != "full" || !baseCurrent ||
		incrementalStatus != "published" || incrementalScope != "incremental" || incrementalBase != baseIRID {
		return PublishIRMergeResult{}, ErrConflict
	}

	fullIRID, _ := newPublicID("ir_")
	operationID, _ := newPublicID("op_")
	traceID, _ := newPublicID("tr_")
	var revisionNumber int
	if err = tx.QueryRow(ctx, `SELECT COALESCE(max(revision_number),0)+1 FROM drama.narrative_ir_revisions
		WHERE source_version_id=$1`, targetSourceID).Scan(&revisionNumber); err != nil {
		return PublishIRMergeResult{}, err
	}
	inputHash := hashText(baseIRID + "|" + incrementalIRID + "|" + proposalID)
	checkpoint := mustJSON(map[string]any{"stage": "copying_full_snapshot", "merge_proposal_id": proposalID,
		"base_full_ir_revision_id": baseIRID, "incremental_ir_revision_id": incrementalIRID})
	if _, err = tx.Exec(ctx, `INSERT INTO drama.operations(operation_id,trace_id,operation_type,target_type,target_id,status,
		idempotency_key,input_hash,checkpoint_stage,checkpoint_data)
		VALUES($1,$2,'ir_merge','ir_revision',$3,'pending',$4,$5,'copying_full_snapshot',$6)`,
		operationID, traceID, fullIRID, key, inputHash, checkpoint); err != nil {
		return PublishIRMergeResult{}, mapPGConflict(err)
	}
	var schemaVersion, extractorVersion string
	if err = tx.QueryRow(ctx, `SELECT schema_version,extractor_version FROM drama.narrative_ir_revisions WHERE ir_revision_id=$1`, incrementalIRID).
		Scan(&schemaVersion, &extractorVersion); err != nil {
		return PublishIRMergeResult{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO drama.narrative_ir_revisions(ir_revision_id,operation_id,operation_type,work_id,
		source_version_id,revision_number,schema_version,extractor_version,status,input_hash,idempotency_key,
		validation_summary,revision_scope,changed_chapter_ids,merge_proposal_id)
		VALUES($1,$2,'ir_merge',$3,$4,$5,$6,$7,'staging',$8,$9,$10,'full',$11,$12)`, fullIRID,
		operationID, workID, targetSourceID, revisionNumber, schemaVersion, extractorVersion, inputHash, key,
		mustJSON(map[string]any{"merge_proposal_id": proposalID, "state": "copying_full_snapshot"}),
		mustJSON(changedChapterIDs), proposalID); err != nil {
		return PublishIRMergeResult{}, mapPGConflict(err)
	}

	if err = prepareIRMergeSelections(ctx, tx, proposalID, baseIRID, incrementalIRID, fullIRID); err != nil {
		return PublishIRMergeResult{}, err
	}
	if err = prepareIRMergeSpanMap(ctx, tx, proposalID, baseIRID, incrementalIRID, targetSourceID, workID); err != nil {
		return PublishIRMergeResult{}, err
	}
	if err = copyIRMergeSnapshot(ctx, tx, proposalID, fullIRID, targetSourceID, workID); err != nil {
		return PublishIRMergeResult{}, err
	}
	outputHash, err := calculateFullIRHash(ctx, tx, fullIRID)
	if err != nil {
		return PublishIRMergeResult{}, err
	}
	publishedBy := any(nil)
	if strings.TrimSpace(input.PublishedBy) != "" {
		publishedBy = strings.TrimSpace(input.PublishedBy)
	}
	validation := mustJSON(map[string]any{"valid": true, "merge_proposal_id": proposalID, "atomic_snapshot": true,
		"canonicalization_confirmed": true, "source_span_locator": "utf8-codepoint-v1"})
	if _, err = tx.Exec(ctx, `UPDATE drama.narrative_ir_revisions SET status='published',is_current=true,output_hash=$2,
		validation_summary=$3,published_at=CURRENT_TIMESTAMP WHERE ir_revision_id=$1`, fullIRID, outputHash, validation); err != nil {
		return PublishIRMergeResult{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE drama.operations SET status='completed',checkpoint_stage='finished',
		checkpoint_data=checkpoint_data||jsonb_build_object('output_hash',$2::text,'atomic_snapshot',true),
		result_type='ir_revision',result_id=$1,completed_at=CURRENT_TIMESTAMP WHERE operation_id=$3`,
		fullIRID, outputHash, operationID); err != nil {
		return PublishIRMergeResult{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE drama.ir_merge_proposals SET status='published',published_full_ir_revision_id=$2,
		publish_idempotency_key=$3,published_by=$4,published_at=CURRENT_TIMESTAMP WHERE ir_merge_proposal_id=$1`,
		proposalID, fullIRID, key, publishedBy); err != nil {
		return PublishIRMergeResult{}, err
	}
	changeSetID, impactOperationIDs, err := enqueuePublishedFullIRImpact(ctx, tx, proposalID, fullIRID, baseIRID,
		targetSourceID, workID, changedChapterIDs)
	if err != nil {
		return PublishIRMergeResult{}, err
	}
	result, err := loadPublishedIRMergeResult(ctx, tx, proposalID, fullIRID)
	if err != nil {
		return PublishIRMergeResult{}, err
	}
	result.SourceChangeSetID = changeSetID
	result.ImpactOperationIDs = impactOperationIDs
	if err = tx.Commit(ctx); err != nil {
		return PublishIRMergeResult{}, mapPGConflict(err)
	}
	return result, nil
}

func prepareIRMergeSelections(ctx context.Context, tx pgx.Tx, proposalID, baseIRID, incrementalIRID, fullIRID string) error {
	statements := []string{
		`CREATE TEMP TABLE merge_entity_selection(canonical_entity_id TEXT PRIMARY KEY,source_revision_id TEXT NOT NULL,resolved_value JSONB) ON COMMIT DROP`,
		`INSERT INTO merge_entity_selection(canonical_entity_id,source_revision_id)
		 SELECT entity.entity_id,entity.entity_revision_id FROM drama.narrative_entity_revisions entity
		 WHERE entity.ir_revision_id=$2 AND NOT EXISTS(SELECT 1 FROM drama.ir_merge_proposal_items item
		   WHERE item.ir_merge_proposal_id=$1 AND item.item_type='entity' AND item.logical_id=entity.entity_id)`,
		`INSERT INTO merge_entity_selection(canonical_entity_id,source_revision_id,resolved_value)
		 SELECT COALESCE(item.canonical_entity_id,item.logical_id),
		   CASE WHEN item.resolution='keep_old' THEN item.before_revision_id ELSE COALESCE(item.after_revision_id,item.before_revision_id) END,
		   item.resolved_value FROM drama.ir_merge_proposal_items item
		 WHERE item.ir_merge_proposal_id=$1 AND item.item_type='entity' AND item.resolution<>'delete_invalid'
		 ON CONFLICT(canonical_entity_id) DO UPDATE SET source_revision_id=EXCLUDED.source_revision_id,resolved_value=EXCLUDED.resolved_value`,
		`CREATE TEMP TABLE merge_entity_map(source_entity_revision_id TEXT PRIMARY KEY,new_entity_revision_id TEXT NOT NULL,canonical_entity_id TEXT NOT NULL) ON COMMIT DROP`,
		`INSERT INTO merge_entity_map
		 SELECT source.entity_revision_id,'er_'||substr(encode(digest($4||'|'||selection.canonical_entity_id,'sha256'),'hex'),1,32),selection.canonical_entity_id
		 FROM (SELECT * FROM drama.narrative_entity_revisions WHERE ir_revision_id IN ($2,$3)) source
		 JOIN drama.narrative_entities logical ON logical.entity_id=source.entity_id
		 JOIN merge_entity_selection selection ON selection.canonical_entity_id=COALESCE((SELECT item.canonical_entity_id
		   FROM drama.ir_merge_proposal_items item WHERE item.ir_merge_proposal_id=$1 AND item.item_type='entity'
		     AND item.logical_id=logical.entity_id AND item.canonicalization_decision='same_entity'),logical.entity_id)`,
		`CREATE TEMP TABLE merge_fact_selection(fact_id TEXT PRIMARY KEY,source_revision_id TEXT NOT NULL,resolved_value JSONB) ON COMMIT DROP`,
		`INSERT INTO merge_fact_selection(fact_id,source_revision_id)
		 SELECT fact.fact_id,fact.fact_revision_id FROM drama.narrative_fact_revisions fact
		 WHERE fact.ir_revision_id=$2 AND NOT EXISTS(SELECT 1 FROM drama.ir_merge_proposal_items item
		   WHERE item.ir_merge_proposal_id=$1 AND item.logical_id=fact.fact_id AND item.item_type IN ('fact','event','state','foreshadow','relation'))`,
		`INSERT INTO merge_fact_selection(fact_id,source_revision_id,resolved_value)
		 SELECT item.logical_id,CASE WHEN item.resolution='keep_old' THEN item.before_revision_id
		   ELSE COALESCE(item.after_revision_id,item.before_revision_id) END,item.resolved_value
		 FROM drama.ir_merge_proposal_items item
		 WHERE item.ir_merge_proposal_id=$1 AND item.item_type IN ('fact','event','state','foreshadow','relation')
		   AND item.logical_id NOT LIKE 'causal:%' AND item.resolution<>'delete_invalid'
		   AND EXISTS(SELECT 1 FROM drama.narrative_fact_revisions fact
		     WHERE fact.fact_revision_id=CASE WHEN item.resolution='keep_old' THEN item.before_revision_id
		       ELSE COALESCE(item.after_revision_id,item.before_revision_id) END)
		 ON CONFLICT(fact_id) DO UPDATE SET source_revision_id=EXCLUDED.source_revision_id,resolved_value=EXCLUDED.resolved_value`,
		`CREATE TEMP TABLE merge_fact_map(source_fact_revision_id TEXT PRIMARY KEY,new_fact_revision_id TEXT NOT NULL,fact_id TEXT NOT NULL) ON COMMIT DROP`,
		`INSERT INTO merge_fact_map SELECT source.fact_revision_id,
		 'fr_'||substr(encode(digest($4||'|'||source.fact_id,'sha256'),'hex'),1,32),source.fact_id
		 FROM drama.narrative_fact_revisions source JOIN merge_fact_selection selection USING(fact_id)
		 WHERE source.ir_revision_id IN ($2,$3) AND $1::text IS NOT NULL`,
		`CREATE TEMP TABLE merge_event_map(source_event_revision_id TEXT PRIMARY KEY,new_event_revision_id TEXT NOT NULL,fact_id TEXT NOT NULL) ON COMMIT DROP`,
		`INSERT INTO merge_event_map SELECT event.event_revision_id,
		 'evr_'||substr(encode(digest($4||'|'||map.fact_id,'sha256'),'hex'),1,32),map.fact_id
		 FROM drama.narrative_event_revisions event JOIN merge_fact_map map
		   ON map.source_fact_revision_id=event.fact_revision_id
		 WHERE $1::text IS NOT NULL AND $2::text IS NOT NULL AND $3::text IS NOT NULL`,
	}
	for _, statement := range statements {
		if err := execMergeStatement(ctx, tx, statement, proposalID, baseIRID, incrementalIRID, fullIRID); err != nil {
			return err
		}
	}
	return prepareOtherIRMergeSelections(ctx, tx, proposalID, baseIRID, incrementalIRID, fullIRID)
}

func prepareOtherIRMergeSelections(ctx context.Context, tx pgx.Tx, proposalID, baseIRID, incrementalIRID, fullIRID string) error {
	statements := []string{
		`CREATE TEMP TABLE merge_relation_selection(logical_id TEXT PRIMARY KEY,source_relation_id TEXT NOT NULL,resolved_value JSONB) ON COMMIT DROP`,
		`INSERT INTO merge_relation_selection(logical_id,source_relation_id)
		 SELECT 'causal:'||from_fact.fact_id||'|'||to_fact.fact_id||'|'||relation.relation_type,relation.event_relation_id
		 FROM drama.event_relations relation
		 JOIN drama.narrative_event_revisions from_event ON from_event.event_revision_id=relation.from_event_revision_id
		 JOIN drama.narrative_fact_revisions from_fact ON from_fact.fact_revision_id=from_event.fact_revision_id
		 JOIN drama.narrative_event_revisions to_event ON to_event.event_revision_id=relation.to_event_revision_id
		 JOIN drama.narrative_fact_revisions to_fact ON to_fact.fact_revision_id=to_event.fact_revision_id
		 WHERE relation.ir_revision_id=$2 AND NOT EXISTS(SELECT 1 FROM drama.ir_merge_proposal_items item
		   WHERE item.ir_merge_proposal_id=$1 AND item.logical_id='causal:'||from_fact.fact_id||'|'||to_fact.fact_id||'|'||relation.relation_type)`,
		`INSERT INTO merge_relation_selection(logical_id,source_relation_id,resolved_value)
		 SELECT item.logical_id,CASE WHEN item.resolution='keep_old' THEN item.before_revision_id
		   ELSE COALESCE(item.after_revision_id,item.before_revision_id) END,item.resolved_value
		 FROM drama.ir_merge_proposal_items item WHERE item.ir_merge_proposal_id=$1 AND item.item_type='relation'
		   AND item.logical_id LIKE 'causal:%' AND item.resolution<>'delete_invalid'`,
		`CREATE TEMP TABLE merge_arc_selection(story_arc_id TEXT PRIMARY KEY,source_revision_id TEXT NOT NULL,resolved_value JSONB) ON COMMIT DROP`,
		`INSERT INTO merge_arc_selection(story_arc_id,source_revision_id)
		 SELECT arc.story_arc_id,arc.story_arc_revision_id FROM drama.story_arc_revisions arc WHERE arc.ir_revision_id=$2
		 AND NOT EXISTS(SELECT 1 FROM drama.ir_merge_proposal_items item WHERE item.ir_merge_proposal_id=$1
		   AND item.item_type='story_arc' AND item.logical_id=arc.story_arc_id)`,
		`INSERT INTO merge_arc_selection(story_arc_id,source_revision_id,resolved_value)
		 SELECT item.logical_id,CASE WHEN item.resolution='keep_old' THEN item.before_revision_id
		   ELSE COALESCE(item.after_revision_id,item.before_revision_id) END,item.resolved_value
		 FROM drama.ir_merge_proposal_items item WHERE item.ir_merge_proposal_id=$1 AND item.item_type='story_arc'
		   AND item.resolution<>'delete_invalid'
		 ON CONFLICT(story_arc_id) DO UPDATE SET source_revision_id=EXCLUDED.source_revision_id,resolved_value=EXCLUDED.resolved_value`,
		`CREATE TEMP TABLE merge_arc_map(source_arc_revision_id TEXT PRIMARY KEY,new_arc_revision_id TEXT NOT NULL,story_arc_id TEXT NOT NULL) ON COMMIT DROP`,
		`INSERT INTO merge_arc_map SELECT source.story_arc_revision_id,
		 'sar_'||substr(encode(digest($4||'|'||source.story_arc_id,'sha256'),'hex'),1,32),source.story_arc_id
		 FROM drama.story_arc_revisions source JOIN merge_arc_selection selection USING(story_arc_id)
		 WHERE source.ir_revision_id IN ($2,$3) AND $1::text IS NOT NULL`,
	}
	for _, statement := range statements {
		if err := execMergeStatement(ctx, tx, statement, proposalID, baseIRID, incrementalIRID, fullIRID); err != nil {
			return err
		}
	}
	return nil
}

func execMergeStatement(ctx context.Context, tx pgx.Tx, statement string, args ...any) error {
	argumentCount := 0
	for index := len(args); index > 0; index-- {
		if strings.Contains(statement, fmt.Sprintf("$%d", index)) {
			argumentCount = index
			break
		}
	}
	_, err := tx.Exec(ctx, statement, args[:argumentCount]...)
	if err != nil {
		label := strings.TrimSpace(statement)
		if len(label) > 90 {
			label = label[:90]
		}
		return fmt.Errorf("IR merge SQL %q: %w", label, err)
	}
	return nil
}

type mergeSpan struct {
	SourceSpanID, SourceVersionID, ChapterID, ChapterRevisionID string
	StartByte, EndByte, StartCodepoint, EndCodepoint            int
	StartParagraph, EndParagraph                                *int
	EvidenceText                                                *string
	ExcerptHash, LocatorVersion                                 string
	TargetChapterRevisionID, TargetContent                      string
}

func prepareIRMergeSpanMap(ctx context.Context, tx pgx.Tx, proposalID, baseIRID, incrementalIRID, targetSourceID, workID string) error {
	if _, err := tx.Exec(ctx, `CREATE TEMP TABLE merge_span_map(source_span_id TEXT PRIMARY KEY,target_span_id TEXT NOT NULL) ON COMMIT DROP`); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `WITH needed(source_span_id) AS (
		SELECT entity.primary_source_span_id FROM merge_entity_selection selection
		JOIN drama.narrative_entity_revisions entity ON entity.entity_revision_id=selection.source_revision_id
		UNION SELECT mention.source_span_id FROM merge_entity_selection selection
		JOIN drama.narrative_entity_mentions mention ON mention.entity_revision_id=selection.source_revision_id
		UNION SELECT fact.primary_source_span_id FROM merge_fact_selection selection
		JOIN drama.narrative_fact_revisions fact ON fact.fact_revision_id=selection.source_revision_id
		UNION SELECT evidence.source_span_id FROM merge_fact_selection selection
		JOIN drama.fact_evidence evidence ON evidence.fact_revision_id=selection.source_revision_id
		UNION SELECT participant.source_span_id FROM merge_fact_selection selection
		JOIN drama.narrative_event_revisions event ON event.fact_revision_id=selection.source_revision_id
		JOIN drama.event_participants participant USING(event_revision_id)
		UNION SELECT relation.source_span_id FROM merge_relation_selection selection
		JOIN drama.event_relations relation ON relation.event_relation_id=selection.source_relation_id
		UNION SELECT arc.primary_source_span_id FROM merge_arc_selection selection
		JOIN drama.story_arc_revisions arc ON arc.story_arc_revision_id=selection.source_revision_id
	)
	SELECT span.source_span_id,span.source_version_id,span.chapter_id,span.chapter_revision_id,
		span.start_utf8_byte,span.end_utf8_byte,span.start_codepoint,span.end_codepoint,
		span.start_paragraph,span.end_paragraph,span.excerpt_hash,span.evidence_text,span.locator_version,
		target.chapter_revision_id,chapter.content
	FROM needed JOIN drama.source_spans span USING(source_span_id)
	JOIN drama.source_version_chapters target ON target.source_version_id=$1 AND target.chapter_id=span.chapter_id
	JOIN drama.chapter_revisions chapter ON chapter.chapter_revision_id=target.chapter_revision_id`, targetSourceID)
	if err != nil {
		return err
	}
	spans := []mergeSpan{}
	for rows.Next() {
		var span mergeSpan
		if err := rows.Scan(&span.SourceSpanID, &span.SourceVersionID, &span.ChapterID, &span.ChapterRevisionID,
			&span.StartByte, &span.EndByte, &span.StartCodepoint, &span.EndCodepoint, &span.StartParagraph,
			&span.EndParagraph, &span.ExcerptHash, &span.EvidenceText, &span.LocatorVersion,
			&span.TargetChapterRevisionID, &span.TargetContent); err != nil {
			rows.Close()
			return err
		}
		spans = append(spans, span)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, span := range spans {
		targetSpanID := span.SourceSpanID
		if span.SourceVersionID != targetSourceID {
			targetSpanID = "spn_" + hashText(proposalID + "|" + span.SourceSpanID)[:32]
			startByte, endByte, startRune, endRune := span.StartByte, span.EndByte, span.StartCodepoint, span.EndCodepoint
			startParagraph, endParagraph := span.StartParagraph, span.EndParagraph
			if span.ChapterRevisionID != span.TargetChapterRevisionID {
				if span.EvidenceText == nil || *span.EvidenceText == "" {
					return fmt.Errorf("%w: source span %s cannot be relocated: evidence_text is empty", ErrIRMergeBlocked, span.SourceSpanID)
				}
				var relocateErr error
				startByte, endByte, startRune, endRune, startParagraph, endParagraph, relocateErr = relocateUTF8Evidence(span.TargetContent, *span.EvidenceText, span.StartByte)
				if relocateErr != nil {
					return fmt.Errorf("%w: source span %s cannot be relocated: %v", ErrIRMergeBlocked, span.SourceSpanID, relocateErr)
				}
			}
			if _, err := tx.Exec(ctx, `INSERT INTO drama.source_spans(source_span_id,work_id,source_version_id,chapter_id,
				chapter_revision_id,start_utf8_byte,end_utf8_byte,start_codepoint,end_codepoint,start_paragraph,end_paragraph,
				excerpt_hash,evidence_text,locator_version,idempotency_key)
				VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,'utf8-codepoint-v1',$14)
				ON CONFLICT(source_span_id) DO NOTHING`, targetSpanID, workID, targetSourceID, span.ChapterID,
				span.TargetChapterRevisionID, startByte, endByte, startRune, endRune, startParagraph, endParagraph,
				span.ExcerptHash, span.EvidenceText, "ir-merge-span:"+proposalID+":"+span.SourceSpanID); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO merge_span_map(source_span_id,target_span_id) VALUES($1,$2)`, span.SourceSpanID, targetSpanID); err != nil {
			return err
		}
	}
	return nil
}

func relocateUTF8Evidence(content, evidence string, oldStart int) (int, int, int, int, *int, *int, error) {
	if evidence == "" {
		return 0, 0, 0, 0, nil, nil, errors.New("evidence_text is empty")
	}
	positions := []int{}
	for offset := 0; offset <= len(content)-len(evidence); {
		index := strings.Index(content[offset:], evidence)
		if index < 0 {
			break
		}
		position := offset + index
		positions = append(positions, position)
		offset = position + 1
	}
	if len(positions) == 0 {
		return 0, 0, 0, 0, nil, nil, errors.New("evidence is absent from the target chapter")
	}
	sort.Slice(positions, func(i, j int) bool {
		di, dj := positions[i]-oldStart, positions[j]-oldStart
		if di < 0 {
			di = -di
		}
		if dj < 0 {
			dj = -dj
		}
		return di < dj
	})
	if len(positions) > 1 {
		d0, d1 := positions[0]-oldStart, positions[1]-oldStart
		if d0 < 0 {
			d0 = -d0
		}
		if d1 < 0 {
			d1 = -d1
		}
		if d0 == d1 {
			return 0, 0, 0, 0, nil, nil, errors.New("evidence relocation is ambiguous")
		}
	}
	startByte := positions[0]
	endByte := startByte + len(evidence)
	startRune := utf8.RuneCountInString(content[:startByte])
	endRune := startRune + utf8.RuneCountInString(evidence)
	startParagraphValue := strings.Count(content[:startByte], "\n") + 1
	endParagraphValue := strings.Count(content[:endByte], "\n") + 1
	return startByte, endByte, startRune, endRune, &startParagraphValue, &endParagraphValue, nil
}

func copyIRMergeSnapshot(ctx context.Context, tx pgx.Tx, proposalID, fullIRID, targetSourceID, workID string) error {
	var missingReferences int
	if err := tx.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM merge_fact_selection selection
		 JOIN drama.narrative_event_revisions event ON event.fact_revision_id=selection.source_revision_id
		 JOIN drama.event_participants participant USING(event_revision_id)
		 LEFT JOIN merge_entity_map entity_map ON entity_map.source_entity_revision_id=participant.entity_revision_id
		 WHERE entity_map.new_entity_revision_id IS NULL)
		+
		(SELECT count(*) FROM merge_fact_selection selection
		 JOIN drama.character_state_changes state ON state.fact_revision_id=selection.source_revision_id
		 LEFT JOIN merge_entity_map entity_map ON entity_map.source_entity_revision_id=state.character_entity_revision_id
		 WHERE entity_map.new_entity_revision_id IS NULL)`).Scan(&missingReferences); err != nil {
		return err
	}
	if missingReferences != 0 {
		return fmt.Errorf("%w: %d selected facts reference deleted or unconfirmed entities", ErrIRMergeBlocked, missingReferences)
	}

	statements := []string{
		`INSERT INTO drama.narrative_entity_revisions(entity_revision_id,entity_id,ir_revision_id,work_id,source_version_id,
		 chapter_id,primary_chapter_revision_id,primary_source_span_id,canonical_name,attributes,confidence,
		 validation_status,idempotency_key)
		 SELECT 'er_'||substr(encode(digest($2||'|'||selection.canonical_entity_id,'sha256'),'hex'),1,32),
		 selection.canonical_entity_id,$2,$4,$3,target_span.chapter_id,target_span.chapter_revision_id,span_map.target_span_id,
		 COALESCE(selection.resolved_value->>'canonical_name',source.canonical_name),
		 COALESCE(selection.resolved_value->'attributes',source.attributes),
		 COALESCE((selection.resolved_value->>'confidence')::numeric,source.confidence),'valid',
		 'ir-merge-entity:'||$1||':'||selection.canonical_entity_id
		 FROM merge_entity_selection selection
		 JOIN drama.narrative_entity_revisions source ON source.entity_revision_id=selection.source_revision_id
		 JOIN merge_span_map span_map ON span_map.source_span_id=source.primary_source_span_id
		 JOIN drama.source_spans target_span ON target_span.source_span_id=span_map.target_span_id`,
		`INSERT INTO drama.narrative_entity_aliases(entity_alias_id,entity_revision_id,alias,alias_type)
		 SELECT 'eal_'||substr(encode(digest($1||'|'||selection.canonical_entity_id||'|'||alias.alias||'|'||alias.alias_type,'sha256'),'hex'),1,32),
		 'er_'||substr(encode(digest($2||'|'||selection.canonical_entity_id,'sha256'),'hex'),1,32),alias.alias,alias.alias_type
		 FROM merge_entity_selection selection JOIN drama.narrative_entity_aliases alias
		   ON alias.entity_revision_id=selection.source_revision_id
		 ON CONFLICT(entity_revision_id,alias) DO NOTHING`,
		`INSERT INTO drama.narrative_entity_mentions(entity_mention_id,entity_revision_id,ir_revision_id,work_id,
		 source_version_id,source_span_id,mention_text,confidence,idempotency_key)
		 SELECT 'em_'||substr(encode(digest($1||'|'||mention.entity_mention_id,'sha256'),'hex'),1,32),
		 'er_'||substr(encode(digest($2||'|'||selection.canonical_entity_id,'sha256'),'hex'),1,32),$2,$4,$3,
		 span_map.target_span_id,mention.mention_text,mention.confidence,'ir-merge-mention:'||$1||':'||mention.entity_mention_id
		 FROM merge_entity_selection selection JOIN drama.narrative_entity_mentions mention
		   ON mention.entity_revision_id=selection.source_revision_id
		 JOIN merge_span_map span_map USING(source_span_id)`,
		`INSERT INTO drama.narrative_fact_revisions(fact_revision_id,fact_id,ir_revision_id,work_id,source_version_id,
		 chapter_id,primary_chapter_revision_id,primary_source_span_id,canonical_fingerprint,confidence,payload,
		 validation_status,idempotency_key)
		 SELECT 'fr_'||substr(encode(digest($2||'|'||selection.fact_id,'sha256'),'hex'),1,32),selection.fact_id,$2,$4,$3,
		 target_span.chapter_id,target_span.chapter_revision_id,span_map.target_span_id,
		 CASE WHEN selection.resolved_value IS NULL THEN source.canonical_fingerprint
		   ELSE encode(digest(selection.resolved_value::text,'sha256'),'hex') END,
		 COALESCE((selection.resolved_value->>'confidence')::numeric,source.confidence),
		 COALESCE(selection.resolved_value->'payload',source.payload),'valid',
		 'ir-merge-fact:'||$1||':'||selection.fact_id
		 FROM merge_fact_selection selection JOIN drama.narrative_fact_revisions source
		   ON source.fact_revision_id=selection.source_revision_id
		 JOIN merge_span_map span_map ON span_map.source_span_id=source.primary_source_span_id
		 JOIN drama.source_spans target_span ON target_span.source_span_id=span_map.target_span_id`,
		`INSERT INTO drama.fact_evidence(fact_evidence_id,fact_revision_id,ir_revision_id,work_id,source_version_id,
		 source_span_id,evidence_role,confidence,idempotency_key)
		 SELECT 'fev_'||substr(encode(digest($1||'|'||evidence.fact_evidence_id,'sha256'),'hex'),1,32),
		 'fr_'||substr(encode(digest($2||'|'||selection.fact_id,'sha256'),'hex'),1,32),$2,$4,$3,
		 span_map.target_span_id,evidence.evidence_role,evidence.confidence,'ir-merge-evidence:'||$1||':'||evidence.fact_evidence_id
		 FROM merge_fact_selection selection JOIN drama.fact_evidence evidence
		   ON evidence.fact_revision_id=selection.source_revision_id JOIN merge_span_map span_map USING(source_span_id)`,
		`INSERT INTO drama.narrative_event_revisions(event_revision_id,fact_revision_id,ir_revision_id,work_id,
		 source_version_id,event_type,summary,narrative_order,temporal_expression,location_entity_revision_id,importance)
		 SELECT 'evr_'||substr(encode(digest($2||'|'||selection.fact_id,'sha256'),'hex'),1,32),
		 'fr_'||substr(encode(digest($2||'|'||selection.fact_id,'sha256'),'hex'),1,32),$2,$4,$3,
		 COALESCE(selection.resolved_value->'event'->>'event_type',event.event_type),
		 COALESCE(selection.resolved_value->'event'->>'summary',event.summary),
		 COALESCE((selection.resolved_value->'event'->>'narrative_order')::numeric,event.narrative_order),
		 COALESCE(selection.resolved_value->'event'->>'temporal_expression',event.temporal_expression),
		 location_map.new_entity_revision_id,
		 COALESCE((selection.resolved_value->'event'->>'importance')::numeric,event.importance)
		 FROM merge_fact_selection selection JOIN drama.narrative_event_revisions event
		   ON event.fact_revision_id=selection.source_revision_id
		 LEFT JOIN merge_entity_map location_map ON location_map.source_entity_revision_id=event.location_entity_revision_id
		 WHERE $1::text IS NOT NULL`,
		`INSERT INTO drama.event_participants(event_participant_id,event_revision_id,entity_revision_id,ir_revision_id,
		 work_id,source_version_id,participant_role,participation_state,source_span_id,confidence,idempotency_key)
		 SELECT 'ep_'||substr(encode(digest($1||'|'||participant.event_participant_id,'sha256'),'hex'),1,32),
		 'evr_'||substr(encode(digest($2||'|'||selection.fact_id,'sha256'),'hex'),1,32),entity_map.new_entity_revision_id,
		 $2,$4,$3,participant.participant_role,participant.participation_state,span_map.target_span_id,
		 participant.confidence,'ir-merge-participant:'||$1||':'||participant.event_participant_id
		 FROM merge_fact_selection selection JOIN drama.narrative_event_revisions event
		   ON event.fact_revision_id=selection.source_revision_id JOIN drama.event_participants participant USING(event_revision_id)
		 JOIN merge_entity_map entity_map ON entity_map.source_entity_revision_id=participant.entity_revision_id
		 JOIN merge_span_map span_map USING(source_span_id)`,
		`INSERT INTO drama.character_state_changes(state_change_id,fact_revision_id,character_entity_revision_id,
		 ir_revision_id,work_id,source_version_id,state_dimension,before_state,after_state,trigger_event_revision_id,sequence_number)
		 SELECT 'state_'||substr(encode(digest($2||'|'||selection.fact_id,'sha256'),'hex'),1,32),
		 'fr_'||substr(encode(digest($2||'|'||selection.fact_id,'sha256'),'hex'),1,32),entity_map.new_entity_revision_id,
		 $2,$4,$3,COALESCE(selection.resolved_value->'state'->>'state_dimension',state.state_dimension),
		 COALESCE(selection.resolved_value->'state'->'before_state',state.before_state),
		 COALESCE(selection.resolved_value->'state'->'after_state',state.after_state),event_map.new_event_revision_id,
		 COALESCE((selection.resolved_value->'state'->>'sequence_number')::numeric,state.sequence_number)
		 FROM merge_fact_selection selection JOIN drama.character_state_changes state
		   ON state.fact_revision_id=selection.source_revision_id
		 JOIN merge_entity_map entity_map ON entity_map.source_entity_revision_id=state.character_entity_revision_id
		 LEFT JOIN merge_event_map event_map ON event_map.source_event_revision_id=state.trigger_event_revision_id
		 WHERE $1::text IS NOT NULL`,
		`INSERT INTO drama.timeline_facts(timeline_fact_id,fact_revision_id,subject_entity_revision_id,event_revision_id,
		 ir_revision_id,work_id,source_version_id,temporal_expression,normalized_time,timeline_order,certainty)
		 SELECT 'timeline_'||substr(encode(digest($2||'|'||selection.fact_id,'sha256'),'hex'),1,32),
		 'fr_'||substr(encode(digest($2||'|'||selection.fact_id,'sha256'),'hex'),1,32),entity_map.new_entity_revision_id,
		 event_map.new_event_revision_id,$2,$4,$3,
		 COALESCE(selection.resolved_value->'timeline'->>'temporal_expression',timeline.temporal_expression),
		 COALESCE(selection.resolved_value->'timeline'->'normalized_time',timeline.normalized_time),
		 COALESCE((selection.resolved_value->'timeline'->>'timeline_order')::numeric,timeline.timeline_order),
		 COALESCE(selection.resolved_value->'timeline'->>'certainty',timeline.certainty)
		 FROM merge_fact_selection selection JOIN drama.timeline_facts timeline
		   ON timeline.fact_revision_id=selection.source_revision_id
		 LEFT JOIN merge_entity_map entity_map ON entity_map.source_entity_revision_id=timeline.subject_entity_revision_id
		 LEFT JOIN merge_event_map event_map ON event_map.source_event_revision_id=timeline.event_revision_id
		 WHERE $1::text IS NOT NULL`,
		`INSERT INTO drama.foreshadow_occurrences(foreshadow_occurrence_id,foreshadow_thread_id,fact_revision_id,
		 event_revision_id,ir_revision_id,work_id,source_version_id,lifecycle_stage,occurrence_order)
		 SELECT 'fso_'||substr(encode(digest($2||'|'||selection.fact_id,'sha256'),'hex'),1,32),
		 occurrence.foreshadow_thread_id,'fr_'||substr(encode(digest($2||'|'||selection.fact_id,'sha256'),'hex'),1,32),
		 event_map.new_event_revision_id,$2,$4,$3,
		 COALESCE(selection.resolved_value->'foreshadow'->>'lifecycle_stage',occurrence.lifecycle_stage),
		 COALESCE((selection.resolved_value->'foreshadow'->>'occurrence_order')::numeric,occurrence.occurrence_order)
		 FROM merge_fact_selection selection JOIN drama.foreshadow_occurrences occurrence
		   ON occurrence.fact_revision_id=selection.source_revision_id
		 LEFT JOIN merge_event_map event_map ON event_map.source_event_revision_id=occurrence.event_revision_id
		 WHERE $1::text IS NOT NULL`,
	}
	for _, statement := range statements {
		if err := execMergeStatement(ctx, tx, statement, proposalID, fullIRID, targetSourceID, workID); err != nil {
			return err
		}
	}
	return copyIRMergeRelationsAndArcs(ctx, tx, proposalID, fullIRID, targetSourceID, workID)
}

func copyIRMergeRelationsAndArcs(ctx context.Context, tx pgx.Tx, proposalID, fullIRID, targetSourceID, workID string) error {
	statements := []string{
		`INSERT INTO drama.event_relations(event_relation_id,from_event_revision_id,to_event_revision_id,ir_revision_id,
		 work_id,source_version_id,relation_type,source_span_id,confidence,idempotency_key)
		 SELECT 'erel_'||substr(encode(digest($2||'|'||selection.logical_id,'sha256'),'hex'),1,32),
		 from_map.new_event_revision_id,to_map.new_event_revision_id,$2,$4,$3,
		 COALESCE(selection.resolved_value->>'relation_type',relation.relation_type),span_map.target_span_id,
		 COALESCE((selection.resolved_value->>'confidence')::numeric,relation.confidence),
		 'ir-merge-relation:'||$1||':'||selection.logical_id
		 FROM merge_relation_selection selection JOIN drama.event_relations relation
		   ON relation.event_relation_id=selection.source_relation_id
		 JOIN merge_event_map from_map ON from_map.source_event_revision_id=relation.from_event_revision_id
		 JOIN merge_event_map to_map ON to_map.source_event_revision_id=relation.to_event_revision_id
		 JOIN merge_span_map span_map USING(source_span_id)
		 ON CONFLICT(from_event_revision_id,to_event_revision_id,relation_type) DO NOTHING`,
		`INSERT INTO drama.story_arc_revisions(story_arc_revision_id,story_arc_id,ir_revision_id,work_id,source_version_id,
		 chapter_id,primary_chapter_revision_id,primary_source_span_id,title,summary,arc_type,confidence,idempotency_key)
		 SELECT 'sar_'||substr(encode(digest($2||'|'||selection.story_arc_id,'sha256'),'hex'),1,32),selection.story_arc_id,
		 $2,$4,$3,target_span.chapter_id,target_span.chapter_revision_id,span_map.target_span_id,
		 COALESCE(selection.resolved_value->>'title',arc.title),COALESCE(selection.resolved_value->>'summary',arc.summary),
		 COALESCE(selection.resolved_value->>'arc_type',arc.arc_type),
		 COALESCE((selection.resolved_value->>'confidence')::numeric,arc.confidence),
		 'ir-merge-arc:'||$1||':'||selection.story_arc_id
		 FROM merge_arc_selection selection JOIN drama.story_arc_revisions arc
		   ON arc.story_arc_revision_id=selection.source_revision_id
		 JOIN merge_span_map span_map ON span_map.source_span_id=arc.primary_source_span_id
		 JOIN drama.source_spans target_span ON target_span.source_span_id=span_map.target_span_id`,
		`INSERT INTO drama.story_arc_events(story_arc_event_id,story_arc_revision_id,event_revision_id,ir_revision_id,
		 work_id,source_version_id,event_ordinal,arc_role,idempotency_key)
		 SELECT 'sae_'||substr(encode(digest($1||'|'||arc_event.story_arc_event_id,'sha256'),'hex'),1,32),
		 'sar_'||substr(encode(digest($2||'|'||selection.story_arc_id,'sha256'),'hex'),1,32),event_map.new_event_revision_id,
		 $2,$4,$3,arc_event.event_ordinal,arc_event.arc_role,'ir-merge-arc-event:'||$1||':'||arc_event.story_arc_event_id
		 FROM merge_arc_selection selection JOIN drama.story_arc_events arc_event
		   ON arc_event.story_arc_revision_id=selection.source_revision_id
		 JOIN merge_event_map event_map ON event_map.source_event_revision_id=arc_event.event_revision_id`,
	}
	for _, statement := range statements {
		if err := execMergeStatement(ctx, tx, statement, proposalID, fullIRID, targetSourceID, workID); err != nil {
			return err
		}
	}
	return nil
}

func calculateFullIRHash(ctx context.Context, tx pgx.Tx, fullIRID string) (string, error) {
	var outputHash string
	err := tx.QueryRow(ctx, `SELECT encode(digest(jsonb_build_object(
		'entities',COALESCE((SELECT jsonb_agg(jsonb_build_object('entity_id',entity.entity_id,'canonical_name',entity.canonical_name,
		  'attributes',entity.attributes,'confidence',entity.confidence) ORDER BY entity.entity_id)
		  FROM drama.narrative_entity_revisions entity WHERE entity.ir_revision_id=$1),'[]'::jsonb),
		'facts',COALESCE((SELECT jsonb_agg(jsonb_build_object('fact_id',fact.fact_id,'fingerprint',fact.canonical_fingerprint,
		  'payload',fact.payload,'confidence',fact.confidence) ORDER BY fact.fact_id)
		  FROM drama.narrative_fact_revisions fact WHERE fact.ir_revision_id=$1),'[]'::jsonb),
		'events',COALESCE((SELECT jsonb_agg(jsonb_build_object('event_revision_id',event.event_revision_id,
		  'summary',event.summary,'order',event.narrative_order) ORDER BY event.narrative_order,event.event_revision_id)
		  FROM drama.narrative_event_revisions event WHERE event.ir_revision_id=$1),'[]'::jsonb),
		'relations',COALESCE((SELECT jsonb_agg(jsonb_build_object('from',relation.from_event_revision_id,
		  'to',relation.to_event_revision_id,'type',relation.relation_type) ORDER BY relation.event_relation_id)
		  FROM drama.event_relations relation WHERE relation.ir_revision_id=$1),'[]'::jsonb),
		'states',COALESCE((SELECT jsonb_agg(jsonb_build_object('fact',state.fact_revision_id,'dimension',state.state_dimension,
		  'before',state.before_state,'after',state.after_state) ORDER BY state.state_change_id)
		  FROM drama.character_state_changes state WHERE state.ir_revision_id=$1),'[]'::jsonb),
		'foreshadow',COALESCE((SELECT jsonb_agg(jsonb_build_object('thread',occurrence.foreshadow_thread_id,
		  'stage',occurrence.lifecycle_stage,'order',occurrence.occurrence_order) ORDER BY occurrence.foreshadow_thread_id,occurrence.occurrence_order)
		  FROM drama.foreshadow_occurrences occurrence WHERE occurrence.ir_revision_id=$1),'[]'::jsonb)
	)::text,'sha256'),'hex')`, fullIRID).Scan(&outputHash)
	return outputHash, err
}

func enqueuePublishedFullIRImpact(ctx context.Context, tx pgx.Tx, proposalID, fullIRID, baseIRID,
	targetSourceID, workID string, changedChapterIDs []string) (string, []string, error) {
	var baseSourceID string
	if err := tx.QueryRow(ctx, `SELECT source_version_id FROM drama.narrative_ir_revisions WHERE ir_revision_id=$1`, baseIRID).
		Scan(&baseSourceID); err != nil {
		return "", nil, err
	}
	changeSetID := "chg_" + hashText("full-ir-merge-impact|" + proposalID)[:32]
	if _, err := tx.Exec(ctx, `INSERT INTO drama.source_change_sets(source_change_set_id,work_id,from_source_version_id,
		to_source_version_id,from_ir_revision_id,to_ir_revision_id,changed_chapter_ids,status,idempotency_key,summary)
		VALUES($1,$2,$3,$4,$5,$6,$7,'pending',$8,jsonb_build_object('merge_proposal_id',$9::text,
		'authoritative_full_ir',true,'auto_rebuild',false))`, changeSetID, workID, baseSourceID, targetSourceID,
		baseIRID, fullIRID, mustJSON(changedChapterIDs), "full-ir-merge-impact:"+proposalID, proposalID); err != nil {
		return "", nil, mapPGConflict(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO drama.source_change_items(source_change_item_id,source_change_set_id,
		entity_type,change_type,before_entity_id,after_entity_id,semantic_fingerprint,details)
		SELECT 'sci_'||substr(encode(digest($2||'|'||item.ir_merge_item_id,'sha256'),'hex'),1,32),$2,
		CASE WHEN item.item_type='entity' THEN 'entity' WHEN item.item_type='story_arc' THEN 'story_arc' ELSE 'fact' END,
		CASE WHEN item.resolution='keep_old' THEN 'unchanged'
		  WHEN item.change_type='added' THEN 'added' WHEN item.change_type='deleted' THEN 'removed'
		  WHEN item.change_type='relocated' THEN 'relocated' WHEN item.change_type='unchanged' THEN 'unchanged' ELSE 'changed' END,
		item.before_revision_id,
		CASE WHEN item.resolution='delete_invalid' THEN NULL
		  WHEN item.item_type='entity' THEN 'er_'||substr(encode(digest($3||'|'||COALESCE(item.canonical_entity_id,item.logical_id),'sha256'),'hex'),1,32)
		  WHEN item.item_type='story_arc' THEN 'sar_'||substr(encode(digest($3||'|'||item.logical_id,'sha256'),'hex'),1,32)
		  WHEN item.logical_id LIKE 'causal:%' THEN 'erel_'||substr(encode(digest($3||'|'||item.logical_id,'sha256'),'hex'),1,32)
		  ELSE 'fr_'||substr(encode(digest($3||'|'||item.logical_id,'sha256'),'hex'),1,32) END,
		item.semantic_fingerprint,jsonb_build_object('subtype',item.item_type,'logical_id',item.logical_id,
		  'semantic_changed',CASE WHEN item.resolution='keep_old' THEN false ELSE item.semantic_changed END,
		  'source_span_changed',item.source_span_changed,
		  'resolution',item.resolution,'merge_proposal_id',$1::text,
		  'before_event_revision_id',item.before_value->'event'->>'event_revision_id',
		  'before_value',item.before_value,'after_value',item.after_value)
		FROM drama.ir_merge_proposal_items item WHERE item.ir_merge_proposal_id=$1
		  AND NOT (item.change_type='added' AND item.resolution='delete_invalid')`,
		proposalID, changeSetID, fullIRID); err != nil {
		return "", nil, err
	}

	operationIDs := []string{}
	rows, err := tx.Query(ctx, `SELECT binding.project_id FROM drama.project_source_bindings binding
		WHERE binding.work_id=$1 AND binding.is_current AND binding.source_version_id IN ($2,$3)
		ORDER BY binding.project_id`, workID, baseSourceID, targetSourceID)
	if err != nil {
		return "", nil, err
	}
	projectIDs := []string{}
	for rows.Next() {
		var projectID string
		if err := rows.Scan(&projectID); err != nil {
			rows.Close()
			return "", nil, err
		}
		projectIDs = append(projectIDs, projectID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return "", nil, err
	}
	for _, projectID := range projectIDs {
		operationID := "op_" + hashText("full-ir-impact|" + projectID + "|" + changeSetID)[:32]
		traceID := "tr_" + hashText("trace|" + operationID)[:32]
		taskID := "inv_" + hashText(operationID)[:32]
		inputHash := hashText(baseIRID + "|" + fullIRID + "|" + strings.Join(changedChapterIDs, ","))
		checkpoint := mustJSON(map[string]any{"source_change_set_id": changeSetID, "changed_chapter_ids": changedChapterIDs,
			"authoritative_full_ir": true, "merge_proposal_id": proposalID})
		if _, err := tx.Exec(ctx, `INSERT INTO drama.operations(operation_id,trace_id,operation_type,target_type,target_id,status,
			idempotency_key,input_hash,checkpoint_stage,checkpoint_data)
			VALUES($1,$2,'invalidation_scan','project',$3,'pending',$4,$5,'queued',$6)`, operationID, traceID,
			projectID, "full-ir-impact-scan:"+projectID+":"+proposalID, inputHash, checkpoint); err != nil {
			return "", nil, err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO drama.invalidation_tasks(invalidation_task_id,operation_id,project_id,
			source_change_set_id,status,reason_type,idempotency_key,checkpoint)
			VALUES($1,$2,$3,$4,'pending','source_changed',$5,$6)`, taskID, operationID, projectID, changeSetID,
			"full-ir-impact-task:"+projectID+":"+proposalID, checkpoint); err != nil {
			return "", nil, err
		}
		operationIDs = append(operationIDs, operationID)
	}
	return changeSetID, operationIDs, nil
}

func loadPublishedIRMergeResult(ctx context.Context, db queryRower, proposalID, fullIRID string) (PublishIRMergeResult, error) {
	var result PublishIRMergeResult
	result.IRMergeProposalID = proposalID
	result.FullIRRevisionID = fullIRID
	result.Status = "published"
	if err := db.QueryRow(ctx, `SELECT published_at FROM drama.narrative_ir_revisions WHERE ir_revision_id=$1`, fullIRID).
		Scan(&result.PublishedAt); err != nil {
		return result, err
	}
	err := db.QueryRow(ctx, `SELECT source_change_set_id FROM drama.source_change_sets
		WHERE to_ir_revision_id=$1 ORDER BY created_at DESC LIMIT 1`, fullIRID).Scan(&result.SourceChangeSetID)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	rows, err := db.Query(ctx, `SELECT operation_id FROM drama.invalidation_tasks WHERE source_change_set_id=$1 ORDER BY operation_id`, result.SourceChangeSetID)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var operationID string
		if err := rows.Scan(&operationID); err != nil {
			return result, err
		}
		result.ImpactOperationIDs = append(result.ImpactOperationIDs, operationID)
	}
	return result, rows.Err()
}
