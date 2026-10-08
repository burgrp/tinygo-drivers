package onewire

import "testing"

type searchBus struct {
	devices    []ROM
	active     []ROM
	bitNumber  uint8
	readPhase  uint8
	resetState ResetStatus
}

func (bus *searchBus) Reset() ResetStatus {
	if bus.resetState != ResetPresent {
		return bus.resetState
	}
	if len(bus.devices) == 0 {
		return ResetNoPresence
	}
	return ResetPresent
}

func (bus *searchBus) SendByte(value byte) {
	if value != commandSearchROM {
		panic("unexpected command")
	}
	bus.active = append(bus.active[:0], bus.devices...)
	bus.bitNumber = 1
	bus.readPhase = 0
}

func (bus *searchBus) ReadBit() bool {
	if bus.bitNumber == 0 || bus.bitNumber > 64 {
		return true
	}
	allZero := true
	allOne := true
	for _, rom := range bus.active {
		value := testROMBit(rom, bus.bitNumber)
		allZero = allZero && !value
		allOne = allOne && value
	}

	var result bool
	if bus.readPhase == 0 {
		result = allOne
		bus.readPhase = 1
	} else {
		result = allZero
		bus.readPhase = 0
	}
	return result
}

func (bus *searchBus) WriteBit(direction bool) {
	filtered := bus.active[:0]
	for _, rom := range bus.active {
		if testROMBit(rom, bus.bitNumber) == direction {
			filtered = append(filtered, rom)
		}
	}
	bus.active = filtered
	bus.bitNumber++
}

func (*searchBus) ReceiveByte() byte {
	panic("unexpected ReceiveByte")
}

func TestSearcherEnumeratesROMs(t *testing.T) {
	devices := []ROM{
		validSearchROM(0x28, 0x01),
		validSearchROM(0x28, 0x02),
		validSearchROM(0x28, 0x80),
	}
	bus := &searchBus{devices: devices}
	var searcher Searcher

	var found []ROM
	for {
		rom, status := searcher.Next(bus)
		if status == SearchDone {
			break
		}
		if status != SearchFound {
			t.Fatalf("Next() status = %d", status)
		}
		found = append(found, rom)
	}

	if len(found) != len(devices) {
		t.Fatalf("found %d ROMs, want %d", len(found), len(devices))
	}
	want := []ROM{devices[2], devices[1], devices[0]}
	for index := range want {
		if found[index] != want[index] {
			t.Errorf("ROM %d = %x, want %x", index, found[index], want[index])
		}
	}
}

func TestSearcherCanReset(t *testing.T) {
	rom := validSearchROM(0x28, 0x44)
	bus := &searchBus{devices: []ROM{rom}}
	var searcher Searcher

	if got, status := searcher.Next(bus); status != SearchFound || got != rom {
		t.Fatalf("first Next() = %x, %d", got, status)
	}
	if _, status := searcher.Next(bus); status != SearchDone {
		t.Fatalf("second Next() status = %d, want SearchDone", status)
	}
	searcher.Reset()
	if got, status := searcher.Next(bus); status != SearchFound || got != rom {
		t.Fatalf("Next() after Reset = %x, %d", got, status)
	}
}

func TestSearcherStatuses(t *testing.T) {
	for _, test := range []struct {
		name   string
		bus    searchBus
		status SearchStatus
	}{
		{name: "empty", bus: searchBus{}, status: SearchDone},
		{name: "stuck low", bus: searchBus{resetState: ResetBusStuckLow}, status: SearchBusStuckLow},
	} {
		t.Run(test.name, func(t *testing.T) {
			var searcher Searcher
			if _, status := searcher.Next(&test.bus); status != test.status {
				t.Fatalf("Next() status = %d, want %d", status, test.status)
			}
		})
	}
}

func TestSearcherRejectsBadCRC(t *testing.T) {
	valid := validSearchROM(0x28, 0x55)
	invalid := valid
	invalid[7] ^= 1
	bus := &searchBus{devices: []ROM{invalid}}
	var searcher Searcher
	if _, status := searcher.Next(bus); status != SearchCRC {
		t.Fatalf("Next() status = %d, want SearchCRC", status)
	}
	bus.devices[0] = valid
	if rom, status := searcher.Next(bus); status != SearchFound || rom != valid {
		t.Fatalf("retry Next() = %x, %d, want %x, SearchFound", rom, status, valid)
	}
}

func TestSearcherRejectsZeroFamily(t *testing.T) {
	rom := ROM{}
	rom[7] = CRC8(rom[:7])
	bus := &searchBus{devices: []ROM{rom}}
	var searcher Searcher
	if _, status := searcher.Next(bus); status != SearchDataError {
		t.Fatalf("Next() status = %d, want SearchDataError", status)
	}
}

func validSearchROM(family, serial byte) ROM {
	rom := ROM{family, serial, 2, 3, 4, 5, 6}
	rom[7] = CRC8(rom[:7])
	return rom
}

func testROMBit(rom ROM, bitNumber uint8) bool {
	zeroBased := bitNumber - 1
	value := rom[zeroBased/8]
	return value&(byte(1)<<uint(zeroBased%8)) != 0
}
