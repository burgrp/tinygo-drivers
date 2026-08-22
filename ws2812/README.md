# WS2812 driver

This module drives WS2812 RGB LEDs from 24 MHz PY32 Cortex-M targets. Its API is
compatible with the common TinyGo WS2812 driver subset: `New`, `NewWS2812`,
`Device.Write`, `WriteByte`, `WriteColors`, and `SetBrightness`.

```go
package main

import (
	"image/color"
	"machine"

	"github.com/burgrp/tinygo-drivers/ws2812"
)

func main() {
	machine.PB7.Configure(machine.PinConfig{Mode: machine.PinOutput})
	led := ws2812.NewWS2812(machine.PB7)
	_ = led.WriteColors([]color.RGBA{{R: 0x10, G: 0x0c}})
}
```

The caller must configure the data pin as an output. `Write` sends raw bytes,
while `WriteColors` encodes colors in WS2812 GRB order and applies the configured
brightness.

## Timing

The PY32 backend requires a 24 MHz CPU clock and writes through the GPIO BSRR
register. Its Cortex-M timing loop has been verified in the generated machine
code:

| Interval | Cycles | Time at 24 MHz |
| --- | ---: | ---: |
| T0H | 9-11 | 375.0-458.3 ns |
| T1H | 18-20 | 750.0-833.3 ns |
| Bit cell | 30 | 1250.0 ns |

Interrupts are disabled for an entire buffer so an interrupt cannot insert a
reset-length gap within a pixel stream. Interrupt latency therefore grows by
about 30 microseconds per RGB LED.

The timing model is derived from the TinyGo WS2812 driver generator and is
distributed under the license in [`LICENSE`](LICENSE).

## Validation

The 24 MHz waveform and RGB byte order were validated on a PY32F030 board with
a WS2812B powered at 3.3 V. Red, green, and blue were each confirmed visually.
Because 3.3 V is below the linked WS2812B part's specified supply range, each
board and LED lot still requires electrical validation.