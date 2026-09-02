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

func TestAgentWorkflowDocumentsWorkspaceBindingInvariant(t *testing.T) {
	workflow := AgentWorkflow()
	for _, expected := range []string{"first valid workspace-scoped call binds the session", "never switch that session to another workspace"} {
		if !strings.Contains(workflow, expected) {
			t.Fatalf("workflow missing workspace binding invariant %q", expected)
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
