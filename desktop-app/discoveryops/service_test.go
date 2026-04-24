package discoveryops

import "testing"

func TestAddressForTest(t *testing.T) {
	if got := AddressForTest("192.168.1.12", 26260); got != "192.168.1.12:26260" {
		t.Fatalf("unexpected address: %q", got)
	}
}
