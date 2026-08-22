// Package ws2812 drives WS2812 RGB LEDs.
package ws2812

import "image/color"

func encodeColor(value color.RGBA, brightness uint8) [3]byte {
	red := uint8(uint16(value.R) * uint16(brightness) >> 8)
	green := uint8(uint16(value.G) * uint16(brightness) >> 8)
	blue := uint8(uint16(value.B) * uint16(brightness) >> 8)
	return [3]byte{green, red, blue}
}
