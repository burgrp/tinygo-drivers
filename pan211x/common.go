package pan211x

import "errors"

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
