package onewire

const (
	resetLowMicroseconds       = 520
	presenceSampleMicroseconds = 65
	resetRecoveryMicroseconds  = 450
	writeOneLowMicroseconds    = 6
	writeZeroLowMicroseconds   = 65
	readLowMicroseconds        = 3
	readSampleMicroseconds     = 10
	slotMicroseconds           = 70
)

// Hardware provides the target-specific line, timer, and interrupt operations
// required by a 1-Wire master. Microseconds must return a free-running 1 MHz
// counter; wrapping at 16 bits is supported.
type Hardware interface {
	DriveLow()
	Release()
	Read() bool
	Microseconds() uint16
	EnterCritical()
	ExitCritical()
}

// Bus is the byte-oriented interface used by 1-Wire device drivers.
type Bus interface {
	Reset() ResetStatus
	WriteBit(bool)
	ReadBit() bool
	SendByte(byte)
	ReceiveByte() byte
}

// ResetStatus describes the result of a 1-Wire reset and presence sequence.
type ResetStatus uint8

const (
	ResetPresent ResetStatus = iota
	ResetNoPresence
	ResetBusStuckLow
)

// Master implements standard-speed 1-Wire signaling.
type Master struct {
	hardware Hardware
}

// NewMaster creates a 1-Wire master using initialized target hardware.
func NewMaster(hardware Hardware) Master {
	return Master{hardware: hardware}
}

// Reset sends a reset pulse and samples for a device presence pulse.
func (master *Master) Reset() ResetStatus {
	master.hardware.Release()
	master.delayMicroseconds(5)
	if !master.hardware.Read() {
		return ResetBusStuckLow
	}

	master.hardware.EnterCritical()
	master.hardware.DriveLow()
	master.delayMicroseconds(resetLowMicroseconds)
	master.hardware.Release()
	master.delayMicroseconds(presenceSampleMicroseconds)
	present := !master.hardware.Read()
	master.hardware.ExitCritical()

	master.delayMicroseconds(resetRecoveryMicroseconds)
	if !master.hardware.Read() {
		return ResetBusStuckLow
	}
	if !present {
		return ResetNoPresence
	}
	return ResetPresent
}

// SendByte sends a byte least-significant bit first.
func (master *Master) SendByte(value byte) {
	for range 8 {
		master.WriteBit(value&1 != 0)
		value >>= 1
	}
}

// ReceiveByte reads a byte least-significant bit first.
func (master *Master) ReceiveByte() byte {
	var value byte
	for bit := uint8(0); bit < 8; bit++ {
		if master.ReadBit() {
			value |= 1 << bit
		}
	}
	return value
}

// WriteBit writes one standard-speed time slot.
func (master *Master) WriteBit(high bool) {
	master.hardware.EnterCritical()
	master.hardware.DriveLow()
	start := master.hardware.Microseconds()
	if high {
		master.waitSince(start, writeOneLowMicroseconds)
	} else {
		master.waitSince(start, writeZeroLowMicroseconds)
	}
	master.hardware.Release()
	master.hardware.ExitCritical()
	master.waitSince(start, slotMicroseconds)
}

// ReadBit reads one standard-speed time slot.
func (master *Master) ReadBit() bool {
	master.hardware.EnterCritical()
	master.hardware.DriveLow()
	start := master.hardware.Microseconds()
	master.waitSince(start, readLowMicroseconds)
	master.hardware.Release()
	master.waitSince(start, readSampleMicroseconds)
	high := master.hardware.Read()
	master.hardware.ExitCritical()
	master.waitSince(start, slotMicroseconds)
	return high
}

func (master *Master) delayMicroseconds(duration uint16) {
	master.waitSince(master.hardware.Microseconds(), duration)
}

func (master *Master) waitSince(start, duration uint16) {
	for master.hardware.Microseconds()-start < duration {
	}
}
