package instructioncontext

import (
	"strings"
	"testing"
)

func TestAgentWorkflowCoversNativeToolFlow(t *testing.T) {
	workflow := AgentWorkflow()
	for _, expected := range []string{
		"MCP session", "project_context", "load_path_rules", "load_skill", "read_files", "read_text_file", "apply_patch", "edit_file", "multi_edit", "run_command", "rewind", "remember", "verify",
	} {
		if !strings.Contains(workflow, expected) {
			t.Fatalf("workflow missing %q: %s", expected, workflow)
		}
	}
}

func TestAgentWorkflowRequiresImmediateRememberOnExplicitUserRequest(t *testing.T) {
	workflow := AgentWorkflow()
	for _, expected := range []string{"explicitly asks to remember", "call remember immediately in that same turn", "do not merely acknowledge or defer"} {
		if !strings.Contains(workflow, expected) {
			t.Fatalf("workflow missing immediate remember guidance %q: %s", expected, workflow)
		}
	}
}

func TestAgentWorkflowDocumentsMultiWorkspaceIsolationInvariant(t *testing.T) {
	workflow := AgentWorkflow()
	for _, expected := range []string{"multiple registered workspaces", "explicitly target workspace_id", "persisted shell cwd", "never carry workspace-specific state into another workspace"} {
		if !strings.Contains(workflow, expected) {
			t.Fatalf("workflow missing multi-workspace isolation invariant %q", expected)
		}
	}
}

func TestAgentWorkflowDoesNotRequireSkillBodiesUpFront(t *testing.T) {
	workflow := AgentWorkflow()
	if strings.Contains(workflow, "load every skill") || strings.Contains(workflow, "all skill bodies") {
		t.Fatalf("workflow eagerly loads skills: %s", workflow)
	}
	if !strings.Contains(workflow, "When a skill is applicable") {
		t.Fatalf("workflow must load only applicable skills: %s", workflow)
	}
}

func TestAgentWorkflowIsStableAndNonEmpty(t *testing.T) {
	if strings.TrimSpace(DefaultAgentWorkflow) == "" || AgentWorkflow() != DefaultAgentWorkflow {
		t.Fatalf("workflow = %q", AgentWorkflow())
	}
}

func TestSharedGuidanceRendersIntoWorkflowAndServerInstructions(t *testing.T) {
	workflow := AgentWorkflow()
	server := StaticServerInstructions()
	steps := SharedGuidanceSteps()
	if len(steps) == 0 {
		t.Fatal("shared guidance is empty")
	}
	for _, step := range steps {
		if !strings.Contains(workflow, step) {
			t.Fatalf("workflow missing shared guidance %q", step)
		}
		if !strings.Contains(server, step) {
			t.Fatalf("server instructions missing shared guidance %q", step)
		}
	}
	for _, expected := range []string{"workspace_register", "workspace_status", "persisted shell cwd", "agent_status", "project_context", "list_skills"} {
		if !strings.Contains(server, expected) {
			t.Fatalf("server instructions missing bootstrap %q: %s", expected, server)
		}
	}
}

func TestSharedGuidanceStepsReturnsCopy(t *testing.T) {
	first := SharedGuidanceSteps()
	first[0] = "mutated"
	second := SharedGuidanceSteps()
	if len(second) == 0 || second[0] == "mutated" {
		t.Fatalf("shared guidance leaked mutable state: %#v", second)
	}
}
