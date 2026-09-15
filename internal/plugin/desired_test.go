package plugin

import "testing"

func TestManagerAssessDesiredSeparatesSatisfiedMissingIncompatibleAndPending(t *testing.T) {
	store := testStore(t)
	trust := ActivationTrust{Registry: "official", Publisher: "mewisme", Trusted: true}
	for _, id := range []PluginID{"satisfied", "pending"} {
		manifest := testManifest(string(id), "1.0.0", Capability("formatter/"+id))
		if _, err := store.Install(manifest, testPayload(t, string(id))); err != nil {
			t.Fatal(err)
		}
		if err := store.Activate(id, "1.0.0", trust); err != nil {
			t.Fatal(err)
		}
	}
	incompatible := testManifest("incompatible", "1.0.0", "formatter/incompatible")
	incompatible.Requires.ChatGPTMCP = ">=9.0.0"
	if _, err := store.Install(incompatible, testPayload(t, "incompatible")); err != nil {
		t.Fatal(err)
	}
	config, err := LoadConfig(store.layout.ConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	if err := config.SetDesired("missing", "official", "1.0.0", true); err != nil {
		t.Fatal(err)
	}
	if err := config.SetDesired("pending", "official", "1.0.0", false); err != nil {
		t.Fatal(err)
	}
	if err := config.SetDesired("incompatible", "official", "1.0.0", true); err != nil {
		t.Fatal(err)
	}
	if err := WriteConfig(store.layout.ConfigPath(), config); err != nil {
		t.Fatal(err)
	}
	report, err := (&Manager{Store: store}).AssessDesired()
	if err != nil {
		t.Fatal(err)
	}
	if report.Desired != 4 || report.Satisfied != 1 || len(report.Missing) != 1 || report.Missing[0] != "missing" || len(report.Incompatible) != 1 || report.Incompatible[0] != "incompatible" || len(report.Pending) != 1 || report.Pending[0] != "pending" {
		t.Fatalf("desired report = %#v", report)
	}
}
