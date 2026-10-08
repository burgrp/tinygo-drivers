// Package ds18b20 drives one externally powered DS18B20 on a 1-Wire bus.
package ds18b20

import (
	"time"

	"github.com/burgrp/tinygo-drivers/onewire"
)

const (
	FamilyCode             = 0x28
	RawUnitsPerDegree      = 16
	ConversionWait12Bit    = 800 * time.Millisecond
	MinimumTemperatureRaw  = int16(-55 * RawUnitsPerDegree)
	MaximumTemperatureRaw  = int16(125 * RawUnitsPerDegree)
	scratchpadSize         = 9
	commandConvertT        = 0x44
	commandWriteScratchpad = 0x4e
	commandReadPowerSupply = 0xb4
	commandReadScratchpad  = 0xbe
	commandSkipROM         = 0xcc
	commandReadROM         = 0x33
	configuration12Bit     = 0x7f
)

// Status describes a DS18B20 operation result without allocating an error value.
type Status uint8

const (
	StatusOK Status = iota
	StatusBusStuckLow
	StatusNoPresence
	StatusROMCRC
	StatusWrongFamily
	StatusParasitePower
	StatusScratchpadCRC
	StatusInvalidScratchpad
	StatusTemperatureRange
	StatusNotConfigured
	StatusConfiguration
)

func (status Status) String() string {
	switch status {
	case StatusOK:
		return "ok"
	case StatusBusStuckLow:
		return "1-Wire bus stuck low"
	case StatusNoPresence:
		return "no 1-Wire presence pulse"
	case StatusROMCRC:
		return "invalid DS18B20 ROM CRC"
	case StatusWrongFamily:
		return "unexpected 1-Wire family code"
	case StatusParasitePower:
		return "parasite-powered DS18B20 is unsupported"
	case StatusScratchpadCRC:
		return "invalid DS18B20 scratchpad CRC"
	case StatusInvalidScratchpad:
		return "invalid DS18B20 scratchpad"
	case StatusTemperatureRange:
		return "DS18B20 temperature outside device range"
	case StatusNotConfigured:
		return "DS18B20 is not configured"
	case StatusConfiguration:
		return "DS18B20 configuration verification failed"
	default:
		return "unknown DS18B20 status"
	}
}

// ROM is a DS18B20's 64-bit registration number, including family and CRC.
type ROM [8]byte

// Device drives one DS18B20 on a bus with no other 1-Wire devices.
type Device struct {
	bus        onewire.Bus
	configured bool
}

// NewSingleDrop creates a driver for one externally powered DS18B20.
func NewSingleDrop(bus onewire.Bus) Device {
	return Device{bus: bus}
}

// ReadROM reads and validates the only device's registration number.
func (device *Device) ReadROM() (ROM, Status) {
	if status := device.reset(); status != StatusOK {
		return ROM{}, status
	}
	device.bus.SendByte(commandReadROM)

	var rom ROM
	for index := range rom {
		rom[index] = device.bus.ReceiveByte()
	}
	if onewire.CRC8(rom[:7]) != rom[7] {
		return ROM{}, StatusROMCRC
	}
	if rom[0] != FamilyCode {
		return ROM{}, StatusWrongFamily
	}
	return rom, StatusOK
}

// Configure12Bit validates the device and selects volatile 12-bit resolution.
// It does not copy the configuration into the DS18B20 EEPROM.
func (device *Device) Configure12Bit() Status {
	device.configured = false
	if _, status := device.ReadROM(); status != StatusOK {
		return status
	}
	if status := device.requireExternalPower(); status != StatusOK {
		return status
	}

	scratchpad, status := device.readScratchpad()
	if status != StatusOK {
		return status
	}
	if scratchpad[4]&0x60 == 0x60 {
		device.configured = true
		return StatusOK
	}

	if status := device.reset(); status != StatusOK {
		return status
	}
	device.bus.SendByte(commandSkipROM)
	device.bus.SendByte(commandWriteScratchpad)
	device.bus.SendByte(scratchpad[2])
	device.bus.SendByte(scratchpad[3])
	device.bus.SendByte(configuration12Bit)

	verification, status := device.readScratchpad()
	if status != StatusOK {
		return status
	}
	if verification[4]&0x60 != 0x60 {
		return StatusConfiguration
	}
	device.configured = true
	return StatusOK
}

// StartConversion starts a temperature conversion and returns immediately.
func (device *Device) StartConversion() Status {
	if !device.configured {
		return StatusNotConfigured
	}
	if status := device.reset(); status != StatusOK {
		device.configured = false
		return status
	}
	device.bus.SendByte(commandSkipROM)
	device.bus.SendByte(commandConvertT)
	return StatusOK
}

// ReadTemperatureRaw returns temperature in signed units of 1/16 degree Celsius.
func (device *Device) ReadTemperatureRaw() (int16, Status) {
	if !device.configured {
		return 0, StatusNotConfigured
	}
	scratchpad, status := device.readScratchpad()
	if status != StatusOK {
		device.configured = false
		return 0, status
	}

	raw := int16(uint16(scratchpad[0]) | uint16(scratchpad[1])<<8)
	if raw < MinimumTemperatureRaw || raw > MaximumTemperatureRaw {
		return 0, StatusTemperatureRange
	}
	return raw, StatusOK
}

func (device *Device) requireExternalPower() Status {
	if status := device.reset(); status != StatusOK {
		return status
	}
	device.bus.SendByte(commandSkipROM)
	device.bus.SendByte(commandReadPowerSupply)
	if !device.bus.ReadBit() {
		return StatusParasitePower
	}
	return StatusOK
}

func (device *Device) readScratchpad() ([scratchpadSize]byte, Status) {
	if status := device.reset(); status != StatusOK {
		return [scratchpadSize]byte{}, status
	}
	device.bus.SendByte(commandSkipROM)
	device.bus.SendByte(commandReadScratchpad)

	var scratchpad [scratchpadSize]byte
	allZero := true
	for index := range scratchpad {
		scratchpad[index] = device.bus.ReceiveByte()
		allZero = allZero && scratchpad[index] == 0
	}
	if allZero {
		return [scratchpadSize]byte{}, StatusInvalidScratchpad
	}
	if onewire.CRC8(scratchpad[:scratchpadSize-1]) != scratchpad[scratchpadSize-1] {
		return [scratchpadSize]byte{}, StatusScratchpadCRC
	}
	return scratchpad, StatusOK
}

func (device *Device) reset() Status {
	switch device.bus.Reset() {
	case onewire.ResetPresent:
		return StatusOK
	case onewire.ResetNoPresence:
		return StatusNoPresence
	default:
		return StatusBusStuckLow
	}
}
