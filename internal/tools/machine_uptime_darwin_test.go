//go:build darwin

package tools

import "testing"

func TestReadMachineUptimeDarwin(t *testing.T) {
	uptime, err := readMachineUptime()
	if err != nil {
		t.Fatal(err)
	}
	if uptime <= 0 {
		t.Fatalf("uptime=%v", uptime)
	}
}
