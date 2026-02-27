// Package main demonstrates the WES Issue #215 proposal:
// structured log validation at both workflow and task level.
//
// Run: go run cmd/demo/main.go
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/animeshs34/wes-logging-schema/internal/logschema"
)

func main() {
	v := &logschema.Validator{}
	exitCode := 0

	fmt.Println("WES Logging Schema PoC")
	fmt.Println()

	// Scenario 1: RunLog with RO-Crate
	fmt.Println("Scenario 1: RunLog with RO-Crate")
	runLog1 := &logschema.RunLog{
		Name:      "variant-calling-pipeline",
		StartTime: "2024-01-01T10:00:00Z",
		EndTime:   "2024-01-01T12:00:00Z",
		ExitCode:  0,
		// Structured log with declared schema
		StructuredLog: `{
			"@context": "https://w3id.org/ro/crate/1.1/context",
			"@graph": [
				{
					"@id": "./",
					"@type": "Dataset",
					"name": "variant-calling-pipeline run 001"
				},
				{
					"@id": "#run-001",
					"@type": "CreateAction",
					"name": "WES Run 001",
					"startTime": "2024-01-01T10:00:00Z",
					"endTime": "2024-01-01T12:00:00Z"
				}
			]
		}`,
		LogSchema: &logschema.LogSchema{
			SchemaURI:     "https://w3id.org/ro/crate/1.1",
			Format:        logschema.FormatROCrate,
			MediaType:     "application/ld+json",
			SchemaVersion: "1.1",
		},
	}
	printResult(v.ValidateRunLog(runLog1))

	// Scenario 2: RunLog with OPM
	fmt.Println("\nScenario 2: RunLog with OPM")
	runLog2 := &logschema.RunLog{
		Name:     "genomic-alignment",
		ExitCode: 0,
		StructuredLog: `{
			"wasGeneratedBy": {
				"id": "alignment-output-001",
				"activity": "bwa-mem2-align",
				"time": "2024-01-01T11:00:00Z"
			},
			"used": {
				"activity": "bwa-mem2-align",
				"entity": "sample-reads-001"
			},
			"agent": {
				"id": "user:researcher-01"
			}
		}`,
		LogSchema: &logschema.LogSchema{
			SchemaURI: "https://www.w3.org/TR/prov-o/",
			Format:    logschema.FormatOPM,
		},
	}
	printResult(v.ValidateRunLog(runLog2))

	// Scenario 3: Missing schema
	fmt.Println("\nScenario 3: Missing schema declaration")
	runLog3 := &logschema.RunLog{
		StructuredLog: `{"wasGeneratedBy": {"id": "run-002"}}`,
		// No LogSchema — client has to GUESS this is OPM
	}
	printResult(v.ValidateRunLog(runLog3))

	// Scenario 4: Schema inheritance
	fmt.Println("\nScenario 4: TaskLog with schema inheritance")
	parentSchema := &logschema.LogSchema{
		SchemaURI: "https://w3id.org/ro/crate/1.1",
		Format:    logschema.FormatROCrate,
	}
	taskLog := &logschema.TaskLog{
		StartTime: "2024-01-01T10:00:00Z",
		EndTime:   "2024-01-01T10:30:00Z",
		ExitCode:  0,
		StructuredLog: `{
			"@context": "https://w3id.org/ro/crate/1.1/context",
			"@graph": [
				{"@id": "#task-bwa-001", "@type": "CreateAction", "name": "BWA-MEM2 task"}
			]
		}`,
		// No LogSchema on task — inherits from parent
	}
	result, err := v.ValidateTaskLog(taskLog, parentSchema)
	if err != nil {
		fmt.Printf("  ERROR: %v\n", err)
		exitCode = 1
	} else {
		fmt.Printf("  Inherited schema: %s\n", parentSchema.SchemaURI)
		fmt.Printf("  Result: %s\n", result)
	}

	// Scenario 5: Malformed JSON
	fmt.Println("\nScenario 5: Malformed JSON")
	runLog5 := &logschema.RunLog{
		StructuredLog: `{this is not valid json`,
		LogSchema: &logschema.LogSchema{
			SchemaURI: "https://www.w3.org/TR/prov-o/",
			Format:    logschema.FormatOPM,
		},
	}
	printResult(v.ValidateRunLog(runLog5))

	fmt.Println("\nExample API Response:")
	exampleRunLog := &logschema.RunLog{
		Name:          "example-workflow",
		StartTime:     "2024-01-01T10:00:00Z",
		EndTime:       "2024-01-01T12:00:00Z",
		ExitCode:      0,
		Stdout:        "https://storage.example.com/stdout.txt",
		StructuredLog: `{"@context":"https://w3id.org/ro/crate/1.1/context","@graph":[]}`,
		LogSchema: &logschema.LogSchema{
			SchemaURI:     "https://w3id.org/ro/crate/1.1",
			Format:        logschema.FormatROCrate,
			MediaType:     "application/ld+json",
			SchemaVersion: "1.1",
		},
	}
	b, _ := json.MarshalIndent(exampleRunLog, "", "  ")
	fmt.Println(string(b))

	os.Exit(exitCode)
}

func printResult(result *logschema.ValidationResult, err error) {
	if err != nil {
		fmt.Printf("  ERROR: %v\n", err)
		return
	}
	if result == nil {
		fmt.Println("  → No structured_log present, skipped")
		return
	}
	fmt.Printf("  → %s\n", result)
	if len(result.Errors) > 0 {
		for _, e := range result.Errors {
			fmt.Printf("    ✗ %s\n", e)
		}
	}
}
