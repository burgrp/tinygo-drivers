package pan211x

import (
	"errors"
	"runtime"
	"time"
)

func enableRxAddress(r Registers, pipeIndex uint8, addr []byte) error {
	if pipeIndex > 5 {
		return errors.New("invalid pipe index")
	}
	if err := ensureSTB3(r); err != nil {
		return err
	}
	switch pipeIndex {
	case 0:
		if err := writeAddrBytes(r, PIPE0_RXADDR0, addr); err != nil {
			return err
		}
	case 1:
		if err := writeAddrBytes(r, PIPE1_RXADDR0, addr); err != nil {
			return err
		}
	default:
		lsbReg := PIPE2_RXADDR0 + pipeIndex - 2
		if err := r.Write(lsbReg, addr[0]); err != nil {
			return err
		}
	}
	mask, err := r.Read(RXPIPE_CFG)
	if err != nil {
		return err
	}
	if err := r.Write(RXPIPE_CFG, mask|(1<<pipeIndex)); err != nil {
		return err
	}
	return enterRX(r)
}

func disableRxAddress(r Registers, pipeIndex uint8) error {
	if pipeIndex > 5 {
		return errors.New("invalid pipe index")
	}
	if err := ensureSTB3(r); err != nil {
		return err
	}
	mask, err := r.Read(RXPIPE_CFG)
	if err != nil {
		return err
	}
	if err := r.Write(RXPIPE_CFG, mask&^(1<<pipeIndex)); err != nil {
		return err
	}
	return enterRX(r)
}

var (
	ErrPayloadTooLarge = errors.New("payload too large")
	ErrTimeout         = errors.New("radio timeout")
	ErrCalibration     = errors.New("calibration failed")
	ErrNoDevice        = errors.New("no device")
	ErrInvalidChannel  = errors.New("invalid channel")
)

type BitRate uint8

const (
	BitRate250Kbps BitRate = 0
	BitRate1Mbps   BitRate = 1
	BitRate2Mbps   BitRate = 2
)

const maxChannel = 83

// Registers abstracts the physical bus (I2C or SPI) for register access.
type Registers interface {
	Read(reg uint8) (uint8, error)
	Write(reg uint8, value uint8) error
	WriteBuffer(reg uint8, data []byte) error
	ReadBuffer(reg uint8, buf []byte) error
}

type SerialInterface uint8

const (
	SerialInterfaceSPI3W SerialInterface = 0
	SerialInterfaceSPI4W SerialInterface = 1
	SerialInterfaceI2C   SerialInterface = 2
)

func pollBit(r Registers, reg, bit uint8, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		v, err := r.Read(reg)
		if err != nil {
			return err
		}
		if v&bit != 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return ErrTimeout
		}
		runtime.Gosched()
	}
}

func enterRX(r Registers) error {
	if err := r.Write(STATE_CFG, STATE_STB3); err != nil {
		return err
	}
	if err := r.Write(RFIRQFLG, IRQ_ALL); err != nil {
		return err
	}
	return r.Write(STATE_CFG, STATE_RX)
}

func ensureSTB3(r Registers) error {
	return r.Write(STATE_CFG, STATE_STB3)
}

func writeAddrBytes(r Registers, startReg uint8, addr []byte) error {
	for i, b := range addr {
		if err := r.Write(startReg+uint8(i), b); err != nil {
			return err
		}
	}
	return nil
}

// send is the shared TX implementation. dst is written to TXADDR0 when non-nil (XN297L);
// pass nil to skip the address write (BLE LongRange uses a fixed sync word).
func send(r Registers, maxPayload uint8, dst []byte, payload []byte) error {
	if uint8(len(payload)) > maxPayload {
		return ErrPayloadTooLarge
	}
	if err := ensureSTB3(r); err != nil {
		return err
	}
	if len(dst) > 0 {
		if err := writeAddrBytes(r, TXADDR0, dst); err != nil {
			return err
		}
	}
	if err := r.Write(TXPLLEN_CFG, uint8(len(payload))); err != nil {
		return err
	}
	if err := r.WriteBuffer(TRX_FIFO, payload); err != nil {
		return err
	}
	if err := r.Write(RFIRQFLG, IRQ_ALL); err != nil {
		return err
	}
	if err := r.Write(STATE_CFG, STATE_TX); err != nil {
		return err
	}
	txErr := pollBit(r, RFIRQFLG, IRQ_TX, 10*time.Millisecond)
	_ = enterRX(r)
	return txErr
}

// receive is the shared RX poll implementation.
func receive(r Registers, buf []byte) (int, bool) {
	flags, err := r.Read(RFIRQFLG)
	if err != nil || flags&IRQ_RX == 0 {
		return 0, false
	}
	length, err := r.Read(STATUS3)
	if err != nil {
		return 0, false
	}
	if int(length) > len(buf) {
		length = uint8(len(buf))
	}
	if err := r.ReadBuffer(TRX_FIFO, buf[:length]); err != nil {
		return 0, false
	}
	_ = r.Write(RFIRQFLG, IRQ_ALL)
	return int(length), true
}

// runCalibration runs the 5-phase RF calibration sequence on Page 1, then restores
// Page 0 and enters RX. Must be called while on Page 0.
func runCalibration(r Registers) error {
	if err := r.Write(PAGE_CFG, 0x01); err != nil {
		return err
	}

	if err := r.Write(P1_CAL_CTL, CAL_VCO); err != nil {
		return err
	}
	if err := pollBit(r, P1_CAL_STATUS_VCO, CAL_VCO_DONE_BIT, 5*time.Millisecond); err != nil {
		return ErrCalibration
	}

	if err := r.Write(P1_CAL_CTL, CAL_THERMAL); err != nil {
		return err
	}
	time.Sleep(55 * time.Millisecond)

	// Phase 3 requires RX mode; STATE_CFG is shared and accessible from Page 1.
	if err := r.Write(STATE_CFG, STATE_RX); err != nil {
		return err
	}
	time.Sleep(200 * time.Microsecond)
	if err := r.Write(P1_CAL_CTL, CAL_FREQ); err != nil {
		return err
	}
	if err := pollBit(r, P1_CAL_STATUS_DONE, CAL_DONE_BIT, 5*time.Millisecond); err != nil {
		return ErrCalibration
	}

	if err := r.Write(P1_CAL_CTL, CAL_PHASE1); err != nil {
		return err
	}
	if err := pollBit(r, P1_CAL_STATUS_PHASE1, CAL_PHASE1_DONE_BIT, 5*time.Millisecond); err != nil {
		return ErrCalibration
	}

	if err := r.Write(P1_CAL_CTL, CAL_PHASE2); err != nil {
		return err
	}
	if err := pollBit(r, P1_CAL_STATUS_DONE, CAL_DONE_BIT, 5*time.Millisecond); err != nil {
		return ErrCalibration
	}

	if err := r.Write(P1_CAL_CTL, CAL_STOP); err != nil {
		return err
	}
	if err := r.Write(PAGE_CFG, 0x00); err != nil {
		return err
	}
	return enterRX(r)
}

// DumpState prints key register values with decoded field meanings for debugging.
func DumpState(r Registers) {

	println("--- PAN211x ---")

	rd := func(reg uint8) (uint8, bool) {
		v, err := r.Read(reg)
		if err != nil {
			return 0, false
		}
		return v, true
	}

	if v, ok := rd(STATE_CFG); ok {
		s := "unknown"
		switch v {
		case STATE_STB3:
			s = "STB3"
		case STATE_TX:
			s = "TX"
		case STATE_RX:
			s = "RX"
		case STATE_SLEEP:
			s = "SLEEP"
		case STATE_STB3_INIT:
			s = "STB3_INIT"
		}
		println("STATE     :", s)
	}

	if v, ok := rd(SPI_CFG); ok {
		en := "0"
		if v&0x80 != 0 {
			en = "1"
		}
		println("SPI_CFG   : 3wire_ren=" + en)
	}

	if v, ok := rd(WMODE_CFG0); ok {
		crc := "OFF"
		switch v & 0xC0 {
		case CRC_1B:
			crc = "1B"
		case CRC_2B:
			crc = "2B"
		case CRC_3B:
			crc = "3B"
		}
		mode := "XN297L"
		switch v & 0x30 {
		case WORK_MODE_FS01:
			mode = "FS01"
		case WORK_MODE_FS32:
			mode = "FS32/BLE"
		}
		whiten, endian := "0", "LE"
		if v&WHITEN_EN_BIT != 0 {
			whiten = "1"
		}
		if v&ENDIAN_BIG != 0 {
			endian = "BE"
		}
		println("WMODE_CFG0: crc=" + crc + " mode=" + mode + " whiten=" + whiten + " endian=" + endian)
	}

	if v, ok := rd(WMODE_CFG1); ok {
		rxgoon, fifo, dpy, enh := "0", "64B", "0", "0"
		if v&RX_GOON_BIT != 0 {
			rxgoon = "1"
		}
		if v&FIFO_128_BIT != 0 {
			fifo = "128B"
		}
		if v&DPY_EN_BIT != 0 {
			dpy = "1"
		}
		if v&ENHANCE_BIT != 0 {
			enh = "1"
		}
		addr := "?"
		switch v & 0x03 {
		case ADDR_2B:
			addr = "2B"
		case ADDR_3B:
			addr = "3B"
		case ADDR_4B:
			addr = "4B"
		case ADDR_5B:
			addr = "5B"
		}
		println("WMODE_CFG1: rx_goon=" + rxgoon + " fifo=" + fifo + " dpy=" + dpy + " enh=" + enh + " addr=" + addr)
	}

	if v, ok := rd(RXPIPE_CFG); ok {
		pipes := ""
		for i := uint8(0); i < 6; i++ {
			if v>>i&1 != 0 {
				pipes += string([]byte{'0' + i})
			}
		}
		if pipes == "" {
			pipes = "(none)"
		}
		println("RXPIPE_CFG: enabled=" + pipes)
	}

	if v, ok := rd(RF_CHANNEL_CFG); ok {
		println("CHANNEL   : ch=", v, "/ freq=", 2400+int(v), "MHz")
	}

	{
		rx, ok1 := rd(RXPLLEN_CFG)
		tx, ok2 := rd(TXPLLEN_CFG)
		if ok1 && ok2 {
			println("PAYLOAD   : rx=", rx, "tx=", tx, "bytes")
		}
	}

	if v, ok := rd(RF_DATARATE_CFG); ok {
		rate := "1Mbps"
		switch v & DATARATE_BW_MASK {
		case DATARATE_BW_2MBPS:
			rate = "2Mbps"
		case DATARATE_BW_250KBPS:
			rate = "250kbps"
		}
		println("DATARATE  :", rate)
	}

	if v, ok := rd(TRXMODE_CFG); ok {
		tx := "SINGLE"
		if v&TX_CONTINUOUS_BIT != 0 {
			tx = "CW"
		}
		rx := "SINGLE"
		switch v & 0x60 {
		case RX_TIMEOUT_BIT:
			rx = "TIMEOUT"
		case RX_CONTINUOUS_BIT:
			rx = "CONT"
		}
		presync := "0"
		if v&PRE_SYNC_EN_BIT != 0 {
			presync = "1"
		}
		println("TRXMODE   : tx=" + tx + " rx=" + rx + " presync=" + presync)
	}

	if v, ok := rd(RFIRQFLG); ok {
		flags := irqNames(v)
		println("RFIRQFLG  : set=" + flags)
	}

	if v, ok := rd(RFIRQ_CFG); ok {
		masked := irqNames(v)
		println("RFIRQ_CFG : masked=" + masked)
	}

	if v, ok := rd(STATUS0); ok {
		pipe := (v & STATUS0_PIPE_MASK) >> STATUS0_PIPE_SHIFT
		cierr := "0"
		if v&STATUS0_CI_ERR_BIT != 0 {
			cierr = "1"
		}
		extra := ""
		if pipe == STATUS0_PIPE_EMPTY>>STATUS0_PIPE_SHIFT {
			extra = " (FIFO empty)"
		}
		println("STATUS0   : pipe=" + string([]byte{'0' + pipe}) + " ci_err=" + cierr + extra)
	}

	if v, ok := rd(STATUS3); ok {
		println("STATUS3   : rxlen=", v)
	}
}

func irqNames(v uint8) string {
	s := ""
	if v&IRQ_TX != 0 {
		s += " TX"
	}
	if v&IRQ_MAX_RT != 0 {
		s += " MAX_RT"
	}
	if v&IRQ_ADDR_ERR != 0 {
		s += " ADDR_ERR"
	}
	if v&IRQ_CRC_ERR != 0 {
		s += " CRC_ERR"
	}
	if v&IRQ_LEN_ERR != 0 {
		s += " LEN_ERR"
	}
	if v&IRQ_PID_ERR != 0 {
		s += " PID_ERR"
	}
	if v&IRQ_RX_TIMEOUT != 0 {
		s += " RX_TO"
	}
	if v&IRQ_RX != 0 {
		s += " RX"
	}
	if s == "" {
		return "(none)"
	}
	return s[1:] // trim leading space
}
