# PAN211x Mode Comparison

All figures assume 9 dBm TX power and a simple chip/PCB antenna.
Range estimates are approximate — actual results depend heavily on antenna quality,
obstructions, and RF environment.

| Mode              | Air data rate | Typical indoor range | Typical outdoor LoS range |
|-------------------|:-------------:|:--------------------:|:-------------------------:|
| XN297L 2 Mbps     | 2 Mbps        | 15–30 m              | 100–250 m                 |
| XN297L 1 Mbps     | 1 Mbps        | 30–60 m              | 300–500 m                 |
| XN297L 250 kbps   | 250 kbps      | 80–150 m             | 800 m – 1.5 km            |
| BLE LR S=2        | 500 kbps      | 80–120 m             | 600 m – 1 km              |
| BLE LR S=8        | 125 kbps      | 200–400 m            | 1.5 – 3 km                |

## Notes

- **XN297L** uses a proprietary packet format (2-byte CRC, 5-byte address, MSB-first).
  No interoperability with BLE or other standards.
- **BLE LR** uses BLE Coded PHY (Bluetooth 5.0). The access address and CI field are
  always S=8 coded; the receiver auto-detects S=2 vs S=8 from the CI field, so S=2 and
  S=8 nodes can coexist on the same network without reconfiguration of receivers.
  `SpreadFactor` in `ConfigBLELongRange` is a TX-only setting.
- **BLE LR vs XN297L 250 kbps**: BLE LR S=8 has ~3–6 dB better receiver sensitivity
  than XN297L 250 kbps, translating to roughly 2× the range at the same data rate class.
  The FEC in Coded PHY is what drives the sensitivity gain.
- **Throughput**: air data rate is the on-air bit rate including all framing. Effective
  payload throughput is lower; at small payload sizes (≤32 bytes) overhead dominates and
  all modes deliver comparable application-level packet rates.
