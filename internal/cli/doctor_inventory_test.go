package cli

import (
	"reflect"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/app"
	"go.mewis.me/chatgpt-mcp/internal/tools"
)

func TestDoctorInventoryCoversAppFields(t *testing.T) {
	assertDoctorCoverage(t, reflect.TypeOf(app.App{}), doctorAppCoverage)
}

func TestDoctorInventoryCoversToolsRuntimeFields(t *testing.T) {
	assertDoctorCoverage(t, reflect.TypeOf(tools.Runtime{}), doctorToolsCoverage)
}

func TestDoctorStandaloneInventoryHasUniqueIDs(t *testing.T) {
	if len(doctorStandaloneChecks) == 0 {
		t.Fatal("standalone inventory is empty")
	}
	seen := map[string]bool{}
	for _, id := range doctorStandaloneChecks {
		if id == "" || seen[id] {
			t.Errorf("invalid standalone check %q", id)
		}
		seen[id] = true
	}
}

func assertDoctorCoverage(t *testing.T, rt reflect.Type, coverage map[string]doctorCoverage) {
	t.Helper()
	for i := 0; i < rt.NumField(); i++ {
		name := rt.Field(i).Name
		item, ok := coverage[name]
		if !ok {
			t.Errorf("%s.%s missing from doctor inventory", rt.String(), name)
			continue
		}
		if len(item.Checks) == 0 && item.Parent == "" {
			t.Errorf("%s.%s inventory has neither checks nor parent", rt.String(), name)
		}
	}
	for name := range coverage {
		if _, ok := rt.FieldByName(name); !ok {
			t.Errorf("doctor inventory has stale field %s.%s", rt.String(), name)
		}
	}
}
