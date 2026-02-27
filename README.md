# WES Structured Logging PoC — Issue #215

Proof-of-concept implementation for the proposal in
[ga4gh/workflow-execution-service-schemas#215](https://github.com/ga4gh/workflow-execution-service-schemas/issues/215).

## The Problem

WES currently cannot describe the *shape* of log data. If an implementation
returns structured logs (OPM, RO-Crate) inside `stdout`/`stderr`, clients
have to guess the format. There is also no single canonical field per logging
level for structured log data.

## This Proposal

Two additive changes to the WES OpenAPI spec (no breaking changes):

1. **`LogSchema` object** — declares the URI, format, and media type of a
   structured log payload.
2. **`structured_log` + `log_schema` fields** on both `RunLog` and `TaskLog`
   — the ONE canonical place for structured log data at each level, with a
   descriptor so clients know the shape.

### Schema inheritance

If a `TaskLog` has no `log_schema`, it inherits the parent `RunLog`'s
`log_schema`. This avoids repeating the same schema declaration on every task
in a large workflow run.

## Files

```
openapi/proposed_log_schema_patch.yaml   # OpenAPI YAML patch — the spec change
internal/logschema/schema.go             # Go types + Validator
internal/logschema/schema_test.go        # Tests covering all scenarios
cmd/demo/main.go                         # Runnable demo
```

## Run locally

```bash
git clone https://github.com/yourusername/wes-logging-schema
cd wes-logging-schema
go mod tidy
go test ./...
go run cmd/demo/main.go
```

## Expected demo output

```
▶ Scenario 1: RunLog with valid RO-Crate structured_log
  → [workflow/ro-crate] ✓ valid (0ms)

▶ Scenario 2: RunLog with valid OPM provenance structured_log
  → [workflow/opm] ✓ valid (0ms)

▶ Scenario 3: structured_log without log_schema (the old WES problem)
  → [workflow/] ✗ invalid: structured_log is set but log_schema is missing

▶ Scenario 4: TaskLog inheriting log_schema from parent RunLog
  Inherited schema: https://w3id.org/ro/crate/1.1
  → [task/ro-crate] ✓ valid (0ms)

▶ Scenario 5: Malformed JSON in structured_log
  → [workflow/opm] ✗ invalid: content does not match media_type "application/json"
```

## Supported formats

| Format | Validator | Key required fields |
|---|---|---|
| `ro-crate` | Structural | `@context`, `@graph` |
| `opm` | Structural | Any PROV key (`wasGeneratedBy`, `used`, etc.) |
| `json-schema` | Media type only | Valid JSON |
| `custom` | Media type only | Valid JSON |

## Why additive-only?

`stdout` and `stderr` are unchanged. Existing WES implementations continue to
work without modification. `structured_log` and `log_schema` are optional
fields — clients that don't understand them ignore them.
