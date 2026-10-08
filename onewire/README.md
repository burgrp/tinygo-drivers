# 1-Wire and DS18B20 drivers

This module provides a target-neutral standard-speed 1-Wire master and an
allocation-free driver for externally powered DS18B20 sensors.

The master does not accept a concrete timer peripheral. TinyGo's generated
timer types and machine APIs differ between targets, so callers implement the
small `onewire.Hardware` interface instead:

```go
type Hardware interface {
	DriveLow()
	Release()
	Read() bool
	Microseconds() uint16
	EnterCritical()
	ExitCritical()
}
```

`Microseconds` must expose a free-running 1 MHz counter. A 16-bit hardware timer
can be returned directly; adapters for wider timers or cycle counters can return
their low 16 bits after scaling. The master calculates elapsed time with wrapping
`uint16` subtraction.

`DriveLow` and `Release` must implement open-drain behavior. `EnterCritical` and
`ExitCritical` must preserve and restore the target's prior interrupt state. The
adapter may retain that state in a field because a master is not concurrent or
reentrant.

The combined interface-dispatch and GPIO latency from a physical edge to a
sample must remain below approximately 3 µs. The default master samples read
slots at 10 µs and presence at 65 µs to leave margin for that overhead and clock
tolerance. Every target adapter must still be checked on a logic analyzer or
oscilloscope.

## DS18B20

The DS18B20 API is split-phase so firmware can do other work during conversion:

```go
master := onewire.NewMaster(hardware)
sensor := ds18b20.NewSingleDrop(&master)

if status := sensor.Configure12Bit(); status != ds18b20.StatusOK {
	// Handle missing, invalid, or parasite-powered sensor.
}
if status := sensor.StartConversion(); status != ds18b20.StatusOK {
	// Handle bus fault.
}

// Continue servicing the application for ds18b20.ConversionWait12Bit.
raw, status := sensor.ReadTemperatureRaw()
if status == ds18b20.StatusOK {
	// raw is a signed Q12.4 value in units of 1/16 degree Celsius.
}
```

For a multidrop bus, enumerate IDs with `onewire.Searcher`, configure each
addressed sensor, then broadcast one conversion and read sensors individually:

```go
var searcher onewire.Searcher
for {
	found, searchStatus := searcher.Next(&master)
	if searchStatus == onewire.SearchDone {
		break
	}
	if searchStatus != onewire.SearchFound {
		// Handle bus or registration-number fault.
		break
	}
	rom := ds18b20.ROM(found)
	if status := rom.Validate(); status != ds18b20.StatusOK {
		continue
	}
	// Store rom in fixed application-owned memory.
}

network := ds18b20.NewNetwork(&master)
if status := network.RequireExternalPowerAll(); status != ds18b20.StatusOK {
	// At least one device requires unsupported parasite power.
}
for _, rom := range configuredROMs {
	if status := network.Configure12Bit(rom); status != ds18b20.StatusOK {
		// Mark this sensor unavailable.
	}
}
if status := network.StartConversionAll(); status == ds18b20.StatusOK {
	// Wait ConversionWait12Bit while continuing to service the application.
}
for _, rom := range configuredROMs {
	raw, status := network.ReadTemperatureRaw(rom)
	_, _ = raw, status
}
```

Each addressed read is a separate transaction. Applications with latency-sensitive
work should return to their main loop between calls rather than reading an entire
bus in one uninterrupted loop.

Import the packages as:

```go
import (
	"github.com/burgrp/tinygo-drivers/onewire"
	"github.com/burgrp/tinygo-drivers/onewire/ds18b20"
)
```

`NewSingleDrop` uses Read ROM once and Skip ROM thereafter; the installation must
guarantee that exactly one device is connected. `NewNetwork` uses Match ROM for
per-sensor operations and Skip ROM only to broadcast Convert T. Both configuration
paths reject parasite power, preserve the alarm bytes, change resolution only in
the volatile scratchpad, and verify the result. Parasite-power strong pull-up and
EEPROM writes are outside this API.

## Validation

Host tests cover reset faults, slot timing, 16-bit counter wrap, bit order,
Dallas CRC-8, command sequences, signed temperatures, and sensor fault cases.
Target adapters still require waveform validation because GPIO and interface
call timing depend on the compiler and MCU clock.

This path-scoped module is released with tags such as `onewire/v1.0.0`.