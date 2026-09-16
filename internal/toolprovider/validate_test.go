package toolprovider

import "testing"

func TestFloorRiskCannotDowngradeExecutePermission(t *testing.T) {
	if got := FloorRisk(RiskRead, []string{"process/execute"}); got != RiskCommand {
		t.Fatalf("got %q", got)
	}
}

func TestFloorRiskAllowsHigherDeclaredRisk(t *testing.T) {
	if got := FloorRisk(RiskDestructive, nil); got != RiskDestructive {
		t.Fatalf("got %q", got)
	}
}

func TestValidateDescribeRejectsUnknownRisk(t *testing.T) {
	err := ValidateDescribe(DescribeResult{Tools: []Tool{{
		Name: "demo_turn", Title: "Demo", InputSchema: []byte(`{"type":"object"}`), OutputSchema: []byte(`{"type":"object"}`), Risk: "harmless",
	}}})
	if err == nil {
		t.Fatal("accepted unknown risk")
	}
}

func TestValidateDescribeRejectsDuplicateNames(t *testing.T) {
	tool := Tool{Name: "demo_turn", Title: "Demo", InputSchema: []byte(`{"type":"object"}`), OutputSchema: []byte(`{"type":"object"}`), Risk: RiskRead}
	err := ValidateDescribe(DescribeResult{Tools: []Tool{tool, tool}})
	if err == nil {
		t.Fatal("accepted duplicate tools")
	}
}
