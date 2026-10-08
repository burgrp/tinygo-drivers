// Package ds18b20 drives externally powered DS18B20s on a 1-Wire bus.
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
	commandMatchROM        = 0x55
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

// Validate checks the DS18B20 family code and registration-number CRC.
func (rom ROM) Validate() Status {
	if onewire.CRC8(rom[:7]) != rom[7] {
		return StatusROMCRC
	}
	if rom[0] != FamilyCode {
		return StatusWrongFamily
	}
	return StatusOK
}

// Network drives addressed DS18B20s on a multidrop 1-Wire bus.
type Network struct {
	bus onewire.Bus
}

// NewNetwork creates a driver for externally powered, explicitly addressed
// DS18B20s sharing one bus.
func NewNetwork(bus onewire.Bus) Network {
	return Network{bus: bus}
}

// Configure12Bit selects volatile 12-bit resolution for one addressed sensor.
func (network *Network) Configure12Bit(rom ROM) Status {
	if status := rom.Validate(); status != StatusOK {
		return status
	}
	if status := network.requireExternalPower(rom); status != StatusOK {
		return status
	}

	scratchpad, status := network.readScratchpad(rom)
	if status != StatusOK {
		return status
	}
	if scratchpad[4]&0x60 == 0x60 {
		return StatusOK
	}

	if status := network.selectROM(rom); status != StatusOK {
		return status
	}
	network.bus.SendByte(commandWriteScratchpad)
	network.bus.SendByte(scratchpad[2])
	network.bus.SendByte(scratchpad[3])
	network.bus.SendByte(configuration12Bit)

	verification, status := network.readScratchpad(rom)
	if status != StatusOK {
		return status
	}
	if verification[4]&0x60 != 0x60 {
		return StatusConfiguration
	}
	return StatusOK
}

// RequireExternalPowerAll verifies that every device on the bus has a VDD
// supply. The Read Power Supply response is wired-AND on a multidrop bus.
func (network *Network) RequireExternalPowerAll() Status {
	if status := reset(network.bus); status != StatusOK {
		return status
	}
	network.bus.SendByte(commandSkipROM)
	network.bus.SendByte(commandReadPowerSupply)
	if !network.bus.ReadBit() {
		return StatusParasitePower
	}
	return StatusOK
}

// StartConversionAll broadcasts Convert T to every sensor on the bus.
func (network *Network) StartConversionAll() Status {
	if status := reset(network.bus); status != StatusOK {
		return status
	}
	network.bus.SendByte(commandSkipROM)
	network.bus.SendByte(commandConvertT)
	return StatusOK
}

// ReadTemperatureRaw reads one addressed sensor in signed units of 1/16 °C.
func (network *Network) ReadTemperatureRaw(rom ROM) (int16, Status) {
	if status := rom.Validate(); status != StatusOK {
		return 0, status
	}
	scratchpad, status := network.readScratchpad(rom)
	if status != StatusOK {
		return 0, status
	}
	return decodeTemperature(scratchpad)
}

func (network *Network) requireExternalPower(rom ROM) Status {
	if status := network.selectROM(rom); status != StatusOK {
		return status
	}
	network.bus.SendByte(commandReadPowerSupply)
	if !network.bus.ReadBit() {
		return StatusParasitePower
	}
	return StatusOK
}

func (network *Network) readScratchpad(rom ROM) ([scratchpadSize]byte, Status) {
	if status := network.selectROM(rom); status != StatusOK {
		return [scratchpadSize]byte{}, status
	}
	network.bus.SendByte(commandReadScratchpad)
	return receiveScratchpad(network.bus)
}

func (network *Network) selectROM(rom ROM) Status {
	if status := reset(network.bus); status != StatusOK {
		return status
	}
	network.bus.SendByte(commandMatchROM)
	for _, value := range rom {
		network.bus.SendByte(value)
	}
	return StatusOK
}

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
	if status := rom.Validate(); status != StatusOK {
		return ROM{}, status
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

	return decodeTemperature(scratchpad)
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
	return receiveScratchpad(device.bus)
}

func receiveScratchpad(bus onewire.Bus) ([scratchpadSize]byte, Status) {

	var scratchpad [scratchpadSize]byte
	allZero := true
	for index := range scratchpad {
		scratchpad[index] = bus.ReceiveByte()
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

func decodeTemperature(scratchpad [scratchpadSize]byte) (int16, Status) {
	raw := int16(uint16(scratchpad[0]) | uint16(scratchpad[1])<<8)
	if raw < MinimumTemperatureRaw || raw > MaximumTemperatureRaw {
		return 0, StatusTemperatureRange
	}
	return raw, StatusOK
}

func (device *Device) reset() Status {
	return reset(device.bus)
}

func reset(bus onewire.Bus) Status {
	switch bus.Reset() {
	case onewire.ResetPresent:
		return StatusOK
	case onewire.ResetNoPresence:
		return StatusNoPresence
	default:
		return StatusBusStuckLow
	}
}
