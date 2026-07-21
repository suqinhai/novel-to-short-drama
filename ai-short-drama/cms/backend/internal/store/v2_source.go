package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

var (
	ErrConflict         = errors.New("resource conflict")
	ErrRevisionConflict = errors.New("resource revision conflict")
	ErrImmutable        = errors.New("resource is immutable")
	ErrUnsupported      = errors.New("operation is not supported")
)

func newPublicID(prefix string) (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(raw), nil
}

func hashJSON(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func hashText(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func (s *Store) ListSourceWorks(ctx context.Context, query string, page, limit int) (SourceWorkList, error) {
	query = strings.TrimSpace(query)
	var result SourceWorkList
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM drama.source_works
		WHERE $1='' OR title ILIKE '%'||$1||'%' OR COALESCE(author,'') ILIKE '%'||$1||'%'`, query).Scan(&result.Total); err != nil {
		return result, err
	}
	rows, err := s.pool.Query(ctx, `SELECT work_id,title,author,status,resource_revision,metadata,created_at,updated_at
		FROM drama.source_works WHERE $1='' OR title ILIKE '%'||$1||'%' OR COALESCE(author,'') ILIKE '%'||$1||'%'
		ORDER BY updated_at DESC,work_id LIMIT $2 OFFSET $3`, query, limit, (page-1)*limit)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	result.Items = make([]SourceWork, 0)
	for rows.Next() {
		var item SourceWork
		if err := rows.Scan(&item.WorkID, &item.Title, &item.Author, &item.Status, &item.ResourceRevision, &item.Metadata, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return SourceWorkList{}, err
		}
		result.Items = append(result.Items, item)
	}
	return result, rows.Err()
}

func (s *Store) CreateSourceWork(ctx context.Context, key string, input CreateSourceWorkInput) (SourceWork, bool, error) {
	workID, err := newPublicID("sw_")
	if err != nil {
		return SourceWork{}, false, err
	}
	var item SourceWork
	err = s.writer.QueryRow(ctx, `INSERT INTO drama.source_works(work_id,title,author,idempotency_key,metadata)
		VALUES($1,$2,$3,$4,$5) ON CONFLICT(idempotency_key) DO NOTHING
		RETURNING work_id,title,author,status,resource_revision,metadata,created_at,updated_at`,
		workID, input.Title, input.Author, key, input.Metadata).Scan(
		&item.WorkID, &item.Title, &item.Author, &item.Status, &item.ResourceRevision, &item.Metadata, &item.CreatedAt, &item.UpdatedAt)
	if err == nil {
		return item, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return SourceWork{}, false, mapPGConflict(err)
	}
	err = s.writer.QueryRow(ctx, `SELECT work_id,title,author,status,resource_revision,metadata,created_at,updated_at
		FROM drama.source_works WHERE idempotency_key=$1`, key).Scan(
		&item.WorkID, &item.Title, &item.Author, &item.Status, &item.ResourceRevision,
		&item.Metadata, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return SourceWork{}, false, err
	}
	if item.Title != input.Title || !equalOptionalString(item.Author, input.Author) || !equalJSON(item.Metadata, input.Metadata) {
		return SourceWork{}, false, ErrConflict
	}
	return item, false, nil
}

func equalOptionalString(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func equalJSON(a, b json.RawMessage) bool {
	var av, bv any
	return json.Unmarshal(a, &av) == nil && json.Unmarshal(b, &bv) == nil && fmt.Sprintf("%#v", av) == fmt.Sprintf("%#v", bv)
}

func (s *Store) GetSourceWork(ctx context.Context, workID string) (SourceWork, error) {
	var item SourceWork
	err := s.pool.QueryRow(ctx, `SELECT work_id,title,author,status,resource_revision,metadata,created_at,updated_at
		FROM drama.source_works WHERE work_id=$1`, workID).Scan(
		&item.WorkID, &item.Title, &item.Author, &item.Status, &item.ResourceRevision, &item.Metadata, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return item, ErrNotFound
	}
	return item, err
}

func scanSourceVersion(row pgx.Row) (SourceVersion, error) {
	var item SourceVersion
	err := row.Scan(&item.SourceVersionID, &item.WorkID, &item.VersionNumber, &item.ParentSourceVersionID,
		&item.Status, &item.VersionHash, &item.NormalizationVersion, &item.ChapterCount,
		&item.TotalChars, &item.ResourceRevision, &item.Metadata)
	return item, err
}

const sourceVersionColumns = `source_version_id,work_id,version_number,parent_source_version_id,status,
	version_hash,normalization_version,chapter_count,total_chars,resource_revision,metadata`

func (s *Store) GetSourceVersion(ctx context.Context, sourceVersionID string) (SourceVersion, error) {
	item, err := scanSourceVersion(s.pool.QueryRow(ctx, `SELECT `+sourceVersionColumns+` FROM drama.source_versions WHERE source_version_id=$1`, sourceVersionID))
	if errors.Is(err, pgx.ErrNoRows) {
		return item, ErrNotFound
	}
	return item, err
}

func (s *Store) ListSourceVersions(ctx context.Context, workID string) ([]SourceVersion, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+sourceVersionColumns+` FROM drama.source_versions WHERE work_id=$1 ORDER BY version_number DESC`, workID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]SourceVersion, 0)
	for rows.Next() {
		item, err := scanSourceVersion(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) CreateSourceVersion(ctx context.Context, workID, key string, input CreateSourceVersionInput) (SourceVersion, bool, error) {
	tx, err := s.writer.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return SourceVersion{}, false, err
	}
	defer tx.Rollback(ctx)
	var lockedWorkID string
	if err := tx.QueryRow(ctx, `SELECT work_id FROM drama.source_works WHERE work_id=$1 FOR UPDATE`, workID).Scan(&lockedWorkID); errors.Is(err, pgx.ErrNoRows) {
		return SourceVersion{}, false, ErrNotFound
	} else if err != nil {
		return SourceVersion{}, false, err
	}
	existing, err := scanSourceVersion(tx.QueryRow(ctx, `SELECT `+sourceVersionColumns+` FROM drama.source_versions WHERE idempotency_key=$1`, key))
	if err == nil {
		if existing.WorkID != workID || existing.NormalizationVersion != input.NormalizationVersion || !equalOptionalString(existing.ParentSourceVersionID, input.ParentSourceVersionID) || !equalJSON(existing.Metadata, input.Metadata) {
			return SourceVersion{}, false, ErrConflict
		}
		return existing, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return SourceVersion{}, false, err
	}
	var versionNumber int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(max(version_number),0)+1 FROM drama.source_versions WHERE work_id=$1`, workID).Scan(&versionNumber); err != nil {
		return SourceVersion{}, false, err
	}
	versionID, err := newPublicID("sv_")
	if err != nil {
		return SourceVersion{}, false, err
	}
	versionHash := strings.Repeat("0", 64)
	chapterCount, totalChars := 0, 0
	if input.ParentSourceVersionID != nil {
		var parentWork, parentStatus string
		if err := tx.QueryRow(ctx, `SELECT work_id,status,version_hash,chapter_count,total_chars FROM drama.source_versions WHERE source_version_id=$1`, *input.ParentSourceVersionID).
			Scan(&parentWork, &parentStatus, &versionHash, &chapterCount, &totalChars); errors.Is(err, pgx.ErrNoRows) {
			return SourceVersion{}, false, ErrNotFound
		} else if err != nil {
			return SourceVersion{}, false, err
		} else if parentWork != workID || (parentStatus != "published" && parentStatus != "superseded") {
			return SourceVersion{}, false, ErrConflict
		}
	}
	item, err := scanSourceVersion(tx.QueryRow(ctx, `INSERT INTO drama.source_versions(
		source_version_id,work_id,version_number,parent_source_version_id,version_hash,normalization_version,
		chapter_count,total_chars,idempotency_key,metadata) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		RETURNING `+sourceVersionColumns, versionID, workID, versionNumber, input.ParentSourceVersionID,
		versionHash, input.NormalizationVersion, chapterCount, totalChars, key, input.Metadata))
	if err != nil {
		return SourceVersion{}, false, mapPGConflict(err)
	}
	if input.ParentSourceVersionID != nil {
		_, err = tx.Exec(ctx, `INSERT INTO drama.source_version_chapters(
			version_chapter_id,work_id,source_version_id,chapter_id,chapter_revision_id,ordinal,idempotency_key)
		SELECT 'svc_'||encode(digest($1||':'||chapter_id,'sha256'),'hex'),work_id,$1,chapter_id,chapter_revision_id,ordinal,
			$2||':clone:'||chapter_id FROM drama.source_version_chapters WHERE source_version_id=$3`, versionID, key, *input.ParentSourceVersionID)
		if err != nil {
			return SourceVersion{}, false, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return SourceVersion{}, false, mapPGConflict(err)
	}
	return item, true, nil
}

func (s *Store) ListVersionChapters(ctx context.Context, versionID string) ([]ChapterRevision, error) {
	rows, err := s.pool.Query(ctx, `SELECT svc.chapter_id,svc.chapter_revision_id,svc.ordinal,cr.revision_number,cr.title,cr.content_hash,cr.char_count
		FROM drama.source_version_chapters svc JOIN drama.chapter_revisions cr ON cr.chapter_revision_id=svc.chapter_revision_id
		WHERE svc.source_version_id=$1 ORDER BY svc.ordinal`, versionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]ChapterRevision, 0)
	for rows.Next() {
		var item ChapterRevision
		if err := rows.Scan(&item.ChapterID, &item.ChapterRevisionID, &item.Ordinal, &item.RevisionNumber, &item.Title, &item.ContentHash, &item.CharCount); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) ApplyImport(ctx context.Context, versionID string, expectedRevision int, key string, input ImportInput) (Operation, int, error) {
	inputHash, err := hashJSON(input)
	if err != nil {
		return Operation{}, 0, err
	}
	tx, err := s.writer.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Operation{}, 0, err
	}
	defer tx.Rollback(ctx)
	var workID, status string
	var revision int
	if err := tx.QueryRow(ctx, `SELECT work_id,status,resource_revision FROM drama.source_versions WHERE source_version_id=$1 FOR UPDATE`, versionID).
		Scan(&workID, &status, &revision); errors.Is(err, pgx.ErrNoRows) {
		return Operation{}, 0, ErrNotFound
	} else if err != nil {
		return Operation{}, 0, err
	}
	if replay, found, err := getOperationByIdempotency(ctx, tx, key); err != nil {
		return Operation{}, 0, err
	} else if found {
		if replay.TargetID != versionID || replay.OperationType != "source_import" || replay.InputHash != inputHash {
			return Operation{}, 0, ErrConflict
		}
		return replay, revision, nil
	}
	if status != "draft" {
		return Operation{}, 0, ErrImmutable
	}
	if revision != expectedRevision {
		return Operation{}, 0, ErrRevisionConflict
	}
	revision++
	if _, err := tx.Exec(ctx, `UPDATE drama.source_versions SET resource_revision=$2 WHERE source_version_id=$1`, versionID, revision); err != nil {
		return Operation{}, 0, err
	}
	operationID, _ := newPublicID("op_")
	traceID, _ := newPublicID("tr_")
	jobID, _ := newPublicID("imp_")
	queued := input.StorageRef != ""
	opStatus, jobStatus, stage := "completed", "completed", "finished"
	if queued {
		opStatus, jobStatus, stage = "pending", "pending", "queued"
	}
	query := `INSERT INTO drama.operations(operation_id,trace_id,operation_type,target_type,target_id,status,idempotency_key,input_hash,
		checkpoint_stage,checkpoint_data,result_type,result_id,completed_at)
		VALUES($1,$2,'source_import','source_version',$3,$4,$5,$6,$7,$8,$9,$10,CASE WHEN $11::boolean THEN NULL ELSE CURRENT_TIMESTAMP END)`
	resultType, resultID := any("source_version"), any(versionID)
	if queued {
		resultType, resultID = nil, nil
	}
	checkpoint := mustJSON(map[string]any{"completed_items": 0, "total_items": len(input.Items), "mode": input.Mode})
	if queued {
		checkpoint = mustJSON(map[string]any{"completed_items": 0, "total_items": 0, "mode": input.Mode, "storage_ref": input.StorageRef})
	}
	if _, err := tx.Exec(ctx, query, operationID, traceID, versionID, opStatus, key, inputHash, stage,
		checkpoint, resultType, resultID, queued); err != nil {
		return Operation{}, 0, mapPGConflict(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO drama.source_import_jobs(import_job_id,operation_id,work_id,source_version_id,import_mode,status,
		idempotency_key,input_hash,total_items,succeeded_items,completed_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,CASE WHEN $6='completed' THEN CURRENT_TIMESTAMP END)`,
		jobID, operationID, workID, versionID, input.Mode, jobStatus, key, inputHash, len(input.Items), 0); err != nil {
		return Operation{}, 0, mapPGConflict(err)
	}
	if !queued {
		for index, chapter := range input.Items {
			_, _, err := applyChapter(ctx, tx, workID, versionID, key, input.Mode, chapter)
			if err != nil {
				return Operation{}, 0, err
			}
			itemID, _ := newPublicID("impi_")
			// The frozen FK ties populated item outputs to the *current* version
			// membership. Keeping these optional columns NULL avoids an old import
			// item preventing a later draft revision from replacing that membership.
			if _, err := tx.Exec(ctx, `INSERT INTO drama.source_import_items(import_item_id,import_job_id,work_id,source_version_id,
				client_item_key,item_ordinal,status,input_hash,idempotency_key,completed_at)
				VALUES($1,$2,$3,$4,$5,$6,'completed',$7,$8,CURRENT_TIMESTAMP)`, itemID, jobID, workID, versionID,
				chapter.ClientItemKey, index+1, hashText(chapter.Content), key+":item:"+chapter.ClientItemKey); err != nil {
				return Operation{}, 0, mapPGConflict(err)
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE drama.source_import_jobs SET total_items=$2,succeeded_items=$2,
			checkpoint=jsonb_build_object('completed_items',$2::integer,'total_items',$2::integer) WHERE import_job_id=$1`, jobID, len(input.Items)); err != nil {
			return Operation{}, 0, err
		}
		if _, err := tx.Exec(ctx, `UPDATE drama.operations SET checkpoint_data=jsonb_build_object('completed_items',$2::integer,'total_items',$2::integer)
			WHERE operation_id=$1`, operationID, len(input.Items)); err != nil {
			return Operation{}, 0, err
		}
		if err := refreshVersionDigest(ctx, tx, versionID); err != nil {
			return Operation{}, 0, err
		}
	}
	operation, _, err := getOperationByIdempotency(ctx, tx, key)
	if err != nil {
		return Operation{}, 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Operation{}, 0, mapPGConflict(err)
	}
	return operation, revision, nil
}

func applyChapter(ctx context.Context, tx pgx.Tx, workID, versionID, key, mode string, input ChapterInput) (string, string, error) {
	if input.Ordinal < 1 || strings.TrimSpace(input.Title) == "" || input.Content == "" {
		return "", "", ErrConflict
	}
	if mode == "revision" {
		if input.ChapterID == nil {
			return "", "", ErrConflict
		}
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM drama.source_version_chapters WHERE source_version_id=$1 AND chapter_id=$2)`, versionID, *input.ChapterID).Scan(&exists); err != nil {
			return "", "", err
		}
		if !exists {
			return "", "", ErrNotFound
		}
	}
	chapterID := ""
	if input.ChapterID != nil {
		chapterID = *input.ChapterID
		var owner string
		err := tx.QueryRow(ctx, `SELECT work_id FROM drama.source_chapters WHERE chapter_id=$1`, chapterID).Scan(&owner)
		if errors.Is(err, pgx.ErrNoRows) {
			if mode == "revision" {
				return "", "", ErrNotFound
			}
			if _, err := tx.Exec(ctx, `INSERT INTO drama.source_chapters(chapter_id,work_id,canonical_key) VALUES($1,$2,$3)`, chapterID, workID, "client:"+chapterID); err != nil {
				return "", "", mapPGConflict(err)
			}
		} else if err != nil {
			return "", "", err
		} else if owner != workID {
			return "", "", ErrConflict
		}
	} else {
		var err error
		chapterID, err = newPublicID("ch_")
		if err != nil {
			return "", "", err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO drama.source_chapters(chapter_id,work_id,canonical_key) VALUES($1,$2,$3)`, chapterID, workID, "generated:"+chapterID); err != nil {
			return "", "", err
		}
	}
	var revisionNumber int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(max(revision_number),0)+1 FROM drama.chapter_revisions WHERE chapter_id=$1`, chapterID).Scan(&revisionNumber); err != nil {
		return "", "", err
	}
	chapterRevisionID, _ := newPublicID("chr_")
	if _, err := tx.Exec(ctx, `INSERT INTO drama.chapter_revisions(chapter_revision_id,work_id,chapter_id,revision_number,title,content,
		content_hash,char_count,idempotency_key) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, chapterRevisionID, workID, chapterID,
		revisionNumber, strings.TrimSpace(input.Title), input.Content, hashText(input.Content), utf8.RuneCountInString(input.Content), key+":revision:"+input.ClientItemKey); err != nil {
		return "", "", mapPGConflict(err)
	}
	if mode == "revision" {
		if _, err := tx.Exec(ctx, `UPDATE drama.source_version_chapters SET chapter_revision_id=$3 WHERE source_version_id=$1 AND chapter_id=$2`, versionID, chapterID, chapterRevisionID); err != nil {
			return "", "", mapPGConflict(err)
		}
	} else {
		membershipID, _ := newPublicID("svc_")
		if _, err := tx.Exec(ctx, `INSERT INTO drama.source_version_chapters(version_chapter_id,work_id,source_version_id,chapter_id,
			chapter_revision_id,ordinal,idempotency_key) VALUES($1,$2,$3,$4,$5,$6,$7)`, membershipID, workID, versionID, chapterID,
			chapterRevisionID, input.Ordinal, key+":membership:"+input.ClientItemKey); err != nil {
			return "", "", mapPGConflict(err)
		}
	}
	return chapterID, chapterRevisionID, nil
}

func refreshVersionDigest(ctx context.Context, tx pgx.Tx, versionID string) error {
	_, err := tx.Exec(ctx, `UPDATE drama.source_versions v SET chapter_count=x.chapter_count,total_chars=x.total_chars,version_hash=x.version_hash
		FROM (SELECT count(*)::int chapter_count,COALESCE(sum(cr.char_count),0)::int total_chars,
			encode(digest(COALESCE(string_agg(svc.ordinal::text||':'||cr.content_hash,'|' ORDER BY svc.ordinal),''),'sha256'),'hex') version_hash
			FROM drama.source_version_chapters svc JOIN drama.chapter_revisions cr ON cr.chapter_revision_id=svc.chapter_revision_id
			WHERE svc.source_version_id=$1) x WHERE v.source_version_id=$1`, versionID)
	return err
}

func (s *Store) ReviseChapter(ctx context.Context, versionID, chapterID string, expectedRevision int, key, title, content string) (Operation, int, error) {
	var ordinal int
	err := s.pool.QueryRow(ctx, `SELECT ordinal FROM drama.source_version_chapters WHERE source_version_id=$1 AND chapter_id=$2`, versionID, chapterID).Scan(&ordinal)
	if errors.Is(err, pgx.ErrNoRows) {
		return Operation{}, 0, ErrNotFound
	}
	if err != nil {
		return Operation{}, 0, err
	}
	return s.ApplyImport(ctx, versionID, expectedRevision, key, ImportInput{Mode: "revision", Items: []ChapterInput{{
		ClientItemKey: chapterID, ChapterID: &chapterID, Ordinal: ordinal, Title: title, Content: content,
	}}})
}

func (s *Store) PublishSourceVersion(ctx context.Context, versionID string, expectedRevision int, key string) (Operation, int, error) {
	inputHash := hashText("publish:" + versionID)
	tx, err := s.writer.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Operation{}, 0, err
	}
	defer tx.Rollback(ctx)
	var workID, status string
	var revision, chapterCount int
	if err := tx.QueryRow(ctx, `SELECT work_id,status,resource_revision,chapter_count FROM drama.source_versions WHERE source_version_id=$1 FOR UPDATE`, versionID).
		Scan(&workID, &status, &revision, &chapterCount); errors.Is(err, pgx.ErrNoRows) {
		return Operation{}, 0, ErrNotFound
	} else if err != nil {
		return Operation{}, 0, err
	}
	if replay, found, err := getOperationByIdempotency(ctx, tx, key); err != nil {
		return Operation{}, 0, err
	} else if found {
		if replay.TargetID != versionID || replay.OperationType != "source_import" || replay.InputHash != inputHash {
			return Operation{}, 0, ErrConflict
		}
		return replay, revision, nil
	}
	if status != "draft" {
		return Operation{}, 0, ErrImmutable
	}
	if revision != expectedRevision {
		return Operation{}, 0, ErrRevisionConflict
	}
	if chapterCount == 0 {
		return Operation{}, 0, ErrConflict
	}
	revision++
	if _, err := tx.Exec(ctx, `UPDATE drama.source_versions SET status='superseded',is_current=false
		WHERE work_id=$1 AND is_current AND source_version_id<>$2`, workID, versionID); err != nil {
		return Operation{}, 0, err
	}
	if _, err := tx.Exec(ctx, `UPDATE drama.source_versions SET status='published',is_current=true,published_at=CURRENT_TIMESTAMP,
		resource_revision=$2 WHERE source_version_id=$1`, versionID, revision); err != nil {
		return Operation{}, 0, err
	}
	operationID, _ := newPublicID("op_")
	traceID, _ := newPublicID("tr_")
	if _, err := tx.Exec(ctx, `INSERT INTO drama.operations(operation_id,trace_id,operation_type,target_type,target_id,status,idempotency_key,
		input_hash,checkpoint_stage,result_type,result_id,completed_at) VALUES($1,$2,'source_import','source_version',$3,'completed',$4,$5,
		'finished','source_version',$3,CURRENT_TIMESTAMP)`, operationID, traceID, versionID, key, inputHash); err != nil {
		return Operation{}, 0, mapPGConflict(err)
	}
	operation, _, err := getOperationByIdempotency(ctx, tx, key)
	if err != nil {
		return Operation{}, 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Operation{}, 0, mapPGConflict(err)
	}
	return operation, revision, nil
}

func (s *Store) StartIRRun(ctx context.Context, versionID, key string, input IRRunInput) (Operation, error) {
	inputHash, err := hashJSON(input)
	if err != nil {
		return Operation{}, err
	}
	tx, err := s.writer.Begin(ctx)
	if err != nil {
		return Operation{}, err
	}
	defer tx.Rollback(ctx)
	var status string
	if err := tx.QueryRow(ctx, `SELECT status FROM drama.source_versions WHERE source_version_id=$1 FOR UPDATE`, versionID).Scan(&status); errors.Is(err, pgx.ErrNoRows) {
		return Operation{}, ErrNotFound
	} else if err != nil {
		return Operation{}, err
	}
	if replay, found, err := getOperationByIdempotency(ctx, tx, key); err != nil {
		return Operation{}, err
	} else if found {
		var replaySourceVersionID, replayInputHash, replaySchemaVersion, replayExtractorVersion string
		err := tx.QueryRow(ctx, `SELECT source_version_id,input_hash,schema_version,extractor_version
			FROM drama.narrative_ir_revisions WHERE ir_revision_id=$1 AND operation_id=$2`, replay.TargetID, replay.OperationID).
			Scan(&replaySourceVersionID, &replayInputHash, &replaySchemaVersion, &replayExtractorVersion)
		if err != nil || replay.TargetType != "ir_revision" || replay.OperationType != "ir_extraction" ||
			replay.InputHash != inputHash || replaySourceVersionID != versionID || replayInputHash != inputHash ||
			replaySchemaVersion != input.SchemaVersion || replayExtractorVersion != input.ExtractorVersion {
			return Operation{}, ErrConflict
		}
		return replay, nil
	}
	if status != "published" {
		return Operation{}, ErrConflict
	}
	if len(input.ChapterIDs) > 0 {
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM drama.source_version_chapters WHERE source_version_id=$1 AND chapter_id=ANY($2)`, versionID, input.ChapterIDs).Scan(&count); err != nil {
			return Operation{}, err
		}
		if count != len(input.ChapterIDs) {
			return Operation{}, ErrConflict
		}
	}
	operationID, _ := newPublicID("op_")
	traceID, _ := newPublicID("tr_")
	irRevisionID, _ := newPublicID("ir_")
	var workID string
	var revisionNumber int
	if err := tx.QueryRow(ctx, `SELECT work_id FROM drama.source_versions WHERE source_version_id=$1`, versionID).Scan(&workID); err != nil {
		return Operation{}, err
	}
	if err := tx.QueryRow(ctx, `SELECT COALESCE(max(revision_number),0)+1 FROM drama.narrative_ir_revisions WHERE source_version_id=$1`, versionID).
		Scan(&revisionNumber); err != nil {
		return Operation{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO drama.operations(operation_id,trace_id,operation_type,target_type,target_id,status,idempotency_key,input_hash,
		checkpoint_stage,checkpoint_data) VALUES($1,$2,'ir_extraction','ir_revision',$3,'pending',$4,$5,'queued',$6)`, operationID, traceID,
		irRevisionID, key, inputHash, mustJSON(map[string]any{"schema_version": input.SchemaVersion, "extractor_version": input.ExtractorVersion, "chapter_ids": input.ChapterIDs})); err != nil {
		return Operation{}, mapPGConflict(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO drama.narrative_ir_revisions(ir_revision_id,operation_id,work_id,source_version_id,revision_number,
		schema_version,extractor_version,status,input_hash,idempotency_key,validation_summary)
		VALUES($1,$2,$3,$4,$5,$6,$7,'staging',$8,$9,$10)`, irRevisionID, operationID, workID, versionID, revisionNumber,
		input.SchemaVersion, input.ExtractorVersion, inputHash, key,
		mustJSON(map[string]any{"requested_chapter_ids": input.ChapterIDs, "state": "queued"})); err != nil {
		return Operation{}, mapPGConflict(err)
	}
	operation, _, err := getOperationByIdempotency(ctx, tx, key)
	if err != nil {
		return Operation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Operation{}, err
	}
	return operation, nil
}

func (s *Store) GetOperation(ctx context.Context, operationID string) (Operation, error) {
	operation, err := scanOperation(s.pool.QueryRow(ctx, operationSelect+` WHERE operation_id=$1`, operationID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Operation{}, ErrNotFound
	}
	return operation, err
}

const operationSelect = `SELECT operation_id,trace_id,operation_type,target_type,target_id,status,input_hash,checkpoint_stage,checkpoint_cursor,
	checkpoint_data,retry_count,max_retries,lease_expires_at,result_type,result_id,error_code,error_message,error_retryable,created_at,updated_at
	FROM drama.operations`

func scanOperation(row pgx.Row) (Operation, error) {
	var item Operation
	var checkpointData json.RawMessage
	var cursor, resultType, resultID, errorCode, errorMessage *string
	var retryable *bool
	err := row.Scan(&item.OperationID, &item.TraceID, &item.OperationType, &item.TargetType, &item.TargetID, &item.Status, &item.InputHash,
		&item.Checkpoint.Stage, &cursor, &checkpointData, &item.RetryCount, &item.MaxRetries, &item.LeaseExpiresAt,
		&resultType, &resultID, &errorCode, &errorMessage, &retryable, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return item, err
	}
	item.Checkpoint.Cursor = cursor
	var progress struct {
		CompletedItems *int `json:"completed_items"`
		TotalItems     *int `json:"total_items"`
	}
	_ = json.Unmarshal(checkpointData, &progress)
	item.Checkpoint.CompletedItems, item.Checkpoint.TotalItems = progress.CompletedItems, progress.TotalItems
	if resultType != nil && resultID != nil {
		item.ResultRef = &ResultReference{ResourceType: *resultType, ResourceID: *resultID}
	}
	if errorCode != nil && errorMessage != nil {
		item.Error = &OperationError{Code: *errorCode, Message: *errorMessage, Retryable: retryable != nil && *retryable}
	}
	return item, nil
}

func getOperationByIdempotency(ctx context.Context, tx pgx.Tx, key string) (Operation, bool, error) {
	item, err := scanOperation(tx.QueryRow(ctx, operationSelect+` WHERE idempotency_key=$1`, key))
	if errors.Is(err, pgx.ErrNoRows) {
		return Operation{}, false, nil
	}
	return item, err == nil, err
}

func mustJSON(value any) json.RawMessage {
	raw, _ := json.Marshal(value)
	return raw
}

func mapPGConflict(err error) error {
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "SQLSTATE 23") || strings.Contains(err.Error(), "SQLSTATE 40") {
		return fmt.Errorf("%w: %v", ErrConflict, err)
	}
	return err
}
