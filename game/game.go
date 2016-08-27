package game

import (
	"bytes"
	"strconv"
	"strings"

	"github.com/gonutz/ld36/log"
	"github.com/gonutz/tiled"
)

type Game interface {
	Frame([]InputEvent)
}

type Resources interface {
	LoadImage(id string) Image
	LoadFile(id string) []byte
}

type DrawOptions struct {
	FlipX             bool
	Transparency      float32
	CenterRotationDeg int
}

type Image interface {
	DrawAt(x, y int)
	DrawAtEx(x, y int, options DrawOptions)
	DrawRectAt(x, y int, source Rectangle)
	Size() (width, height int)
}

type Rectangle struct {
	X, Y, W, H int
}

func New(resources Resources) Game {
	g := &game{
		resources:     resources,
		gateGlowDelta: 0.02,
	}
	g.init()
	return g
}

type game struct {
	resources Resources

	caveman     Image
	cavemanPush Image
	rock        Image
	gateGlowA   Image
	gateGlowB   Image
	tiles       Image

	gateGlowRatio float32
	gateGlowDelta float32

	cavemanX     int
	cavemanFlipX bool

	rockX        int
	rockRotation int

	leftDown  bool
	rightDown bool
	upDown    bool

	mapW, mapH   int
	tileMap      []tile
	tileW, tileH int
}

func (g *game) init() {
	g.caveman = g.resources.LoadImage("caveman_stand_left")
	g.cavemanPush = g.resources.LoadImage("caveman_push_left")
	g.rock = g.resources.LoadImage("rock")
	g.gateGlowA = g.resources.LoadImage("gate_a")
	g.gateGlowB = g.resources.LoadImage("gate_b")
	g.tiles = g.resources.LoadImage("tiles")

	level, err := tiled.Read(bytes.NewReader(g.resources.LoadFile("level_0.tmx")))
	if err != nil {
		log.Fatal("unable to decode level_0.tmx: ", err)
	}

	log.Println(level)

	g.mapW, g.mapH = level.Width, level.Height
	g.tileW, g.tileH = level.TileWidth, level.TileHeight
	tileSheetW, _ := g.tiles.Size()
	tileCountX := tileSheetW / g.tileW
	g.tileMap = make([]tile, g.mapW*g.mapH)
	for i := range level.Layers {
		if level.Layers[i].Name == "0" {
			text := strings.Trim(level.Layers[i].Data.Text, "\n")
			lines := strings.Split(text, "\n")
			for i := range lines {
				y := len(lines) - 1 - i
				line := strings.TrimSuffix(lines[i], ",")
				cols := strings.Split(line, ",")
				for x := range cols {
					id, err := strconv.Atoi(cols[x])
					if err != nil {
						log.Fatalf("tile ID is not an integer: '%v' at %v,%v", cols[x], x, y)
					}
					if id != 0 {
						id--
						tileX, tileY := id%tileCountX, id/tileCountX
						g.tileMap[x+y*g.mapW].Rectangle = Rectangle{
							tileX * g.tileW,
							tileY * g.tileH,
							g.tileW,
							g.tileH,
						}
					}
				}
			}
		}
	}
}

func (g *game) Frame(events []InputEvent) {
	// handle events
	for _, e := range events {
		switch e.Key {
		case KeyLeft:
			g.leftDown = e.Down
		case KeyRight:
			g.rightDown = e.Down
		case KeyUp:
			g.upDown = e.Down
		}
	}

	const speed = 7
	if g.leftDown && !g.rightDown {
		g.cavemanX -= speed
		g.cavemanFlipX = false
	}
	if g.rightDown && !g.leftDown {
		g.cavemanX += speed
		g.cavemanFlipX = true
	}

	g.gateGlowRatio += g.gateGlowDelta
	if g.gateGlowRatio < 0 {
		g.gateGlowRatio = 0
		g.gateGlowDelta = -g.gateGlowDelta
	}
	if g.gateGlowRatio > 1 {
		g.gateGlowRatio = 1
		g.gateGlowDelta = -g.gateGlowDelta
	}

	g.rockRotation += 2
	g.rockX += 3

	// render
	var empty Rectangle
	for y := 0; y < g.mapH; y++ {
		for x := 0; x < g.mapW; x++ {
			tile := g.tileMap[x+y*g.mapW].Rectangle
			if tile != empty {
				g.tiles.DrawRectAt(x*g.tileW, y*g.tileH, tile)
			}
		}
	}

	caveman := g.caveman
	if g.upDown {
		caveman = g.cavemanPush
	}
	caveman.DrawAtEx(g.cavemanX, 50, flipX(g.cavemanFlipX))

	g.rock.DrawAtEx(g.rockX, 50, centerRotation(g.rockRotation))

	g.gateGlowA.DrawAtEx(0, 50, flipX(true))
	g.gateGlowB.DrawAtEx(0, 50, flipX(true).opacity(g.gateGlowRatio))
}

func flipX(value bool) DrawOptions {
	return DrawOptions{FlipX: value}
}

func (o DrawOptions) flipX(value bool) DrawOptions {
	o.FlipX = value
	return o
}

func opacity(value float32) DrawOptions {
	return DrawOptions{Transparency: 1 - value}
}

func (o DrawOptions) opacity(value float32) DrawOptions {
	o.Transparency = 1 - value
	return o
}

func centerRotation(value int) DrawOptions {
	return DrawOptions{CenterRotationDeg: value}
}

func (o DrawOptions) centerRotation(value int) DrawOptions {
	o.CenterRotationDeg = value
	return o
}

type tile struct {
	Rectangle
}
