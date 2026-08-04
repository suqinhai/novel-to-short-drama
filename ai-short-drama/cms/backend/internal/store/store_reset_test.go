package store

import "testing"

func TestResetPreservesSystemDictionaries(t *testing.T) {
	for _, table := range []string{
		"artifact_types",
		"effective_input_stage_requirements",
		"migration_audit",
		"schema_migrations",
	} {
		if _, preserved := preservedBusinessTables[table]; !preserved {
			t.Errorf("system table %q is not preserved by business-data reset", table)
		}
	}
}
