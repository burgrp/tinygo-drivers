package ws2812

import (
	"image/color"
	"testing"
)

func TestEncodeColorUsesGRBOrder(t *testing.T) {
	got := encodeColor(color.RGBA{R: 0x12, G: 0x34, B: 0x56}, 255)
	want := [3]byte{0x33, 0x11, 0x55}
	if got != want {
		t.Fatalf("encoded color = %x, want %x", got, want)
	}
}

func TestEncodeColorAppliesBrightness(t *testing.T) {
	got := encodeColor(color.RGBA{R: 200, G: 100, B: 50}, 128)
	want := [3]byte{50, 100, 25}
	if got != want {
		t.Fatalf("encoded color = %v, want %v", got, want)
	}
}
