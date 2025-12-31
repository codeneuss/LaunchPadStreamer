# LaunchPadStreamer

A Go application for controlling the Launchpad Mini MK3 MIDI controller. This program demonstrates LED control and MIDI input handling with interactive pad responses.

## Features

- Connect to Launchpad Mini MK3 via MIDI
- Control LED colors and lighting modes (Permanent, Blinking, Pulsing)
- Interactive pad response - pads pulse when pressed
- Startup LED animation

## Requirements

- Go 1.16 or higher
- Launchpad Mini MK3 MIDI controller
- MIDI drivers installed on your system

## Installation

```bash
go get
go build
```

## Usage

```bash
./LaunchPadStreamer
```

Press `Ctrl+C` to exit the application.

## License

MIT License - see [LICENSE](LICENSE) file for details.
