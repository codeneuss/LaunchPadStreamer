package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gitlab.com/gomidi/midi/v2"
	_ "gitlab.com/gomidi/midi/v2/drivers/rtmididrv"
)

var pads = make(map[uint8]Pad)

type Pad struct {
	pos       PadPos
	color     uint8
	lightMode uint8
}

type PadPos struct {
	row uint8
	col uint8
}

func NewPad(pos PadPos) Pad {
	return Pad{
		pos:       pos,
		color:     0,
		lightMode: Permanent,
	}
}

func (p *Pad) getKey() uint8 {
	return p.pos.row*10 + p.pos.col
}

func PadPosFromKey(key uint8) PadPos {
	return PadPos{uint8(key / 10), uint8(key % 10)}
}

const (
	Permanent = iota
	Blinking
	Pulsing
)

const On = true
const Off = false

var Send func(msg midi.Message) error

func main() {
	defer midi.CloseDriver()

	out, err := midi.FindOutPort("LPMiniMK3 MIDI In")
	if err != nil {
		fmt.Printf("MIDI Error: %v\n", err)
		os.Exit(1)
	}
	// LED auf Pad setzen (Note On, Kanal 1)
	Send, _ = midi.SendTo(out)

	in, err := midi.FindInPort("LPMiniMK3 MIDI Out")
	if err != nil {
		fmt.Printf("MIDI Error: %v\n", err)
		os.Exit(1)
	}

	stop, _ := midi.ListenTo(in, midiNoteReceived)
	defer stop()

	sysex := []byte{0xF0, 0x00, 0x20, 0x29, 0x02, 0x0D, 0x0E, 0x01, 0xF7}
	Send(sysex)

	clearPad()

	sig := make(chan os.Signal, 2)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

}

func pulsePad(pad Pad) {
	timeout := time.After(5 * time.Second)
	pad.lightMode = Pulsing
	sendNote(On, pad)
	for {
		select {
		case <-timeout:
			sendNote(Off, pad)
			return
		default:
			time.Sleep(500 * time.Millisecond)
			continue
		}

	}

}

func clearPad() {
	for r := range uint8(9) {
		for c := range uint8(9) {
			sendNote(On, NewPad(PadPos{r + 1, c + 1}))
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func sendNote(on bool, pad Pad) {
	pads[pad.getKey()] = pad
	if on {
		if pad.pos.col > 8 || pad.pos.row > 8 {
			Send(midi.ControlChange(pad.lightMode, pad.getKey(), pad.color))
		} else {
			Send(midi.NoteOn(pad.lightMode, pad.getKey(), pad.color))
		}
	} else {
		if pad.pos.col > 8 || pad.pos.row > 8 {
			Send(midi.ControlChange(pad.lightMode, pad.getKey(), 0))
		} else {
			Send(midi.NoteOff(pad.lightMode, pad.getKey()))

		}
	}
}

func changeColor(pad Pad) {
	var curPad = pads[pad.getKey()]

	fmt.Printf("Current Pad: %v\n", curPad)
	if curPad.color < 128 {
		curPad.color = curPad.color + 4
		fmt.Printf("Color: %d\n", curPad.color)
	} else {
		curPad.color = 0
	}
	sendNote(On, curPad)
}

func midiNoteReceived(msg midi.Message, ts int32) {
	var channel, key, velocity, controller, value uint8

	switch {
	case msg.GetNoteOn(&channel, &key, &velocity):
		if velocity > 0 {
			fmt.Printf("%d %d %d\n", key, channel, velocity)
			go changeColor(NewPad(PadPosFromKey(key)))

		}
	case msg.GetControlChange(&channel, &controller, &value):
		if value > 0 {
			fmt.Printf("Controller: %d %d %d\n", channel, controller, value)
			go changeColor(NewPad(PadPosFromKey(controller)))
		}
	}

}

const (
	ColorOff         uint8 = 0
	ColorWhite       uint8 = 3
	ColorRed         uint8 = 5
	ColorRedDim      uint8 = 6
	ColorRedLight    uint8 = 7
	ColorOrange      uint8 = 9
	ColorOrangeDim   uint8 = 10
	ColorYellow      uint8 = 13
	ColorYellowLight uint8 = 14
	ColorLime        uint8 = 17
	ColorGreen       uint8 = 21
	ColorGreenDim    uint8 = 22
	ColorGreenLight  uint8 = 23
	ColorMint        uint8 = 29
	ColorCyan        uint8 = 37
	ColorCyanLight   uint8 = 38
	ColorSky         uint8 = 41
	ColorBlue        uint8 = 45
	ColorBlueDim     uint8 = 46
	ColorBlueLight   uint8 = 47
	ColorPurple      uint8 = 49
	ColorPurpleLight uint8 = 51
	ColorMagenta     uint8 = 53
	ColorPink        uint8 = 57
	ColorPinkLight   uint8 = 58
	ColorHotPink     uint8 = 56
)
