// Package logschema implements the LogSchema proposal for GA4GH WES Issue #215.
// It provides types that mirror the proposed OpenAPI additions and a validator
// that checks structured log payloads against their declared schemas.
package logschema

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Format enumerates well-known log schema formats.
type Format string

const (
	FormatOPM        Format = "opm"         // W3C PROV / Open Provenance Model
	FormatROCrate    Format = "ro-crate"    // Workflow Run RO-Crate
	FormatJSONSchema Format = "json-schema" // Generic JSON Schema
	FormatCustom     Format = "custom"      // Any other format
)

// LogSchema describes the shape of structured log content.
type LogSchema struct {
	// SchemaURI is a resolvable URI pointing to the schema definition.
	// e.g. "https://www.w3.org/TR/prov-o/" for OPM
	SchemaURI string `json:"schema_uri"`

	// Format is a well-known log format identifier.
	Format Format `json:"format,omitempty"`

	// MediaType is the MIME type of the structured_log content.
	// Defaults to "application/json".
	MediaType string `json:"media_type,omitempty"`

	// SchemaVersion allows clients to handle backward-incompatible changes.
	SchemaVersion string `json:"schema_version,omitempty"`
}

// Validate performs basic structural validation of the LogSchema itself.
func (ls *LogSchema) Validate() error {
	if ls.SchemaURI == "" {
		return fmt.Errorf("log_schema.schema_uri is required")
	}
	if !strings.HasPrefix(ls.SchemaURI, "http://") &&
		!strings.HasPrefix(ls.SchemaURI, "https://") {
		return fmt.Errorf("log_schema.schema_uri must be an absolute HTTP/HTTPS URI, got: %q", ls.SchemaURI)
	}
	switch ls.Format {
	case FormatOPM, FormatROCrate, FormatJSONSchema, FormatCustom, "":
		// valid
	default:
		return fmt.Errorf("log_schema.format %q is not a recognised value", ls.Format)
	}
	return nil
}

// MediaTypeOrDefault returns the declared media type or "application/json".
func (ls *LogSchema) MediaTypeOrDefault() string {
	if ls.MediaType == "" {
		return "application/json"
	}
	return ls.MediaType
}

// RunLog mirrors the WES RunLog with structured logging support.
type RunLog struct {
	Name      string   `json:"name,omitempty"`
	Cmd       []string `json:"cmd,omitempty"`
	StartTime string   `json:"start_time,omitempty"`
	EndTime   string   `json:"end_time,omitempty"`

	Stdout   string `json:"stdout,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
	ExitCode int    `json:"exit_code,omitempty"`

	// StructuredLog is the canonical location for machine-readable logs.
	StructuredLog string `json:"structured_log,omitempty"`

	// LogSchema describes the shape of StructuredLog.
	LogSchema *LogSchema `json:"log_schema,omitempty"`
}

// Log is the per-executor attempt log.
type Log struct {
	StartTime string `json:"start_time,omitempty"`
	EndTime   string `json:"end_time,omitempty"`
	Stdout    string `json:"stdout,omitempty"`
	Stderr    string `json:"stderr,omitempty"`
	ExitCode  int    `json:"exit_code,omitempty"`
}

// TaskLog mirrors the WES TaskLog with structured logging support.
type TaskLog struct {
	Logs      []Log             `json:"logs,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	StartTime string            `json:"start_time,omitempty"`
	EndTime   string            `json:"end_time,omitempty"`

	Stdout   string `json:"stdout,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
	ExitCode int    `json:"exit_code,omitempty"`

	// StructuredLog is the canonical location for machine-readable logs.
	StructuredLog string `json:"structured_log,omitempty"`

	// LogSchema describes the shape of StructuredLog. If absent,
	// it should be inherited from the parent RunLog.
	LogSchema *LogSchema `json:"log_schema,omitempty"`
}

// Validator checks structured log payloads against schemas.
type ValidationResult struct {
	Valid   bool
	Level   string // "workflow" or "task"
	Format  Format
	Errors  []string
	Elapsed time.Duration
}

// String returns a human-readable summary of the validation result.
func (v *ValidationResult) String() string {
	if v.Valid {
		return fmt.Sprintf("[%s/%s] ✓ valid (%s)", v.Level, v.Format, v.Elapsed)
	}
	return fmt.Sprintf("[%s/%s] ✗ invalid: %s", v.Level, v.Format, strings.Join(v.Errors, "; "))
}

// Validator validates structured log payloads against their declared schemas.
type Validator struct {
	// HTTPClient is used to resolve external schema URIs.
	// Defaults to a client with a 10s timeout if nil.
	HTTPClient *http.Client
}

func (v *Validator) httpClient() *http.Client {
	if v.HTTPClient != nil {
		return v.HTTPClient
	}
	return &http.Client{Timeout: 10 * time.Second}
}

// ValidateRunLog validates the structured_log of a RunLog against its
// declared log_schema. Returns nil ValidationResult if no structured_log
// is present (nothing to validate).
func (v *Validator) ValidateRunLog(rl *RunLog) (*ValidationResult, error) {
	if rl.StructuredLog == "" {
		return nil, nil // nothing to validate
	}
	if rl.LogSchema == nil {
		// structured_log is present but no schema declared — warn but don't error.
		return &ValidationResult{
			Valid:  false,
			Level:  "workflow",
			Errors: []string{"structured_log is set but log_schema is missing — clients cannot determine log shape"},
		}, nil
	}
	return v.validate("workflow", rl.StructuredLog, rl.LogSchema)
}

// ValidateTaskLog validates the structured_log of a TaskLog.
// parentSchema is the RunLog's LogSchema, used if the TaskLog has no
// schema of its own (schema inheritance).
func (v *Validator) ValidateTaskLog(tl *TaskLog, parentSchema *LogSchema) (*ValidationResult, error) {
	if tl.StructuredLog == "" {
		return nil, nil
	}

	schema := tl.LogSchema
	if schema == nil {
		// Inherit from parent RunLog if available.
		schema = parentSchema
	}
	if schema == nil {
		return &ValidationResult{
			Valid:  false,
			Level:  "task",
			Errors: []string{"structured_log is set but no log_schema found (neither on task nor inherited from run)"},
		}, nil
	}
	return v.validate("task", tl.StructuredLog, schema)
}

// validate is the shared core validation logic.
func (v *Validator) validate(level, content string, schema *LogSchema) (*ValidationResult, error) {
	start := time.Now()
	result := &ValidationResult{
		Level:  level,
		Format: schema.Format,
	}

	// Step 1: validate the LogSchema itself is well-formed.
	if err := schema.Validate(); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("invalid log_schema: %v", err))
		result.Elapsed = time.Since(start)
		return result, nil
	}

	// Step 2: validate the content is parseable as its declared media type.
	mediaType := schema.MediaTypeOrDefault()
	if err := v.validateMediaType(content, mediaType); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("content does not match media_type %q: %v", mediaType, err))
		result.Elapsed = time.Since(start)
		return result, nil
	}

	// Step 3: format-specific structural validation.
	if err := v.validateByFormat(content, schema.Format); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("format validation failed: %v", err))
		result.Elapsed = time.Since(start)
		return result, nil
	}

	result.Valid = true
	result.Elapsed = time.Since(start)
	return result, nil
}

// validateMediaType checks that content is parseable for the declared type.
// Supports both inline content and resource URIs.
func (v *Validator) validateMediaType(content, mediaType string) error {
	// If content is a URI, skip structural validation of the string itself.
	if strings.HasPrefix(content, "http://") || strings.HasPrefix(content, "https://") {
		return nil
	}

	switch mediaType {
	case "application/json", "application/ld+json":
		var raw json.RawMessage
		if err := json.Unmarshal([]byte(content), &raw); err != nil {
			return fmt.Errorf("not valid JSON: %w", err)
		}
	}
	return nil
}

// validateByFormat does light structural checks for known formats.
func (v *Validator) validateByFormat(content string, format Format) error {
	// Cannot validate structure if content is a remote URI.
	if strings.HasPrefix(content, "http://") || strings.HasPrefix(content, "https://") {
		return nil
	}

	switch format {
	case FormatOPM:
		return validateOPM(content)
	case FormatROCrate:
		return validateROCrate(content)
	}
	return nil
}

// validateOPM checks for minimum required W3C PROV-O / OPM fields.
// A valid OPM document should have at least one of: wasGeneratedBy,
// used, wasAssociatedWith, actedOnBehalfOf.
func validateOPM(content string) error {
	var doc map[string]interface{}
	if err := json.Unmarshal([]byte(content), &doc); err != nil {
		return err
	}
	provKeys := []string{"wasGeneratedBy", "used", "wasAssociatedWith",
		"entity", "activity", "agent"}
	for _, k := range provKeys {
		if _, ok := doc[k]; ok {
			return nil // found at least one PROV key
		}
	}
	return fmt.Errorf("no W3C PROV keys found (expected at least one of: %s)",
		strings.Join(provKeys, ", "))
}

// validateROCrate checks for minimum required RO-Crate fields.
// A valid RO-Crate metadata file must have @context and @graph.
func validateROCrate(content string) error {
	var doc map[string]interface{}
	if err := json.Unmarshal([]byte(content), &doc); err != nil {
		return err
	}
	if _, ok := doc["@context"]; !ok {
		return fmt.Errorf("missing required field '@context' for RO-Crate")
	}
	if _, ok := doc["@graph"]; !ok {
		return fmt.Errorf("missing required field '@graph' for RO-Crate")
	}
	return nil
}

// FetchRemoteSchema fetches and returns the raw schema content from the
// declared schema_uri. Useful for clients that want to do full validation.
func (v *Validator) FetchRemoteSchema(schema *LogSchema) ([]byte, error) {
	resp, err := v.httpClient().Get(schema.SchemaURI)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch schema from %q: %w", schema.SchemaURI, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("schema URI %q returned HTTP %d", schema.SchemaURI, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read schema body: %w", err)
	}
	return body, nil
}
