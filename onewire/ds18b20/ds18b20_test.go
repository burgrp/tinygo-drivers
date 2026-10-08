package ds18b20

import (
	"testing"

	"github.com/burgrp/tinygo-drivers/onewire"
)

type fakeBus struct {
	resets       []onewire.ResetStatus
	readBits     []bool
	readBytes    []byte
	writtenBits  []bool
	writtenBytes []byte
}

func (bus *fakeBus) Reset() onewire.ResetStatus {
	if len(bus.resets) == 0 {
		return onewire.ResetPresent
	}
	status := bus.resets[0]
	bus.resets = bus.resets[1:]
	return status
}

func (bus *fakeBus) WriteBit(value bool) {
	bus.writtenBits = append(bus.writtenBits, value)
}

func (bus *fakeBus) ReadBit() bool {
	if len(bus.readBits) == 0 {
		return true
	}
	value := bus.readBits[0]
	bus.readBits = bus.readBits[1:]
	return value
}

func (bus *fakeBus) SendByte(value byte) {
	bus.writtenBytes = append(bus.writtenBytes, value)
}

func (bus *fakeBus) ReceiveByte() byte {
	if len(bus.readBytes) == 0 {
		return 0xff
	}
	value := bus.readBytes[0]
	bus.readBytes = bus.readBytes[1:]
	return value
}

func TestReadROM(t *testing.T) {
	rom := validROM()
	bus := &fakeBus{readBytes: rom[:]}
	device := NewSingleDrop(bus)

	got, status := device.ReadROM()
	if status != StatusOK {
		t.Fatalf("ReadROM() status = %v", status)
	}
	if got != rom {
		t.Fatalf("ReadROM() = %x, want %x", got, rom)
	}
	assertBytes(t, bus.writtenBytes, []byte{commandReadROM})
}

func TestReadROMFaults(t *testing.T) {
	rom := validROM()
	badCRC := rom
	badCRC[7] ^= 1
	wrongFamily := rom
	wrongFamily[0] = 0x10
	wrongFamily[7] = onewire.CRC8(wrongFamily[:7])

	for _, test := range []struct {
		name   string
		resets []onewire.ResetStatus
		rom    ROM
		want   Status
	}{
		{name: "no presence", resets: []onewire.ResetStatus{onewire.ResetNoPresence}, want: StatusNoPresence},
		{name: "bus stuck low", resets: []onewire.ResetStatus{onewire.ResetBusStuckLow}, want: StatusBusStuckLow},
		{name: "bad CRC", rom: badCRC, want: StatusROMCRC},
		{name: "wrong family", rom: wrongFamily, want: StatusWrongFamily},
	} {
		t.Run(test.name, func(t *testing.T) {
			bus := &fakeBus{resets: test.resets, readBytes: test.rom[:]}
			device := NewSingleDrop(bus)
			if _, got := device.ReadROM(); got != test.want {
				t.Fatalf("ReadROM() status = %v, want %v", got, test.want)
			}
		})
	}
}

func TestConfigure12Bit(t *testing.T) {
	rom := validROM()
	scratchpad := validScratchpad(0x1f)
	verification := validScratchpad(configuration12Bit)
	readBytes := append(append([]byte{}, rom[:]...), scratchpad[:]...)
	readBytes = append(readBytes, verification[:]...)
	bus := &fakeBus{
		readBits:  []bool{true},
		readBytes: readBytes,
	}
	device := NewSingleDrop(bus)

	if status := device.Configure12Bit(); status != StatusOK {
		t.Fatalf("Configure12Bit() = %v", status)
	}
	assertBytes(t, bus.writtenBytes, []byte{
		commandReadROM,
		commandSkipROM, commandReadPowerSupply,
		commandSkipROM, commandReadScratchpad,
		commandSkipROM, commandWriteScratchpad, scratchpad[2], scratchpad[3], configuration12Bit,
		commandSkipROM, commandReadScratchpad,
	})
}

func TestConfigure12BitLeavesExistingConfiguration(t *testing.T) {
	rom := validROM()
	scratchpad := validScratchpad(configuration12Bit)
	bus := &fakeBus{
		readBits:  []bool{true},
		readBytes: append(append([]byte{}, rom[:]...), scratchpad[:]...),
	}
	device := NewSingleDrop(bus)

	if status := device.Configure12Bit(); status != StatusOK {
		t.Fatalf("Configure12Bit() = %v", status)
	}
	assertBytes(t, bus.writtenBytes, []byte{
		commandReadROM,
		commandSkipROM, commandReadPowerSupply,
		commandSkipROM, commandReadScratchpad,
	})
}

func TestConfigure12BitRejectsParasitePower(t *testing.T) {
	rom := validROM()
	bus := &fakeBus{readBits: []bool{false}, readBytes: rom[:]}
	device := NewSingleDrop(bus)
	if status := device.Configure12Bit(); status != StatusParasitePower {
		t.Fatalf("Configure12Bit() = %v, want %v", status, StatusParasitePower)
	}
}

func TestConfigure12BitVerifiesWrite(t *testing.T) {
	rom := validROM()
	scratchpad := validScratchpad(0x1f)
	readBytes := append(append([]byte{}, rom[:]...), scratchpad[:]...)
	readBytes = append(readBytes, scratchpad[:]...)
	bus := &fakeBus{readBits: []bool{true}, readBytes: readBytes}
	device := NewSingleDrop(bus)

	if status := device.Configure12Bit(); status != StatusConfiguration {
		t.Fatalf("Configure12Bit() = %v, want %v", status, StatusConfiguration)
	}
	if status := device.StartConversion(); status != StatusNotConfigured {
		t.Fatalf("StartConversion() = %v after failed verification, want %v", status, StatusNotConfigured)
	}
}

func TestOperationsRequireConfiguration(t *testing.T) {
	bus := &fakeBus{}
	device := NewSingleDrop(bus)
	if status := device.StartConversion(); status != StatusNotConfigured {
		t.Errorf("StartConversion() = %v, want %v", status, StatusNotConfigured)
	}
	if _, status := device.ReadTemperatureRaw(); status != StatusNotConfigured {
		t.Errorf("ReadTemperatureRaw() = %v, want %v", status, StatusNotConfigured)
	}
	assertBytes(t, bus.writtenBytes, nil)
}

func TestStartConversion(t *testing.T) {
	bus := &fakeBus{}
	device := NewSingleDrop(bus)
	device.configured = true
	if status := device.StartConversion(); status != StatusOK {
		t.Fatalf("StartConversion() = %v", status)
	}
	assertBytes(t, bus.writtenBytes, []byte{commandSkipROM, commandConvertT})
}

func TestBusFaultClearsConfiguration(t *testing.T) {
	bus := &fakeBus{resets: []onewire.ResetStatus{onewire.ResetNoPresence}}
	device := NewSingleDrop(bus)
	device.configured = true
	if status := device.StartConversion(); status != StatusNoPresence {
		t.Fatalf("StartConversion() = %v, want %v", status, StatusNoPresence)
	}
	if status := device.StartConversion(); status != StatusNotConfigured {
		t.Fatalf("second StartConversion() = %v, want %v", status, StatusNotConfigured)
	}
}

func TestReadTemperatureRaw(t *testing.T) {
	for _, test := range []struct {
		name string
		raw  int16
	}{
		{name: "positive", raw: 0x0191},
		{name: "negative", raw: -162},
		{name: "minimum", raw: MinimumTemperatureRaw},
		{name: "maximum", raw: MaximumTemperatureRaw},
	} {
		t.Run(test.name, func(t *testing.T) {
			scratchpad := validScratchpad(configuration12Bit)
			scratchpad[0] = byte(test.raw)
			scratchpad[1] = byte(uint16(test.raw) >> 8)
			scratchpad[8] = onewire.CRC8(scratchpad[:8])
			bus := &fakeBus{readBytes: scratchpad[:]}
			device := NewSingleDrop(bus)
			device.configured = true
			got, status := device.ReadTemperatureRaw()
			if status != StatusOK {
				t.Fatalf("ReadTemperatureRaw() status = %v", status)
			}
			if got != test.raw {
				t.Fatalf("ReadTemperatureRaw() = %d, want %d", got, test.raw)
			}
			assertBytes(t, bus.writtenBytes, []byte{commandSkipROM, commandReadScratchpad})
		})
	}
}

func TestReadTemperatureFaults(t *testing.T) {
	badCRC := validScratchpad(configuration12Bit)
	badCRC[8] ^= 1
	outOfRange := validScratchpad(configuration12Bit)
	outOfRange[0] = 0xd1
	outOfRange[1] = 0x07
	outOfRange[8] = onewire.CRC8(outOfRange[:8])

	for _, test := range []struct {
		name       string
		scratchpad [scratchpadSize]byte
		want       Status
	}{
		{name: "all zero", want: StatusInvalidScratchpad},
		{name: "bad CRC", scratchpad: badCRC, want: StatusScratchpadCRC},
		{name: "out of range", scratchpad: outOfRange, want: StatusTemperatureRange},
	} {
		t.Run(test.name, func(t *testing.T) {
			bus := &fakeBus{readBytes: test.scratchpad[:]}
			device := NewSingleDrop(bus)
			device.configured = true
			if _, got := device.ReadTemperatureRaw(); got != test.want {
				t.Fatalf("ReadTemperatureRaw() status = %v, want %v", got, test.want)
			}
		})
	}
}

func TestStatusIsNotError(t *testing.T) {
	if _, ok := any(StatusOK).(error); ok {
		t.Fatal("Status implements error; successful status would become a non-nil error interface")
	}
}

func validROM() ROM {
	rom := ROM{FamilyCode, 0xff, 0x64, 0x1e, 0x93, 0x16, 0x04}
	rom[7] = onewire.CRC8(rom[:7])
	return rom
}

func validScratchpad(configuration byte) [scratchpadSize]byte {
	scratchpad := [scratchpadSize]byte{0x50, 0x05, 0x4b, 0x46, configuration, 0xff, 0x0c, 0x10}
	scratchpad[8] = onewire.CRC8(scratchpad[:8])
	return scratchpad
}

func assertBytes(t *testing.T, got, want []byte) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("written bytes = %x, want %x", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("written bytes = %x, want %x", got, want)
		}
	}
}
