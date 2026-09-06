//go:build darwin

package systeminfo

import "testing"

func TestUptimeDarwin(t *testing.T) {
	uptime, err := Uptime()
	if err != nil {
		t.Fatal(err)
	}
	if uptime <= 0 {
		t.Fatalf("uptime=%v", uptime)
	}
}
