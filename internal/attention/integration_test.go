package attention

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/openconductorhq/openconductor/internal/llm"
)

// Integration tests for the L2 LLM classifier with real provider APIs.
// These tests are skipped unless the corresponding API key env vars are set.
//
// Run manually:
//   ANTHROPIC_API_KEY=sk-... go test ./internal/attention/ -run TestIntegration -v
//   OPENAI_API_KEY=sk-...   go test ./internal/attention/ -run TestIntegration -v

// terminalScenarios returns test cases with known terminal output and the
// expected classification result.
func terminalScenarios() []struct {
	name     string
	lines    []string
	wantAny  []string // any of these states is acceptable
	wantNone []string // none of these states should appear
} {
	return []struct {
		name     string
		lines    []string
		wantAny  []string
		wantNone []string
	}{
		{
			name: "agent_waiting_for_input",
			lines: []string{
				"╭────────────────────────────────────────╮",
				"│ ✻ Welcome to Claude Code!              │",
				"│                                        │",
				"│ /help for available commands            │",
				"╰────────────────────────────────────────╯",
				"",
				"> ",
			},
			wantAny:  []string{"WAITING_INPUT", "NEEDS_PERMISSION"},
			wantNone: []string{"WORKING"},
		},
		{
			name: "agent_working_spinner",
			lines: []string{
				"✦ Analyzing the codebase structure…",
				"",
				"  Reading files in src/",
				"  Found 42 TypeScript files",
			},
			wantAny:  []string{"WORKING"},
			wantNone: []string{"DONE", "ERROR"},
		},
		{
			name: "agent_asking_permission",
			lines: []string{
				"I'd like to edit the following files:",
				"  - src/app.ts",
				"  - src/config.ts",
				"",
				"Do you want me to proceed? (y/n)",
			},
			wantAny:  []string{"NEEDS_PERMISSION", "WAITING_INPUT"},
			wantNone: []string{"WORKING"},
		},
		{
			name: "agent_task_complete",
			lines: []string{
				"✓ All changes applied successfully.",
				"",
				"Summary:",
				"  - Updated 3 files",
				"  - Added 2 new tests",
				"  - All tests passing",
				"",
				"> ",
			},
			wantAny:  []string{"DONE", "WAITING_INPUT"},
			wantNone: []string{"WORKING", "ERROR"},
		},
		{
			name: "agent_error_state",
			lines: []string{
				"Error: ENOENT: no such file or directory",
				"  at Object.openSync (node:fs:601:3)",
				"",
				"The build failed with exit code 1.",
				"",
				"> ",
			},
			wantAny:  []string{"ERROR", "WAITING_INPUT"},
			wantNone: []string{"WORKING"},
		},
	}
}

func runProviderIntegration(t *testing.T, client llm.Client, providerName string) {
	t.Helper()

	classifier := NewClassifier(client)
	// Allow fast calls in test (no throttle between scenarios).
	classifier.minInterval = 0
	classifier.workingBackoff = 0

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for _, sc := range terminalScenarios() {
		t.Run(sc.name, func(t *testing.T) {
			sessionName := providerName + "_" + sc.name
			result, err := classifier.Classify(ctx, sessionName, sc.lines)
			if err != nil {
				t.Fatalf("%s classify error: %v", providerName, err)
			}

			t.Logf("%s returned: %q", providerName, result)

			// Check result is in wantAny.
			found := false
			for _, want := range sc.wantAny {
				if result == want {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("%s: got %q, want one of %v", providerName, result, sc.wantAny)
			}

			// Check result is not in wantNone.
			for _, bad := range sc.wantNone {
				if result == bad {
					t.Errorf("%s: got unwanted state %q", providerName, bad)
				}
			}
		})
	}
}

func TestIntegrationAnthropicClassifier(t *testing.T) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		t.Skip("ANTHROPIC_API_KEY not set, skipping Anthropic integration test")
	}

	client := llm.NewAnthropicClient(apiKey, "")
	runProviderIntegration(t, client, "anthropic")
}

func TestIntegrationOpenAIClassifier(t *testing.T) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set, skipping OpenAI integration test")
	}

	client := llm.NewOpenAIClient(apiKey, "")
	runProviderIntegration(t, client, "openai")
}

func TestIntegrationGoogleClassifier(t *testing.T) {
	apiKey := os.Getenv("GOOGLE_API_KEY")
	if apiKey == "" {
		t.Skip("GOOGLE_API_KEY not set, skipping Google integration test")
	}

	client, err := llm.NewGoogleClient(context.Background(), apiKey, "")
	if err != nil {
		t.Fatalf("creating Google client: %v", err)
	}
	runProviderIntegration(t, client, "google")
}

// TestIntegrationFullPipeline tests the full Detector → Classifier → LLM
// pipeline with a real provider. Uses whichever API key is available.
func TestIntegrationFullPipeline(t *testing.T) {
	var client llm.Client
	var providerName string

	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		client = llm.NewAnthropicClient(key, "")
		providerName = "anthropic"
	} else if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		client = llm.NewOpenAIClient(key, "")
		providerName = "openai"
	} else if key := os.Getenv("GOOGLE_API_KEY"); key != "" {
		var err error
		client, err = llm.NewGoogleClient(context.Background(), key, "")
		if err != nil {
			t.Fatalf("creating Google client: %v", err)
		}
		providerName = "google"
	} else {
		t.Skip("No LLM API key set (ANTHROPIC_API_KEY, OPENAI_API_KEY, or GOOGLE_API_KEY), skipping")
	}

	t.Logf("Using provider: %s", providerName)

	detector := NewDetector()
	classifier := NewClassifier(client)
	classifier.minInterval = 0
	classifier.workingBackoff = 0
	detector.SetClassifier(classifier)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Scenario: agent prompt (uncertain heuristic → L2 escalation).
	// The "> " suffix triggers Uncertain in heuristics, which should
	// escalate to the LLM classifier.
	lines := []string{
		"╭────────────────────────────╮",
		"│ ✻ Welcome to Claude Code!  │",
		"╰────────────────────────────╯",
		"",
		"> ",
	}

	event, isWorking := detector.Check(ctx, "test-proj", lines, 0, "")
	t.Logf("Full pipeline result: event=%v isWorking=%v", event, isWorking)

	if isWorking {
		t.Error("expected isWorking=false for prompt screen")
	}

	// The LLM should detect this as WAITING_INPUT or NEEDS_PERMISSION.
	if event == nil {
		t.Fatal("expected attention event from full pipeline, got nil")
	}

	validTypes := []AttentionType{NeedsInput, NeedsPermission}
	found := false
	for _, vt := range validTypes {
		if event.Type == vt {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected event type to be NeedsInput or NeedsPermission, got %v", event.Type)
	}

	if !strings.Contains(event.Source, "llm") {
		t.Errorf("expected source to contain 'llm', got %q", event.Source)
	}
}
