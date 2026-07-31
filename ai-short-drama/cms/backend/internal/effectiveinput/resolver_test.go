package effectiveinput

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

type fakeRepository struct {
	raw json.RawMessage
	err error
}

func (f fakeRepository) ResolveEffectiveInputs(context.Context, string, string, string) (json.RawMessage, error) {
	return f.raw, f.err
}

func TestResolverPreservesRequiredOptionalAndDiagnosticStates(t *testing.T) {
	raw := json.RawMessage(`{
		"schema_version":"effective-input-resolution.v1",
		"resolver_version":"effective-input-resolver.v1",
		"resolution_id":"eir_1",
		"project_id":"p1",
		"episode_id":"ep1",
		"stage":"image_to_video",
		"mode":"effective",
		"status":"blocked",
		"ready":false,
		"context_hash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"resolution_hash":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		"items":[
			{"kind":"narrative_ir","requirement":"required","state":"resolved",
			 "input_id":"ir1","input_ids":["ir1"],"versions":[1],
			 "content_hash":"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
			 "source_status":"published","content":{},"artifact_ids":[],"blocks":false},
			{"kind":"candidate_selection","requirement":"optional","state":"needs_review",
			 "input_ids":[],"versions":[],"source_status":"unconfirmed","content":{},
			 "artifact_ids":[],"reason":"CANDIDATE_SELECTION_NOT_CONFIRMED","blocks":true}
		],
		"context":{},"missing":[],
		"blockers":[{"kind":"candidate_selection","state":"needs_review",
			"reason":"CANDIDATE_SELECTION_NOT_CONFIRMED"}]
	}`)
	result, err := New(fakeRepository{raw: raw}).Resolve(context.Background(), "p1", "ep1", "09")
	if err != nil {
		t.Fatal(err)
	}
	if result.Ready || result.Status != "blocked" || len(result.Blockers) != 1 {
		t.Fatalf("unexpected resolution: %+v", result)
	}
	if result.Items[0].Requirement != "required" || result.Items[1].Requirement != "optional" ||
		result.Items[1].State != "needs_review" || !result.Items[1].Blocks {
		t.Fatalf("input state distinctions were lost: %+v", result.Items)
	}
}

func TestResolverRejectsMalformedResolvedInput(t *testing.T) {
	raw := json.RawMessage(`{
		"schema_version":"effective-input-resolution.v1",
		"resolver_version":"effective-input-resolver.v1",
		"resolution_id":"eir_1","project_id":"p1","stage":"episode_script",
		"mode":"effective","status":"ready","ready":true,
		"context_hash":"a","resolution_hash":"b",
		"items":[{"kind":"narrative_ir","requirement":"required","state":"resolved",
			"input_ids":[],"versions":[],"source_status":"published","content":{},"artifact_ids":[],
			"blocks":false}],
		"context":{},"missing":[],"blockers":[]
	}`)
	if _, err := New(fakeRepository{raw: raw}).Resolve(context.Background(), "p1", "", "05"); err == nil {
		t.Fatal("malformed resolved item was accepted")
	}
}

func TestResolverValidatesScopeBeforeRepository(t *testing.T) {
	sentinel := errors.New("repository should not run")
	if _, err := New(fakeRepository{err: sentinel}).Resolve(context.Background(), "", "", "05"); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected invalid request, got %v", err)
	}
}
