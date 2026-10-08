package onewire

import "testing"

type hardwareEvent struct {
	name string
	time uint16
}

type fakeHardware struct {
	now    uint16
	reads  []bool
	events []hardwareEvent
}

func (hardware *fakeHardware) DriveLow() {
	hardware.record("low")
}

func (hardware *fakeHardware) Release() {
	hardware.record("release")
}

func (hardware *fakeHardware) Read() bool {
	hardware.record("read")
	if len(hardware.reads) == 0 {
		return true
	}
	value := hardware.reads[0]
	hardware.reads = hardware.reads[1:]
	return value
}

func (hardware *fakeHardware) Microseconds() uint16 {
	hardware.now++
	return hardware.now
}

func (hardware *fakeHardware) EnterCritical() {
	hardware.record("enter")
}

func (hardware *fakeHardware) ExitCritical() {
	hardware.record("exit")
}

func (hardware *fakeHardware) record(name string) {
	hardware.events = append(hardware.events, hardwareEvent{name: name, time: hardware.now})
}

func TestReset(t *testing.T) {
	for _, test := range []struct {
		name  string
		reads []bool
		want  ResetStatus
	}{
		{name: "present", reads: []bool{true, false, true}, want: ResetPresent},
		{name: "no presence", reads: []bool{true, true, true}, want: ResetNoPresence},
		{name: "initially stuck low", reads: []bool{false}, want: ResetBusStuckLow},
		{name: "stuck low after presence", reads: []bool{true, false, false}, want: ResetBusStuckLow},
	} {
		t.Run(test.name, func(t *testing.T) {
			hardware := &fakeHardware{reads: test.reads}
			master := NewMaster(hardware)
			if got := master.Reset(); got != test.want {
				t.Fatalf("Reset() = %d, want %d", got, test.want)
			}
		})
	}
}

func TestResetTiming(t *testing.T) {
	hardware := &fakeHardware{reads: []bool{true, false, true}}
	master := NewMaster(hardware)
	if status := master.Reset(); status != ResetPresent {
		t.Fatalf("Reset() = %d, want ResetPresent", status)
	}

	low := findEvent(t, hardware.events, "low", 0)
	release := findEvent(t, hardware.events, "release", 1)
	if elapsed := release.time - low.time; elapsed < resetLowMicroseconds {
		t.Errorf("reset low = %d us, want at least %d us", elapsed, resetLowMicroseconds)
	}
	enter := findEvent(t, hardware.events, "enter", 0)
	exit := findEvent(t, hardware.events, "exit", 0)
	if elapsed := exit.time - enter.time; elapsed < resetLowMicroseconds+presenceSampleMicroseconds {
		t.Errorf("reset critical section = %d us, want at least %d us", elapsed, resetLowMicroseconds+presenceSampleMicroseconds)
	}
}

func TestSendByteIsLSBFirst(t *testing.T) {
	hardware := &fakeHardware{}
	master := NewMaster(hardware)
	master.SendByte(0x01)

	pulses := lowPulseDurations(hardware.events)
	if len(pulses) != 8 {
		t.Fatalf("low pulse count = %d, want 8", len(pulses))
	}
	if pulses[0] >= writeZeroLowMicroseconds {
		t.Errorf("first pulse = %d us, want write-one pulse", pulses[0])
	}
	for index, duration := range pulses[1:] {
		if duration < writeZeroLowMicroseconds {
			t.Errorf("pulse %d = %d us, want write-zero pulse", index+1, duration)
		}
	}
}

func TestReceiveByteIsLSBFirst(t *testing.T) {
	hardware := &fakeHardware{reads: []bool{true, false, true, false, false, true, false, true}}
	master := NewMaster(hardware)
	if got := master.ReceiveByte(); got != 0xa5 {
		t.Fatalf("ReceiveByte() = %#02x, want 0xa5", got)
	}
}

func TestSlotTimingAcrossCounterWrap(t *testing.T) {
	hardware := &fakeHardware{now: 0xfff0}
	master := NewMaster(hardware)
	start := hardware.now
	master.WriteBit(false)
	if elapsed := hardware.now - start; elapsed < slotMicroseconds {
		t.Fatalf("wrapped slot = %d us, want at least %d us", elapsed, slotMicroseconds)
	}
}

func findEvent(t *testing.T, events []hardwareEvent, name string, occurrence int) hardwareEvent {
	t.Helper()
	for _, event := range events {
		if event.name != name {
			continue
		}
		if occurrence == 0 {
			return event
		}
		occurrence--
	}
	t.Fatalf("event %q not found", name)
	return hardwareEvent{}
}

func lowPulseDurations(events []hardwareEvent) []uint16 {
	var pulses []uint16
	var low uint16
	inPulse := false
	for _, event := range events {
		switch event.name {
		case "low":
			low = event.time
			inPulse = true
		case "release":
			if inPulse {
				pulses = append(pulses, event.time-low)
				inPulse = false
			}
		}
	}
	return pulses
}
