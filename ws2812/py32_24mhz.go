//go:build tinygo && py32 && cortexm

package ws2812

import (
	"errors"
	"machine"
	"unsafe"
)

/*
#include <stdint.h>

__attribute__((always_inline))
void ws2812_writeByte24(char c, uint32_t *portSet, uint32_t *portClear,
		uint32_t maskSet, uint32_t maskClear) {
	// Generated with the tinygo-org/drivers WS2812 Cortex-M timing model.
	// T0H:  9 - 11 cycles or 375.0ns - 458.3ns
	// T1H: 18 - 20 cycles or 750.0ns - 833.3ns
	// Bit cell: 30 cycles or 1250.0ns
	uint32_t value = (uint32_t)c << 24;
	char i = 8;
	__asm__ __volatile__(
		"1: @ send_bit\n"
		"\tstr   %[maskSet], %[portSet]\n"
		"\tnop\n"
		"\tnop\n"
		"\tnop\n"
		"\tnop\n"
		"\tnop\n"
		"\tlsls  %[value], #1\n"
		"\tbcs.n 2f\n"
		"\tstr   %[maskClear], %[portClear]\n"
		"2: @ skip_store\n"
		"\tnop\n"
		"\tnop\n"
		"\tnop\n"
		"\tnop\n"
		"\tnop\n"
		"\tnop\n"
		"\tnop\n"
		"\tnop\n"
		"\tnop\n"
		"\tstr   %[maskClear], %[portClear]\n"
		"\tnop\n"
		"\tnop\n"
		"\tnop\n"
		"\tsubs  %[i], #1\n"
		"\tbeq.n 3f\n"
		"\tb     1b\n"
		"3: @ end\n"
	: [value] "+r" (value),
	  [i] "+r" (i)
	: [maskSet] "r" (maskSet),
	  [portSet] "m" (*portSet),
	  [maskClear] "r" (maskClear),
	  [portClear] "m" (*portClear));
}
*/
import "C"

const (
	py32GPIOBase       = uintptr(0x50000000)
	py32GPIOPortStride = uintptr(0x400)
	py32GPIOBSRROffset = uintptr(0x18)
	py32PortCount      = 6
)

var (
	errUnsupportedClock = errors.New("ws2812: PY32 backend requires a 24 MHz CPU clock")
	errInvalidPin       = errors.New("ws2812: invalid PY32 GPIO pin")
)

type writer struct {
	portSet   *uint32
	portClear *uint32
	maskSet   uint32
	maskClear uint32
}

func newWriter(pin machine.Pin) (writer, error) {
	if machine.CPUFrequency() != 24_000_000 {
		return writer{}, errUnsupportedClock
	}

	pinNumber := uint8(pin)
	portNumber := pinNumber >> 4
	if portNumber >= py32PortCount {
		return writer{}, errInvalidPin
	}
	bitNumber := pinNumber & 0x0f
	port := py32GPIOBase + uintptr(portNumber)*py32GPIOPortStride + py32GPIOBSRROffset
	register := (*uint32)(unsafe.Pointer(port))
	return writer{
		portSet:   register,
		portClear: register,
		maskSet:   uint32(1) << bitNumber,
		maskClear: uint32(1) << (bitNumber + 16),
	}, nil
}

func (writer writer) writeByte(value byte) {
	C.ws2812_writeByte24(
		C.char(value),
		(*C.uint32_t)(unsafe.Pointer(writer.portSet)),
		(*C.uint32_t)(unsafe.Pointer(writer.portClear)),
		C.uint32_t(writer.maskSet),
		C.uint32_t(writer.maskClear),
	)
}
