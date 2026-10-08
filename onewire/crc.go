package onewire

// CRC8 returns the Dallas/Maxim 1-Wire CRC-8 of data.
func CRC8(data []byte) byte {
	var crc byte
	for _, value := range data {
		crc ^= value
		for range 8 {
			if crc&1 != 0 {
				crc = crc>>1 ^ 0x8c
			} else {
				crc >>= 1
			}
		}
	}
	return crc
}
