package onewire

const commandSearchROM = 0xf0

// ROM is a 64-bit 1-Wire registration number in wire byte order.
type ROM [8]byte

// SearchStatus describes one Search ROM iteration.
type SearchStatus uint8

const (
	SearchFound SearchStatus = iota
	SearchDone
	SearchNoPresence
	SearchBusStuckLow
	SearchDataError
	SearchCRC
)

// Searcher enumerates devices on a 1-Wire bus without allocating memory.
// Call Next until it returns SearchDone. A Searcher can be reused after Reset.
type Searcher struct {
	rom             ROM
	lastDiscrepancy uint8
	found           bool
	done            bool
}

// Reset restarts enumeration from the first ROM.
func (searcher *Searcher) Reset() {
	*searcher = Searcher{}
}

// Next returns the next ROM in deterministic Search ROM order.
func (searcher *Searcher) Next(bus Bus) (ROM, SearchStatus) {
	if searcher.done {
		return ROM{}, SearchDone
	}

	switch bus.Reset() {
	case ResetPresent:
	case ResetNoPresence:
		if !searcher.found {
			searcher.done = true
			return ROM{}, SearchDone
		}
		return ROM{}, SearchNoPresence
	default:
		return ROM{}, SearchBusStuckLow
	}

	bus.SendByte(commandSearchROM)
	rom := searcher.rom
	var lastZero uint8
	for bitNumber := uint8(1); bitNumber <= 64; bitNumber++ {
		bit := bus.ReadBit()
		complement := bus.ReadBit()
		if bit && complement {
			return ROM{}, SearchDataError
		}

		var direction bool
		if bit != complement {
			direction = bit
		} else {
			switch {
			case bitNumber < searcher.lastDiscrepancy:
				direction = romBit(rom, bitNumber)
			case bitNumber == searcher.lastDiscrepancy:
				direction = true
			default:
				direction = false
			}
			if !direction {
				lastZero = bitNumber
			}
		}

		setROMBit(&rom, bitNumber, direction)
		bus.WriteBit(direction)
	}

	if rom[0] == 0 {
		return ROM{}, SearchDataError
	}
	if CRC8(rom[:7]) != rom[7] {
		return ROM{}, SearchCRC
	}
	searcher.rom = rom
	searcher.lastDiscrepancy = lastZero
	searcher.found = true
	searcher.done = lastZero == 0
	return rom, SearchFound
}

func romBit(rom ROM, bitNumber uint8) bool {
	bitNumber--
	return rom[bitNumber/8]&(1<<(bitNumber%8)) != 0
}

func setROMBit(rom *ROM, bitNumber uint8, high bool) {
	bitNumber--
	mask := byte(1 << (bitNumber % 8))
	if high {
		rom[bitNumber/8] |= mask
	} else {
		rom[bitNumber/8] &^= mask
	}
}
