#!/usr/bin/env python3
"""Standards-based Phase 1 JSON Schema validation (requires jsonschema)."""

from __future__ import annotations

import copy
import json
from pathlib import Path

from jsonschema import Draft202012Validator, FormatChecker


ROOT = Path(__file__).resolve().parents[1]
SCHEMA_DIR = ROOT / "contracts" / "json-schema"
FIXTURE_DIR = ROOT / "test-data" / "contracts"
PAIRS = (
    ("workflow-command.v2.json", "workflow-command"),
    ("narrative-extraction.v1.json", "narrative-extraction"),
    ("adaptation-spec.v1.json", "adaptation-spec"),
    ("compiler-plan.v1.json", "compiler-plan"),
    ("worker-execution.v1.json", "worker-execution"),
)


def load_json(path: Path) -> dict:
    return json.loads(path.read_text(encoding="utf-8-sig"))


def assert_invalid(validator: Draft202012Validator, value: dict, label: str) -> None:
    if not list(validator.iter_errors(value)):
        raise AssertionError(f"{label} unexpectedly passed")


def main() -> None:
    validators: dict[str, Draft202012Validator] = {}
    for schema_file, fixture_base in PAIRS:
        schema = load_json(SCHEMA_DIR / schema_file)
        Draft202012Validator.check_schema(schema)
        validator = Draft202012Validator(schema, format_checker=FormatChecker())
        validators[schema_file] = validator

        valid = load_json(FIXTURE_DIR / f"{fixture_base}.valid.json")
        errors = list(validator.iter_errors(valid))
        if errors:
            raise AssertionError(f"{fixture_base}.valid failed: {errors[0].message}")
        assert_invalid(
            validator,
            load_json(FIXTURE_DIR / f"{fixture_base}.invalid.json"),
            f"{fixture_base}.invalid",
        )

    spec = load_json(FIXTURE_DIR / "adaptation-spec.valid.json")
    spec_validator = validators["adaptation-spec.v1.json"]
    empty_scope = copy.deepcopy(spec)
    empty_scope["scope"]["chapter_ids"] = []
    empty_scope["scope"]["story_arc_revision_ids"] = []
    assert_invalid(spec_validator, empty_scope, "adaptation spec empty scope")

    untargeted_rule = copy.deepcopy(spec)
    del untargeted_rule["rules"][0]["target_id"]
    assert_invalid(spec_validator, untargeted_rule, "adaptation spec untargeted rule")

    attribute_rule = copy.deepcopy(spec)
    attribute_rule["rules"][0]["target_type"] = "attribute"
    attribute_rule["rules"][0]["parameters"] = {}
    assert_invalid(spec_validator, attribute_rule, "attribute rule without owner/path")

    print("PASS Phase 1 Draft 2020-12 schemas, formats, fixtures and focused negatives")


if __name__ == "__main__":
    main()
