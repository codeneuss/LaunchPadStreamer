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

type Game interface {
	Start()
	Stop()
	HandlePadPress(pad Pad)
	Name() string
}

type GameManager struct {
	currentGame Game
	games       []Game
	currentIdx  int
}

func NewGameManager() *GameManager {
	return &GameManager{
		games:      make([]Game, 0),
		currentIdx: 0,
	}
}

func (gm *GameManager) AddGame(game Game) {
	gm.games = append(gm.games, game)
}

func (gm *GameManager) StartCurrentGame() {
	if len(gm.games) > 0 {
		gm.currentGame = gm.games[gm.currentIdx]
		fmt.Printf("Starting game: %s\n", gm.currentGame.Name())
		gm.currentGame.Start()
	}
}

func (gm *GameManager) SwitchToNextGame() {
	if len(gm.games) == 0 {
		return
	}

	if gm.currentGame != nil {
		gm.currentGame.Stop()
	}

	gm.currentIdx = (gm.currentIdx + 1) % len(gm.games)
	gm.currentGame = gm.games[gm.currentIdx]

	fmt.Printf("Switching to game: %s\n", gm.currentGame.Name())
	clearPad()
	gm.currentGame.Start()
}

func (gm *GameManager) HandlePadPress(pad Pad) {
	if gm.currentGame != nil {
		gm.currentGame.HandlePadPress(pad)
	}
}

var gameManager *GameManager

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

	// Initialize game manager FIRST before any MIDI events can arrive
	gameManager = NewGameManager()
	gameManager.AddGame(&ColorChangerGame{})
	gameManager.AddGame(NewPixelPaintGame())
	gameManager.AddGame(NewTicTacToeGame())

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

	sysex := []byte{0xF0, 0x00, 0x20, 0x29, 0x02, 0x0D, 0x0E, 0x01, 0xF7}
	Send(sysex)

	clearPad()

	fmt.Println("===========================================")
	fmt.Println("LaunchPad Game Streamer")
	fmt.Println("Press top-right button to switch games!")
	fmt.Println("===========================================")

	gameManager.StartCurrentGame()

	// Start listening to MIDI AFTER everything is initialized
	stop, _ := midi.ListenTo(in, midiNoteReceived)
	defer stop()

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
			pad := NewPad(PadPosFromKey(key))
			gameManager.HandlePadPress(pad)
		}
	case msg.GetControlChange(&channel, &controller, &value):
		if value > 0 {
			fmt.Printf("Controller: %d %d %d\n", channel, controller, value)
			pad := NewPad(PadPosFromKey(controller))

			// Top-right button (row 1, col 9) switches games
			if pad.pos.row == 1 && pad.pos.col == 9 {
				gameManager.SwitchToNextGame()
			} else if pad.pos.col == 9 {
				// Ignore all other buttons in column 9 (right side control buttons)
				fmt.Printf("Ignoring control button at row %d\n", pad.pos.row)
			} else {
				gameManager.HandlePadPress(pad)
			}
		}
	}
}

// ColorChanger Game - cycles through colors when pads are pressed
type ColorChangerGame struct{}

func (g *ColorChangerGame) Name() string {
	return "Color Changer"
}

func (g *ColorChangerGame) Start() {
	clearPad()
	fmt.Println("ColorChanger game started - Press pads to cycle colors!")
}

func (g *ColorChangerGame) Stop() {
	fmt.Println("ColorChanger game stopped")
}

func (g *ColorChangerGame) HandlePadPress(pad Pad) {
	changeColor(pad)
}

// PixelPaint Game - draw patterns with different colors
type PixelPaintGame struct {
	currentColor uint8
	colorPalette []uint8
	colorIndex   int
}

func NewPixelPaintGame() *PixelPaintGame {
	return &PixelPaintGame{
		currentColor: ColorRed,
		colorPalette: []uint8{
			ColorRed,
			ColorOrange,
			ColorYellow,
			ColorGreen,
			ColorCyan,
			ColorBlue,
			ColorPurple,
			ColorMagenta,
			ColorPink,
			ColorWhite,
		},
		colorIndex: 0,
	}
}

func (g *PixelPaintGame) Name() string {
	return "Pixel Paint"
}

func (g *PixelPaintGame) Start() {
	fmt.Println("PixelPaint game started - Left column selects color, draw on the grid!")

	// Display color palette on the left column
	for i, color := range g.colorPalette {
		if i < 8 {
			pad := NewPad(PadPos{uint8(i + 1), 1})
			pad.color = color
			sendNote(On, pad)
		}
	}

	// Highlight current color
	g.highlightCurrentColor()
}

func (g *PixelPaintGame) Stop() {
	fmt.Println("PixelPaint game stopped")
}

func (g *PixelPaintGame) highlightCurrentColor() {
	// Add pulsing effect to current color
	for i := range g.colorPalette {
		if i < 8 {
			pad := NewPad(PadPos{uint8(i + 1), 1})
			pad.color = g.colorPalette[i]
			if i == g.colorIndex {
				pad.lightMode = Pulsing
			} else {
				pad.lightMode = Permanent
			}
			sendNote(On, pad)
		}
	}
}

func (g *PixelPaintGame) HandlePadPress(pad Pad) {
	// If pressing left column, change color
	if pad.pos.col == 1 && pad.pos.row >= 1 && pad.pos.row <= 8 {
		g.colorIndex = int(pad.pos.row - 1)
		if g.colorIndex < len(g.colorPalette) {
			g.currentColor = g.colorPalette[g.colorIndex]
			g.highlightCurrentColor()
			fmt.Printf("Selected color: %d\n", g.currentColor)
		}
	} else {
		// Paint on the grid
		pad.color = g.currentColor
		sendNote(On, pad)
	}
}

// TicTacToe Game - classic 3x3 grid game
type TicTacToeGame struct {
	board         [3][3]int // 0 = empty, 1 = player 1 (X), 2 = player 2 (O)
	currentPlayer int
	gameOver      bool
	winner        int
	moveHistory   []struct {
		row    int
		col    int
		player int
	}
	maxMoves int
}

func NewTicTacToeGame() *TicTacToeGame {
	return &TicTacToeGame{
		currentPlayer: 1,
		gameOver:      false,
		winner:        0,
		moveHistory: make([]struct {
			row    int
			col    int
			player int
		}, 0),
		maxMoves: 2, // Each player gets max 2 moves
	}
}

func (g *TicTacToeGame) Name() string {
	return "Tic Tac Toe"
}

func (g *TicTacToeGame) Start() {
	fmt.Println("TicTacToe game starting...")

	// Reset game state
	g.board = [3][3]int{}
	g.currentPlayer = 1
	g.gameOver = false
	g.winner = 0
	g.moveHistory = make([]struct {
		row    int
		col    int
		player int
	}, 0)

	// Show scrolling text "TIC TAC TOE" in red
	g.showScrollingText()

	// Show grid after animation
	g.drawGrid()
}

func (g *TicTacToeGame) Stop() {
	fmt.Println("TicTacToe game stopped")
}

// Letter patterns for scrolling text (5x3 pixels each)
var letterPatterns = map[rune][][]bool{
	'T': {
		{true, true, true},
		{false, true, false},
		{false, true, false},
		{false, true, false},
		{false, true, false},
	},
	'I': {
		{true, true, true},
		{false, true, false},
		{false, true, false},
		{false, true, false},
		{true, true, true},
	},
	'C': {
		{false, true, true},
		{true, false, false},
		{true, false, false},
		{true, false, false},
		{false, true, true},
	},
	'A': {
		{false, true, false},
		{true, false, true},
		{true, true, true},
		{true, false, true},
		{true, false, true},
	},
	'O': {
		{false, true, false},
		{true, false, true},
		{true, false, true},
		{true, false, true},
		{false, true, false},
	},
	' ': {
		{false, false, false},
		{false, false, false},
		{false, false, false},
		{false, false, false},
		{false, false, false},
	},
}

func (g *TicTacToeGame) showScrollingText() {
	text := "TIC TAC TOE"

	// Create a wide canvas for scrolling
	totalWidth := 0
	for _, char := range text {
		if pattern, exists := letterPatterns[char]; exists {
			totalWidth += len(pattern[0]) + 1 // +1 for spacing
		}
	}

	// Build the full text pattern
	canvas := make([][]bool, 5)
	for i := range canvas {
		canvas[i] = make([]bool, totalWidth)
	}

	xPos := 0
	for _, char := range text {
		if pattern, exists := letterPatterns[char]; exists {
			for row := 0; row < 5 && row < len(pattern); row++ {
				for col := 0; col < len(pattern[row]); col++ {
					if xPos+col < totalWidth {
						canvas[row][xPos+col] = pattern[row][col]
					}
				}
			}
			xPos += len(pattern[0]) + 1
		}
	}

	// Scroll the text from right to left
	for offset := -8; offset < totalWidth; offset++ {
		// Clear display
		for row := 1; row <= 5; row++ {
			for col := 1; col <= 8; col++ {
				pad := NewPad(PadPos{uint8(row + 1), uint8(col)})
				pad.color = ColorOff
				sendNote(On, pad)
			}
		}

		// Draw visible portion
		for row := 0; row < 5; row++ {
			for col := 0; col < 8; col++ {
				srcCol := totalWidth - offset - 8 + col
				if srcCol >= 0 && srcCol < totalWidth && canvas[row][srcCol] {
					pad := NewPad(PadPos{uint8(row + 2), uint8(col + 1)})
					pad.color = ColorRed
					sendNote(On, pad)
				}
			}
		}

		time.Sleep(100 * time.Millisecond)
	}
}

func (g *TicTacToeGame) drawGrid() {
	// Clear the board first
	clearPad()

	// Draw 3x3 grid with yellow lines
	// Horizontal lines: row 3 from col 1-8, row 6 from col 1-8
	// Vertical lines: col 3 from row 1-8, col 6 from row 1-8
	// This creates 9 cells with spacing of 2 pads

	// First horizontal line: from 13 to 83 (row 1, col 3 to row 8, col 3)
	// This means col 3 from row 1 to 8
	for row := uint8(1); row <= 8; row++ {
		pad := NewPad(PadPos{row, 3})
		pad.color = ColorYellow
		sendNote(On, pad)
	}

	// Second horizontal line: from 16 to 86 (row 1, col 6 to row 8, col 6)
	// This means col 6 from row 1 to 8
	for row := uint8(1); row <= 8; row++ {
		pad := NewPad(PadPos{row, 6})
		pad.color = ColorYellow
		sendNote(On, pad)
	}

	// First vertical line: from 31 to 38 (row 3, col 1 to row 3, col 8)
	// This means row 3 from col 1 to 8
	for col := uint8(1); col <= 8; col++ {
		pad := NewPad(PadPos{3, col})
		pad.color = ColorYellow
		sendNote(On, pad)
	}

	// Second vertical line: from 61 to 68 (row 6, col 1 to row 6, col 8)
	// This means row 6 from col 1 to 8
	for col := uint8(1); col <= 8; col++ {
		pad := NewPad(PadPos{6, col})
		pad.color = ColorYellow
		sendNote(On, pad)
	}

	fmt.Println("Grid drawn! Player 1 (Red) starts")
}

func (g *TicTacToeGame) HandlePadPress(pad Pad) {
	if g.gameOver {
		// Reset game on any pad press after game over
		g.Start()
		return
	}

	// Convert pad position to grid position (0-2, 0-2)
	gridRow, gridCol := g.padToGridPosition(pad)

	if gridRow == -1 || gridCol == -1 {
		return // Not a valid grid position
	}

	// Check if cell is empty
	if g.board[gridRow][gridCol] != 0 {
		fmt.Println("Cell already occupied!")
		return
	}

	// If there are already 7 moves total, remove the oldest one
	if len(g.moveHistory) >= 7 {
		// Remove the very first move (oldest)
		oldestMove := g.moveHistory[0]
		g.board[oldestMove.row][oldestMove.col] = 0
		g.clearMark(oldestMove.row, oldestMove.col)
		g.moveHistory = g.moveHistory[1:]
		fmt.Printf("Removed oldest move (player %d) at (%d,%d)\n", oldestMove.player, oldestMove.row, oldestMove.col)
	}

	// Place mark
	g.board[gridRow][gridCol] = g.currentPlayer
	g.drawMark(gridRow, gridCol, g.currentPlayer)

	// Add to move history
	g.moveHistory = append(g.moveHistory, struct {
		row    int
		col    int
		player int
	}{gridRow, gridCol, g.currentPlayer})

	// Check for winner
	if g.checkWinner() {
		g.gameOver = true
		g.winner = g.currentPlayer
		fmt.Printf("Player %d wins!\n", g.currentPlayer)
		g.showWinner()
		return
	}

	// Switch player
	if g.currentPlayer == 1 {
		g.currentPlayer = 2
	} else {
		g.currentPlayer = 1
	}

	fmt.Printf("Player %d's turn\n", g.currentPlayer)
}

func (g *TicTacToeGame) padToGridPosition(pad Pad) (int, int) {
	// Map pad positions to 3x3 grid (0-2, 0-2)
	// Grid structure (inverted rows - row 8 is top, row 1 is bottom):
	// - Top row (0): rows 7-8
	// - Middle row (1): rows 4-5
	// - Bottom row (2): rows 1-2
	// - Left col (0): cols 1-2
	// - Middle col (1): cols 4-5
	// - Right col (2): cols 7-8

	gridRow := -1
	gridCol := -1

	// Determine row (inverted - 8 is at top)
	if pad.pos.row >= 7 && pad.pos.row <= 8 {
		gridRow = 0
	} else if pad.pos.row >= 4 && pad.pos.row <= 5 {
		gridRow = 1
	} else if pad.pos.row >= 1 && pad.pos.row <= 2 {
		gridRow = 2
	}

	// Determine column
	if pad.pos.col >= 1 && pad.pos.col <= 2 {
		gridCol = 0
	} else if pad.pos.col >= 4 && pad.pos.col <= 5 {
		gridCol = 1
	} else if pad.pos.col >= 7 && pad.pos.col <= 8 {
		gridCol = 2
	}

	return gridRow, gridCol
}

func (g *TicTacToeGame) drawMark(gridRow, gridCol, player int) {
	// Convert grid position to pad position
	// Grid cells are 2x2, so we light up all 4 pads in each cell
	var startRow, startCol uint8

	// Determine starting position for this grid cell (inverted rows)
	if gridRow == 0 {
		startRow = 7 // Top row
	} else if gridRow == 1 {
		startRow = 4 // Middle row
	} else {
		startRow = 1 // Bottom row
	}

	if gridCol == 0 {
		startCol = 1
	} else if gridCol == 1 {
		startCol = 4
	} else {
		startCol = 7
	}

	var color uint8
	if player == 1 {
		color = ColorRed
	} else {
		color = ColorBlue
	}

	// Light up all 4 pads in the 2x2 cell
	for r := startRow; r < startRow+2; r++ {
		for c := startCol; c < startCol+2; c++ {
			pad := NewPad(PadPos{r, c})
			pad.color = color
			pad.lightMode = Pulsing
			sendNote(On, pad)
			time.Sleep(50 * time.Millisecond)
		}
	}

	// Redraw the grid lines to ensure they stay visible
	//g.redrawGridLines()
}

func (g *TicTacToeGame) clearMark(gridRow, gridCol int) {
	// Convert grid position to pad position
	var startRow, startCol uint8

	// Determine starting position for this grid cell (inverted rows)
	if gridRow == 0 {
		startRow = 7 // Top row
	} else if gridRow == 1 {
		startRow = 4 // Middle row
	} else {
		startRow = 1 // Bottom row
	}

	if gridCol == 0 {
		startCol = 1
	} else if gridCol == 1 {
		startCol = 4
	} else {
		startCol = 7
	}

	// Turn off all 4 pads in the 2x2 cell
	for r := startRow; r < startRow+2; r++ {
		for c := startCol; c < startCol+2; c++ {
			pad := NewPad(PadPos{r, c})
			pad.color = ColorOff
			sendNote(Off, pad)
		}
	}

	// Redraw the grid lines to ensure they stay visible
	g.redrawGridLines()
}

func (g *TicTacToeGame) redrawGridLines() {
	// Redraw vertical lines (col 3 and 6)
	for row := uint8(1); row <= 8; row++ {
		pad := NewPad(PadPos{row, 3})
		pad.color = ColorYellow
		sendNote(On, pad)

		pad = NewPad(PadPos{row, 6})
		pad.color = ColorYellow
		sendNote(On, pad)
	}

	// Redraw horizontal lines (row 3 and 6)
	for col := uint8(1); col <= 8; col++ {
		pad := NewPad(PadPos{3, col})
		pad.color = ColorYellow
		sendNote(On, pad)

		pad = NewPad(PadPos{6, col})
		pad.color = ColorYellow
		sendNote(On, pad)
	}
}

func (g *TicTacToeGame) checkWinner() bool {
	// Check rows
	for row := 0; row < 3; row++ {
		if g.board[row][0] != 0 && g.board[row][0] == g.board[row][1] && g.board[row][1] == g.board[row][2] {
			return true
		}
	}

	// Check columns
	for col := 0; col < 3; col++ {
		if g.board[0][col] != 0 && g.board[0][col] == g.board[1][col] && g.board[1][col] == g.board[2][col] {
			return true
		}
	}

	// Check diagonals
	if g.board[0][0] != 0 && g.board[0][0] == g.board[1][1] && g.board[1][1] == g.board[2][2] {
		return true
	}

	if g.board[0][2] != 0 && g.board[0][2] == g.board[1][1] && g.board[1][1] == g.board[2][0] {
		return true
	}

	return false
}

func (g *TicTacToeGame) isBoardFull() bool {
	for row := 0; row < 3; row++ {
		for col := 0; col < 3; col++ {
			if g.board[row][col] == 0 {
				return false
			}
		}
	}
	return true
}

func (g *TicTacToeGame) showWinner() {
	// Flash the winning player's color
	var color uint8
	if g.winner == 1 {
		color = ColorRed
	} else {
		color = ColorBlue
	}

	for i := 0; i < 5; i++ {
		for row := 1; row <= 8; row++ {
			for col := 1; col <= 8; col++ {
				pad := NewPad(PadPos{uint8(row), uint8(col)})
				if i%2 == 0 {
					pad.color = color
				} else {
					pad.color = ColorOff
				}
				sendNote(On, pad)
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
}

func (g *TicTacToeGame) showDraw() {
	// Alternate colors for draw
	for i := 0; i < 8; i++ {
		for row := 1; row <= 8; row++ {
			for col := 1; col <= 8; col++ {
				pad := NewPad(PadPos{uint8(row), uint8(col)})
				if (row+col+i)%2 == 0 {
					pad.color = ColorYellow
				} else {
					pad.color = ColorOff
				}
				sendNote(On, pad)
			}
		}
		time.Sleep(300 * time.Millisecond)
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
