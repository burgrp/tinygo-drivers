package pan211x

import "time"

type AddressXN297L [5]byte

type ConfigXN297L struct {
	BitRate         BitRate
	PayloadLen      uint8
	SerialInterface SerialInterface
}

type DriverXN297L struct {
	registers  Registers
	payloadLen uint8
}

func NewDriverXN297L(registers Registers) *DriverXN297L {
	return &DriverXN297L{registers: registers}
}

// Init initialises the chip for XN297L Normal mode (fixed payload, no auto-ACK).
// Crystal: 16 MHz. TX power: 9 dBm. Caller must call SetChannel after this returns.
func (d *DriverXN297L) Init(cfg ConfigXN297L) error {
	d.payloadLen = cfg.PayloadLen
	r := d.registers

	// Step 1: ensure Page 0.
	if err := r.Write(PAGE_CFG, 0x00); err != nil {
		return err
	}

	// Step 2: enter STB3 with soft reset to bring all Page 0 registers to defaults.
	if err := r.Write(STATE_CFG, STATE_STB3_INIT); err != nil {
		return err
	}
	time.Sleep(time.Millisecond)
	if err := r.Write(STATE_CFG, STATE_STB3); err != nil {
		return err
	}
	time.Sleep(time.Millisecond)
	if err := r.Write(SYS_CFG, SYS_CFG_RESET); err != nil {
		return err
	}
	time.Sleep(time.Millisecond)
	if err := r.Write(SYS_CFG, SYS_CFG_RELEASE); err != nil {
		return err
	}
	// Soft reset clears SPI_CFG to its default, which has REG_SPI3_REN=0 (reads disabled).
	// Must re-enable 3-wire SPI reads before any register read operation.
	if cfg.SerialInterface == SerialInterfaceSPI3W {
		if err := r.Write(SPI_CFG, SPI_CFG_INIT_3W); err != nil {
			return err
		}
	}
	// Required for 16 MHz crystal before any Page 1 access.
	if err := r.Write(RF_OSC_CFG, RF_OSC_CFG_16MHZ); err != nil {
		return err
	}

	// Step 3: read eFuse factory calibration from Page 1.
	if err := r.Write(PAGE_CFG, 0x01); err != nil {
		return err
	}
	if err := r.Write(P1_OTP_CTL, OTP_CTL_START); err != nil {
		return err
	}
	if err := r.Write(P1_OTP_DATA, OTP_READ_WORD2); err != nil {
		return err
	}
	value2, err := r.Read(P1_OTP_DATA)
	if err != nil {
		return err
	}
	if err := r.Write(P1_OTP_DATA, OTP_READ_WORD4); err != nil {
		return err
	}
	value4, err := r.Read(P1_OTP_DATA)
	if err != nil {
		return err
	}
	if err := r.Write(P1_OTP_CTL, OTP_CTL_STOP); err != nil {
		return err
	}
	if value2&OTP_VALID_MASK != OTP_VALID_VAL {
		return ErrNoDevice
	}
	// Apply eFuse-derived trim values while still on Page 1.
	calBit := uint8(0)
	if value2&OTP_CAL_MASK == 0 {
		calBit = 1
	}
	if err := r.Write(P1_PA_TUNE_47, 0x83|((value2>>1)&0x70)); err != nil {
		return err
	}
	if err := r.Write(P1_PA_TUNE_43, 0x10|calBit); err != nil {
		return err
	}

	// Step 4: Page 1 pre-configuration — XN297L Normal, 16 MHz crystal.
	if err := r.Write(P1_RF_TUNE_27, 0xAA); err != nil {
		return err
	}
	if err := r.Write(P1_RF_TUNE_32, 0x1E); err != nil {
		return err
	}
	if err := r.Write(P1_RF_TUNE_33, 0x19); err != nil {
		return err
	}
	if err := r.Write(P1_RF_TUNE_37, 0x15); err != nil {
		return err
	}
	p1Tune3A := uint8(0x14)
	if cfg.BitRate == BitRate250Kbps {
		p1Tune3A |= P1_RF_TUNE_3A_FLTRTUNE_250K
	}
	if err := r.Write(P1_RF_TUNE_3A, p1Tune3A); err != nil {
		return err
	}
	if err := r.Write(P1_RF_TUNE_3E, 0xF1); err != nil {
		return err
	}
	if err := r.Write(P1_RF_TUNE_3F, 0xD2); err != nil { // 16 MHz only
		return err
	}
	if err := r.Write(P1_RF_TUNE_40, 0x20); err != nil { // 16 MHz only
		return err
	}
	if err := r.Write(P1_VCO_PA_CTL, P1_VCO_PA_CTL_16MHZ); err != nil {
		return err
	}
	if err := r.Write(P1_PA_BIAS, PA_BIAS_9DBM); err != nil {
		return err
	}
	switch cfg.BitRate {
	case BitRate2Mbps:
		if err := r.Write(P1_TX_DAC, P1_TX_DAC_GC_BIT|P1_TX_DAC_ISEL_4); err != nil {
			return err
		}
		if err := r.Write(P1_RF_TUNE_4C, 0x48|P1_RF_TUNE_4C_TX_DAC_BW); err != nil {
			return err
		}
	case BitRate250Kbps:
		if err := r.Write(P1_TX_DAC, P1_TX_DAC_ISEL_4); err != nil {
			return err
		}
		if err := r.Write(P1_RF_TUNE_4C, 0x48); err != nil {
			return err
		}
	default: // 1 Mbps
		if err := r.Write(P1_RF_TUNE_4C, 0x48); err != nil {
			return err
		}
	}

	// Step 5: Page 0 configuration.
	if err := r.Write(PAGE_CFG, 0x00); err != nil {
		return err
	}
	if err := r.Write(XTAL_CFG, (value4>>4)|0xC0); err != nil {
		return err
	}
	if err := r.Write(LP_CFG, 0x0D); err != nil { // IRQ routed onto SDA (I2C mode)
		return err
	}
	// WMODE_CFG0 = 0x89: 2-byte CRC | XN297L mode | whitening | MSB-first
	if err := r.Write(WMODE_CFG0, CRC_2B|WORK_MODE_XN297L|WHITEN_EN_BIT|ENDIAN_BIG); err != nil {
		return err
	}
	// WMODE_CFG1 = 0xA3: RX_GOON | 128-byte FIFO | Normal (no ENHANCE) | 5-byte addr
	if err := r.Write(WMODE_CFG1, RX_GOON_BIT|FIFO_128_BIT|ADDR_5B); err != nil {
		return err
	}
	if err := r.Write(RXPLLEN_CFG, cfg.PayloadLen); err != nil {
		return err
	}
	if err := r.Write(TXPLLEN_CFG, cfg.PayloadLen); err != nil {
		return err
	}
	if err := r.Write(RFIRQ_CFG, 0x7F); err != nil { // unmask TX_IRQ only; RX flag still set in RFIRQFLG
		return err
	}
	if err := r.Write(TXAUTO_CFG, 0x00); err != nil { // no auto-retransmit
		return err
	}
	if err := r.Write(TRXMODE_CFG, TRXMODE_CFG_NORMAL); err != nil { // single TX, continuous RX
		return err
	}
	if err := r.Write(WHITEN_CFG, WHITEN_DEFAULT); err != nil {
		return err
	}
	if err := r.Write(RF_CHANNEL_CFG, RF_CH_CAL); err != nil { // calibration freq; replaced after Step 6
		return err
	}
	// RF_DATARATE_CFG not written — chip defaults to 1 Mbps (ES_Tool omits it on 16 MHz).
	// RF_PA_MODE_CFG: 16 MHz values (EN_RXADCCLK=0, unlike 32 MHz).
	switch cfg.BitRate {
	case BitRate2Mbps:
		if err := r.Write(RF_PA_MODE_CFG, 0x36); err != nil { // TXPA_MODE=11, FSYNVCO_TXCTK=1, RXFLTR=2
			return err
		}
	case BitRate250Kbps:
		if err := r.Write(RF_PA_MODE_CFG, 0x03); err != nil { // TXPA_MODE=00, RXFLTR=3
			return err
		}
	default: // 1 Mbps
		if err := r.Write(RF_PA_MODE_CFG, 0x32); err != nil { // TXPA_MODE=11, RXFLTR=2
			return err
		}
	}
	if err := r.Write(RF_PA_POUT_CFG, RF_PA_POUT_CFG_9DBM); err != nil {
		return err
	}
	if err := r.Write(RF_RSSI_TH1, 0xDD); err != nil {
		return err
	}
	if err := r.Write(RF_RSSI_TH2, 0xC9); err != nil {
		return err
	}
	if err := r.Write(RF_RSSI_TH3, 0xB7); err != nil {
		return err
	}
	// RF_RSSI_FIX0–3 (0x5A–0x5D) and RF_GAIN_WORD0–3 (0x5E–0x61) are NOT written
	// on 16 MHz — ES_Tool omits them entirely for this crystal frequency.
	if err := r.Write(RF_TX_ANA_TIME, 0x64); err != nil { // 16 MHz value
		return err
	}
	if err := r.Write(RF_RX_PLL_SETUP, 0x19); err != nil { // 16 MHz value
		return err
	}
	if err := r.Write(RF_PA_RAMP_DLY, 0x40); err != nil { // 16 MHz value
		return err
	}

	// Step 6: RF calibration — 5 phases in strict order on Page 1.
	return runCalibration(r)
}

// SetChannel sets the RF channel. ch = frequency_MHz − 2400 (valid 0–83).
func (d *DriverXN297L) SetChannel(channel uint8) error {
	if channel > maxChannel {
		return ErrInvalidChannel
	}
	if err := ensureSTB3(d.registers); err != nil {
		return err
	}
	if err := d.registers.Write(RF_CHANNEL_CFG, channel); err != nil {
		return err
	}
	return enterRX(d.registers)
}

// EnableRxAddress sets the receive address for pipe pipeIndex (0–5) and enables the pipe.
// Pipes 0 and 1 use the full 5-byte addr. Pipes 2–5 use only addr[0] (LSB);
// their upper 4 bytes are shared with pipe 1 and must be set via pipe 1 first.
func (d *DriverXN297L) EnableRxAddress(pipeIndex uint8, addr AddressXN297L) error {
	return enableRxAddress(d.registers, pipeIndex, addr[:])
}

// DisableRxAddress disables the given pipe without changing its stored address.
func (d *DriverXN297L) DisableRxAddress(pipeIndex uint8) error {
	return disableRxAddress(d.registers, pipeIndex)
}

// Send transmits payload to dst. len(payload) must not exceed PayloadLen from config.
// Blocks until TX complete or ~10 ms timeout, then re-enters RX mode.
func (d *DriverXN297L) Send(dst AddressXN297L, payload []byte) error {
	return send(d.registers, d.payloadLen, dst[:], payload)
}

// Receive checks for a received packet without blocking.
// Returns (n, true) if a packet was available and copied into buf, (0, false) otherwise.
func (d *DriverXN297L) Receive(buf []byte) (n int, ok bool) {
	return receive(d.registers, buf)
}
