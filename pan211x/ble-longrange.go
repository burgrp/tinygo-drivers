package pan211x

import "time"

// SpreadFactor selects the BLE Coded PHY spreading factor for transmission.
// It sets the CI field in outgoing packets; the receiver always auto-detects
// the spread factor from the CI field of incoming packets, so mixed S=2/S=8
// networks interoperate transparently.
type SpreadFactor uint8

const (
	SpreadFactorS8 SpreadFactor = 0 // S=8: highest range, ~125 kbps
	SpreadFactorS2 SpreadFactor = 1 // S=2: medium range, ~500 kbps
)

// AddressBLE is a 4-byte BLE access address used as the node identifier.
// It is written to PIPE0_RXADDR (RX filter) during init and to TXADDR on each Send.
type AddressBLE = [4]byte

type ConfigBLELongRange struct {
	SpreadFactor    SpreadFactor
	PayloadLen      uint8
	SerialInterface SerialInterface
}

type DriverBLELongRange struct {
	registers  Registers
	payloadLen uint8
}

func NewDriverBLELongRange(registers Registers) *DriverBLELongRange {
	return &DriverBLELongRange{registers: registers}
}

// Init initialises the chip for BLE LongRange (Coded PHY S2/S8) mode.
// Crystal: 16 MHz. TX power: 9 dBm. Defaults to BLE advertising channel 37 (2402 MHz).
// Caller must call SetBLEChannel after this returns.
func (d *DriverBLELongRange) Init(cfg ConfigBLELongRange) error {
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
	if cfg.SerialInterface == SerialInterfaceSPI3W {
		if err := r.Write(SPI_CFG, SPI_CFG_INIT_3W); err != nil {
			return err
		}
	}
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

	// Step 4: Page 1 pre-configuration — BLE LongRange, 16 MHz crystal.
	// P1_DEMOD_CFG=P1_DEMOD_LR enables the Coded PHY demodulator path; absent in standard BLE Beacon.
	if err := r.Write(P1_DEMOD_CFG, P1_DEMOD_LR); err != nil {
		return err
	}
	if err := r.Write(P1_RF_TUNE_27, 0xAA); err != nil {
		return err
	}
	// P1_RF_TUNE_32 and P1_RF_TUNE_33 (Gaussian filter) are NOT written — BLE uses its own modulator path.
	if err := r.Write(P1_RF_TUNE_37, 0x15); err != nil {
		return err
	}
	if err := r.Write(P1_RF_TUNE_3A, 0x14); err != nil {
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
	if err := r.Write(P1_RF_TUNE_4C, 0x48); err != nil {
		return err
	}

	// Step 5: Page 0 configuration.
	if err := r.Write(PAGE_CFG, 0x00); err != nil {
		return err
	}
	if err := r.Write(XTAL_CFG, (value4>>4)|0xC0); err != nil {
		return err
	}
	if err := r.Write(LP_CFG, 0x0D); err != nil {
		return err
	}
	// WMODE_CFG0 = 0xFC: 3-byte CRC | BLE mode | whitening | CRC_SKIP_ADDR | LSB-first
	if err := r.Write(WMODE_CFG0, CRC_3B|WORK_MODE_BLE|WHITEN_EN_BIT|CRC_SKIP_ADDR_BIT|ENDIAN_LITTLE); err != nil {
		return err
	}
	// WMODE_CFG1 = 0xB2: RX_GOON | 128-byte FIFO | DPY_EN | 4-byte addr
	if err := r.Write(WMODE_CFG1, RX_GOON_BIT|FIFO_128_BIT|DPY_EN_BIT|ADDR_4B); err != nil {
		return err
	}
	// PID_CFG bits[6:4] = ADDR_ERR_THR: 0x00 = exact match, no bit-error tolerance.
	if err := r.Write(PID_CFG, 0x00); err != nil {
		return err
	}
	if err := r.Write(RXPLLEN_CFG, cfg.PayloadLen); err != nil {
		return err
	}
	if err := r.Write(TXPLLEN_CFG, cfg.PayloadLen); err != nil {
		return err
	}
	if err := r.Write(RFIRQ_CFG, 0x7F); err != nil {
		return err
	}
	// PKT_EXT_CFG: HDR_LEN_EXIST=1, 1 auto-header byte, TX+RX FEC enabled, spread factor in bits[1:0].
	pktExt := HDR_LEN_EXIST_BIT | HDR_LEN_1_BIT | PRI_TX_FEC_BIT | PRI_RX_FEC_BIT | uint8(cfg.SpreadFactor)
	if err := r.Write(PKT_EXT_CFG, pktExt); err != nil {
		return err
	}
	// ACCADDR_SCR_DIS must be set for all BLE modes — the access address is never whitened.
	if err := r.Write(WHITEN_CFG, bleWhitenSeed(37)); err != nil {
		return err
	}
	if err := r.Write(TXAUTO_CFG, 0x00); err != nil {
		return err
	}
	// TRXMODE_CFG = 0x40: continuous RX, W_PRE_SYNC_EN=0 (LR demodulator handles preamble detection).
	if err := r.Write(TRXMODE_CFG, RX_CONTINUOUS_BIT); err != nil {
		return err
	}
	if err := r.Write(BLEMATCH_CFG0, BLELEN_EQUAL); err != nil {
		return err
	}
	if err := r.Write(WLIST1_CFG, 0xCC); err != nil {
		return err
	}
	if err := r.Write(WLIST2_CFG, 0xCC); err != nil {
		return err
	}
	if err := r.Write(WLIST3_CFG, 0xCC); err != nil {
		return err
	}
	if err := r.Write(WLIST4_CFG, 0xCC); err != nil {
		return err
	}
	if err := r.Write(WLIST5_CFG, 0xCC); err != nil {
		return err
	}
	if err := r.Write(BLEMATCHSTART_CFG, 0x00); err != nil {
		return err
	}
	if err := r.Write(RF_CHANNEL_CFG, RF_CH_CAL); err != nil {
		return err
	}
	if err := r.Write(RF_PA_MODE_CFG, 0x32); err != nil { // 16 MHz value
		return err
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
	// RF_RSSI_FIX0–3 (0x5A–0x5D) and RF_GAIN_WORD0–3 (0x5E–0x61) NOT written on 16 MHz.
	if err := r.Write(RF_TX_ANA_TIME, 0x64); err != nil { // 16 MHz value
		return err
	}
	if err := r.Write(RF_RX_PLL_SETUP, 0x19); err != nil { // 16 MHz value
		return err
	}
	if err := r.Write(RF_PA_RAMP_DLY, 0x40); err != nil { // 16 MHz value
		return err
	}
	if err := r.Write(MISC_CFG, PID_LOW_SEL_BIT); err != nil {
		return err
	}

	// Step 6: RF calibration — 5 phases in strict order on Page 1.
	return runCalibration(r)
}

// SetChannel sets the RF channel by BLE logical channel index (0–39).
// Updates both RF frequency and whitening seed atomically.
// All 40 BLE channels are valid; channels 37–39 are the standard advertising channels.
func (d *DriverBLELongRange) SetChannel(bleChIndex uint8) error {
	rfCh, ok := bleChannelToRF(bleChIndex)
	if !ok {
		return ErrInvalidChannel
	}
	if err := ensureSTB3(d.registers); err != nil {
		return err
	}
	if err := d.registers.Write(RF_CHANNEL_CFG, rfCh); err != nil {
		return err
	}
	if err := d.registers.Write(WHITEN_CFG, bleWhitenSeed(bleChIndex)); err != nil {
		return err
	}
	return enterRX(d.registers)
}

// bleChannelToRF maps a BLE logical channel index (0–39) to an RF channel number
// suitable for RF_CHANNEL_CFG (center frequency = 2400 + RF_CH MHz).
func bleChannelToRF(ch uint8) (uint8, bool) {
	switch {
	case ch <= 10:
		return 4 + 2*ch, true // 2404–2424 MHz
	case ch <= 36:
		return 28 + 2*(ch-11), true // 2428–2478 MHz (skips 2426 = ch38)
	case ch == 37:
		return RF_CH_BLE_37, true // 2402 MHz
	case ch == 38:
		return RF_CH_BLE_38, true // 2426 MHz
	case ch == 39:
		return RF_CH_BLE_39, true // 2480 MHz
	}
	return 0, false
}

// bleWhitenSeed computes WHITEN_CFG for BLE channel index 0–39.
// Formula from BLE spec: bit_reverse7(ch | 0x40), with ACCADDR_SCR_DIS always set.
func bleWhitenSeed(ch uint8) uint8 {
	x := ch | 0x40
	var r uint8
	for i := 0; i < 7; i++ {
		r = (r << 1) | (x & 1)
		x >>= 1
	}
	return ACCADDR_SCR_DIS_BIT | r
}

// EnableRxAddress sets the receive address for pipe pipeIndex (0–5) and enables the pipe.
// Pipes 0 and 1 use the full 4-byte addr. Pipes 2–5 use only addr[0] (LSB);
// their upper 3 bytes are shared with pipe 1 and must be set via pipe 1 first.
// Calling this for pipe 0 overwrites the NodeAddr set during init.
func (d *DriverBLELongRange) EnableRxAddress(pipeIndex uint8, addr AddressBLE) error {
	return enableRxAddress(d.registers, pipeIndex, addr[:])
}

// DisableRxAddress disables the given pipe without changing its stored address.
func (d *DriverBLELongRange) DisableRxAddress(pipeIndex uint8) error {
	return disableRxAddress(d.registers, pipeIndex)
}

// Send transmits payload to dst. len(payload) must not exceed PayloadLen from config.
// Blocks until TX complete or ~10 ms timeout, then re-enters RX mode.
func (d *DriverBLELongRange) Send(dst AddressBLE, payload []byte) error {
	return send(d.registers, d.payloadLen, dst[:], payload)
}

// Receive checks for a received packet without blocking.
// Returns (n, true) if a packet was available and copied into buf, (0, false) otherwise.
func (d *DriverBLELongRange) Receive(buf []byte) (n int, ok bool) {
	return receive(d.registers, buf)
}
