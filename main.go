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

type Pad struct {
	key       uint8
	color     uint8
	lightMode uint8
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

	pulsePad(Pad{key: 11, color: 53})

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
	var i uint8
	for i = 0; i < 128; i++ {
		if i > 0 {
			sendNote(Off, Pad{key: i - 1})
		}
		sendNote(On, Pad{key: i, color: 53})
		time.Sleep(10 * time.Millisecond)
	}
}

func sendNote(on bool, pad Pad) {
	if on {
		Send(midi.NoteOn(pad.lightMode, pad.key, pad.color))
	} else {
		Send(midi.NoteOff(pad.lightMode, pad.key))
	}
}

var activeKeys = make(map[uint8]bool)

func midiNoteReceived(msg midi.Message, ts int32) {
	var channel, key, velocity uint8

	switch {
	case msg.GetNoteOn(&channel, &key, &velocity):
		if velocity > 0 && !activeKeys[key] {
			fmt.Printf("%d %d %d\n", key, channel, velocity)
			activeKeys[key] = true
			go pulsePad(Pad{key: key, color: 53})

		}
	case msg.GetNoteOff(&channel, &key, &velocity):
		delete(activeKeys, key)
	}
}
