package cli

import "testing"

func TestDoctorCommandRegistered(t *testing.T) {
	cmd, _, err := newRootCommand().Find([]string{"doctor"})
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Name() != "doctor" || !cmd.Runnable() {
		t.Fatalf("doctor command = %q runnable=%t", cmd.Name(), cmd.Runnable())
	}
}
