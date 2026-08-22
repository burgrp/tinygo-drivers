//go:build tinygo && py32 && cortexm

package ws2812

import (
	"image/color"
	"machine"
	"runtime/interrupt"
)

// Device drives a chain of WS2812 RGB LEDs from one GPIO pin.
type Device struct {
	Pin        machine.Pin
	brightness uint8
}

// New is kept for compatibility with the TinyGo WS2812 driver.
func New(pin machine.Pin) Device {
	return NewWS2812(pin)
}

// NewWS2812 returns a WS2812 driver. The caller must configure pin as an
// output before writing.
func NewWS2812(pin machine.Pin) Device {
	return Device{Pin: pin, brightness: 255}
}

// SetBrightness sets the brightness applied by WriteColors.
func (device *Device) SetBrightness(brightness uint8) {
	device.brightness = brightness
}

// Write sends bytes without changing their order or value.
func (device Device) Write(buffer []byte) (int, error) {
	writer, err := newWriter(device.Pin)
	if err != nil {
		return 0, err
	}

	state := interrupt.Disable()
	for _, value := range buffer {
		writer.writeByte(value)
	}
	interrupt.Restore(state)
	return len(buffer), nil
}

// WriteByte sends one byte, most-significant bit first.
func (device Device) WriteByte(value byte) error {
	writer, err := newWriter(device.Pin)
	if err != nil {
		return err
	}

	state := interrupt.Disable()
	writer.writeByte(value)
	interrupt.Restore(state)
	return nil
}

// WriteColors sends colors in the GRB byte order used by WS2812 LEDs.
func (device Device) WriteColors(colors []color.RGBA) error {
	writer, err := newWriter(device.Pin)
	if err != nil {
		return err
	}

	state := interrupt.Disable()
	for _, value := range colors {
		encoded := encodeColor(value, device.brightness)
		writer.writeByte(encoded[0])
		writer.writeByte(encoded[1])
		writer.writeByte(encoded[2])
	}
	interrupt.Restore(state)
	return nil
}
