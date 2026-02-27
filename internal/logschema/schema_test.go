package logschema_test

import (
	"testing"

	"github.com/animeshs34/wes-logging-schema/internal/logschema"
)

// LogSchema.Validate() tests

func TestLogSchema_Validate(t *testing.T) {
	tests := []struct {
		name    string
		schema  logschema.LogSchema
		wantErr bool
	}{
		{
			name:    "valid opm schema",
			schema:  logschema.LogSchema{SchemaURI: "https://www.w3.org/TR/prov-o/", Format: logschema.FormatOPM},
			wantErr: false,
		},
		{
			name:    "valid ro-crate schema",
			schema:  logschema.LogSchema{SchemaURI: "https://w3id.org/ro/crate/1.1", Format: logschema.FormatROCrate},
			wantErr: false,
		},
		{
			name:    "missing schema_uri",
			schema:  logschema.LogSchema{Format: logschema.FormatOPM},
			wantErr: true,
		},
		{
			name:    "relative schema_uri not allowed",
			schema:  logschema.LogSchema{SchemaURI: "/relative/path"},
			wantErr: true,
		},
		{
			name:    "invalid format enum",
			schema:  logschema.LogSchema{SchemaURI: "https://example.com/schema", Format: "not-a-format"},
			wantErr: true,
		},
		{
			name:    "empty format is allowed (optional field)",
			schema:  logschema.LogSchema{SchemaURI: "https://example.com/schema"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.schema.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// Validator.ValidateRunLog() tests

func TestValidator_ValidateRunLog(t *testing.T) {
	v := &logschema.Validator{}

	t.Run("no structured_log returns nil — nothing to validate", func(t *testing.T) {
		rl := &logschema.RunLog{Stdout: "plain text log"}
		result, err := v.ValidateRunLog(rl)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result != nil {
			t.Errorf("expected nil result, got %v", result)
		}
	})

	t.Run("structured_log without log_schema returns invalid result", func(t *testing.T) {
		rl := &logschema.RunLog{
			StructuredLog: `{"wasGeneratedBy": {}}`,
		}
		result, err := v.ValidateRunLog(rl)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Valid {
			t.Error("expected invalid result when schema is missing")
		}
	})

	t.Run("valid RO-Crate structured_log passes validation", func(t *testing.T) {
		rl := &logschema.RunLog{
			StructuredLog: `{
				"@context": "https://w3id.org/ro/crate/1.1/context",
				"@graph": [
					{"@id": "./", "@type": "Dataset"}
				]
			}`,
			LogSchema: &logschema.LogSchema{
				SchemaURI: "https://w3id.org/ro/crate/1.1",
				Format:    logschema.FormatROCrate,
			},
		}
		result, err := v.ValidateRunLog(rl)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Valid {
			t.Errorf("expected valid result, got errors: %v", result.Errors)
		}
	})

	t.Run("invalid RO-Crate missing @graph fails validation", func(t *testing.T) {
		rl := &logschema.RunLog{
			StructuredLog: `{"@context": "https://w3id.org/ro/crate/1.1/context"}`,
			LogSchema: &logschema.LogSchema{
				SchemaURI: "https://w3id.org/ro/crate/1.1",
				Format:    logschema.FormatROCrate,
			},
		}
		result, err := v.ValidateRunLog(rl)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Valid {
			t.Error("expected invalid result for RO-Crate missing @graph")
		}
	})

	t.Run("valid OPM structured_log passes validation", func(t *testing.T) {
		rl := &logschema.RunLog{
			StructuredLog: `{"wasGeneratedBy": {"id": "run-001", "activity": "workflow"}}`,
			LogSchema: &logschema.LogSchema{
				SchemaURI: "https://www.w3.org/TR/prov-o/",
				Format:    logschema.FormatOPM,
			},
		}
		result, err := v.ValidateRunLog(rl)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Valid {
			t.Errorf("expected valid result, got errors: %v", result.Errors)
		}
	})

	t.Run("malformed JSON in structured_log fails validation", func(t *testing.T) {
		rl := &logschema.RunLog{
			StructuredLog: `{not valid json`,
			LogSchema: &logschema.LogSchema{
				SchemaURI: "https://www.w3.org/TR/prov-o/",
				Format:    logschema.FormatOPM,
			},
		}
		result, err := v.ValidateRunLog(rl)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Valid {
			t.Error("expected invalid result for malformed JSON")
		}
	})

	t.Run("URI in structured_log passes validation", func(t *testing.T) {
		rl := &logschema.RunLog{
			StructuredLog: "https://storage.example.com/logs/structured.json",
			LogSchema: &logschema.LogSchema{
				SchemaURI: "https://w3id.org/ro/crate/1.1",
				Format:    logschema.FormatROCrate,
			},
		}
		result, err := v.ValidateRunLog(rl)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Valid {
			t.Errorf("expected valid result for URI log content, got: %v", result.Errors)
		}
	})
}

// Validator.ValidateTaskLog() tests (inheritance)

func TestValidator_ValidateTaskLog(t *testing.T) {
	v := &logschema.Validator{}

	parentSchema := &logschema.LogSchema{
		SchemaURI: "https://w3id.org/ro/crate/1.1",
		Format:    logschema.FormatROCrate,
	}

	validROCrate := `{
		"@context": "https://w3id.org/ro/crate/1.1/context",
		"@graph": [{"@id": "./", "@type": "Dataset"}]
	}`

	t.Run("task inherits schema from parent RunLog", func(t *testing.T) {
		tl := &logschema.TaskLog{
			StructuredLog: validROCrate,
			// No LogSchema on task — should inherit from parent
		}
		result, err := v.ValidateTaskLog(tl, parentSchema)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Valid {
			t.Errorf("expected valid via schema inheritance, got: %v", result.Errors)
		}
	})

	t.Run("task schema overrides parent schema", func(t *testing.T) {
		tl := &logschema.TaskLog{
			// OPM content
			StructuredLog: `{"entity": {"id": "task-001"}}`,
			// Task declares its own OPM schema, overriding parent RO-Crate
			LogSchema: &logschema.LogSchema{
				SchemaURI: "https://www.w3.org/TR/prov-o/",
				Format:    logschema.FormatOPM,
			},
		}
		result, err := v.ValidateTaskLog(tl, parentSchema)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Valid {
			t.Errorf("expected valid with task-level OPM schema override, got: %v", result.Errors)
		}
	})

	t.Run("task with no schema and no parent returns invalid", func(t *testing.T) {
		tl := &logschema.TaskLog{
			StructuredLog: validROCrate,
		}
		result, err := v.ValidateTaskLog(tl, nil) // no parent schema
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Valid {
			t.Error("expected invalid when no schema is available anywhere")
		}
	})
}
