# 1-Wire and DS18B20 drivers

This module provides a target-neutral standard-speed 1-Wire master and an
allocation-free driver for one externally powered DS18B20.

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

Import the packages as:

```go
import (
	"github.com/burgrp/tinygo-drivers/onewire"
	"github.com/burgrp/tinygo-drivers/onewire/ds18b20"
)
```

The current DS18B20 driver intentionally supports one device on the bus. It uses
Read ROM to validate the family code and ROM CRC, then Skip ROM for subsequent
commands. The installation must guarantee that exactly one device is connected;
Read ROM cannot prove that independently. `Configure12Bit` rejects parasite
power, preserves the alarm bytes, changes resolution only in the volatile
scratchpad, and verifies the result. It must succeed before conversion or
temperature methods are used. A bus or scratchpad fault clears that validation.
Search ROM, multidrop, parasite-power strong pull-up, and EEPROM writes are
outside this API.

## Validation

Host tests cover reset faults, slot timing, 16-bit counter wrap, bit order,
Dallas CRC-8, command sequences, signed temperatures, and sensor fault cases.
Target adapters still require waveform validation because GPIO and interface
call timing depend on the compiler and MCU clock.

This path-scoped module is released with tags such as `onewire/v1.0.0`.