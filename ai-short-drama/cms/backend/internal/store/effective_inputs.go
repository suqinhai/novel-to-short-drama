package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"short-drama-cms/backend/internal/effectiveinput"
)

func (s *Store) ResolveEffectiveInputs(
	ctx context.Context,
	projectID string,
	episodeID string,
	stage string,
) (json.RawMessage, error) {
	var raw json.RawMessage
	err := s.pool.QueryRow(ctx, `SELECT drama.resolve_effective_inputs($1::text,NULLIF($2::text,''),$3::text)`,
		strings.TrimSpace(projectID), strings.TrimSpace(episodeID), strings.TrimSpace(stage)).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, effectiveinput.ErrNotFound
	}
	if err != nil {
		message := strings.ToUpper(err.Error())
		if strings.Contains(message, "PROJECT_NOT_FOUND") || strings.Contains(message, "EPISODE_NOT_FOUND") {
			return nil, effectiveinput.ErrNotFound
		}
		if strings.Contains(message, "UNSUPPORTED_EFFECTIVE_INPUT_STAGE") {
			return nil, effectiveinput.ErrInvalidRequest
		}
		return nil, err
	}
	return raw, nil
}
