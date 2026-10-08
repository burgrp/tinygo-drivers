package onewire

import "testing"

func TestCRC8(t *testing.T) {
	data := []byte{0x50, 0x05, 0x4b, 0x46, 0x7f, 0xff, 0x0c, 0x10}
	if got := CRC8(data); got != 0x1c {
		t.Fatalf("CRC8(%x) = %#02x, want 0x1c", data, got)
	}
}
