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
	cavemanFall Image
	rock        Image
	gateGlowA   Image
	gateGlowB   Image
	tiles       Image

	gateGlowRatio float32
	gateGlowDelta float32

	cavemanX, cavemanY int
	cavemanFlipX       bool

	rockX        int
	rockRotation int

	leftDown  bool
	rightDown bool
	upDown    bool

	tileMap tileMap
}

func (g *game) init() {
	g.caveman = g.resources.LoadImage("caveman_stand_left")
	g.cavemanPush = g.resources.LoadImage("caveman_push_left")
	g.cavemanFall = g.resources.LoadImage("caveman_fall_left")
	g.rock = g.resources.LoadImage("rock")
	g.gateGlowA = g.resources.LoadImage("gate_a")
	g.gateGlowB = g.resources.LoadImage("gate_b")
	g.tiles = g.resources.LoadImage("tiles")

	level, err := tiled.Read(bytes.NewReader(g.resources.LoadFile("level_0.tmx")))
	if err != nil {
		log.Fatal("unable to decode level_0.tmx: ", err)
	}

	g.tileMap.setSize(level.Width, level.Height)
	g.tileMap.tileW, g.tileMap.tileH = level.TileWidth, level.TileHeight
	tileSheetW, _ := g.tiles.Size()
	tileCountX := tileSheetW / g.tileMap.tileW
	for i := range level.Layers {
		if level.Layers[i].Name == "objects" {
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
					if id == 1 {
						g.cavemanX = g.tileMap.toWorldX(x)
						g.cavemanY = g.tileMap.toWorldX(y)
					}
				}
			}
		}

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
						tile := g.tileMap.tileAt(x, y)
						tile.imageSource = Rectangle{
							tileX * g.tileMap.tileW,
							tileY * g.tileMap.tileH,
							g.tileMap.tileW,
							g.tileMap.tileH,
						}
						tile.isSolid = id >= 1
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
	var cavemanDx int
	if g.leftDown && !g.rightDown {
		cavemanDx = -speed
		g.cavemanFlipX = false
	}
	if g.rightDown && !g.leftDown {
		cavemanDx = speed
		g.cavemanFlipX = true
	}
	cavemanW, cavemanH := g.caveman.Size()
	cavemanRect := Rectangle{g.cavemanX, g.cavemanY, cavemanW, cavemanH}
	newCavemanRect, _ := g.tileMap.moveInX(cavemanRect, cavemanDx)
	g.cavemanX = newCavemanRect.X

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
	for y := 0; y < g.tileMap.height; y++ {
		for x := 0; x < g.tileMap.width; x++ {
			tile := g.tileMap.tileAt(x, y).imageSource
			if tile != empty {
				x, y := g.tileMap.toWorldXY(x, y)
				g.tiles.DrawRectAt(x, y, tile)
			}
		}
	}

	caveman := g.caveman
	if g.upDown {
		caveman = g.cavemanFall
	}
	caveman.DrawAtEx(g.cavemanX, g.cavemanY, flipX(g.cavemanFlipX))

	g.rock.DrawAtEx(g.rockX, g.cavemanY, centerRotation(g.rockRotation))
	g.rockX %= 2000

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
	imageSource Rectangle
	isSolid     bool
}

type tileMap struct {
	width, height int
	tileW, tileH  int
	tiles         []tile
}

func (m *tileMap) setSize(w, h int) {
	m.width, m.height = w, h
	m.tiles = make([]tile, w*h)
}

func (m *tileMap) toTileX(worldX int) int {
	return worldX / m.tileW
}

func (m *tileMap) toTileY(worldY int) int {
	return worldY / m.tileH
}

func (m *tileMap) toWorldX(tileX int) int {
	return tileX * m.tileW
}

func (m *tileMap) toWorldY(tileY int) int {
	return tileY * m.tileH
}

func (m *tileMap) toWorldXY(tileX, tileY int) (worldX, worldY int) {
	return tileX * m.tileW, tileY * m.tileH
}

func (m *tileMap) tileAt(tileX, tileY int) *tile {
	return &m.tiles[tileX+tileY*m.width]
}

func (m *tileMap) moveInX(start Rectangle, dx int) (end Rectangle, hitWall bool) {
	if dx < 0 {
		// going left, create a rect from current right to new left position and
		// check that against object collisions
		r := start
		r.X += dx
		r.W -= dx
		newX := r.X
		for tileY := m.toTileY(r.Y); tileY <= m.toTileY(r.Y+r.H-1); tileY++ {
			for tileX := m.toTileX(r.X); tileX <= m.toTileX(r.X+r.W-1); tileX++ {
				if m.tileAt(tileX, tileY).isSolid {
					right := m.toWorldX(tileX + 1)
					// there could be multiple collisions, use the one which
					// produces the lowest height (highest x value)
					if right > newX {
						newX = right
					}
				}
			}
		}
		if newX != r.X {
			hitWall = true
		}
		start.X = newX
	} else if dx > 0 {
		// going right, create a rect from the current left to new right and
		// check that against object collisions
		r := start
		r.W += dx
		newRight := r.X + r.W - 1
		for tileY := m.toTileY(r.Y); tileY <= m.toTileY(r.Y+r.H-1); tileY++ {
			for tileX := m.toTileX(r.X); tileX <= m.toTileX(r.X+r.W-1); tileX++ {
				if m.tileAt(tileX, tileY).isSolid {
					left := m.toWorldX(tileX) - 1
					// there could be multiple object collisions, use the one which
					// produces the highest height (lowest x value)
					if left < newRight {
						newRight = left
					}
				}
			}
		}
		if newRight != r.X+r.W-1 {
			hitWall = true
		}
		start.X = newRight - start.W + 1
	}

	end = start
	return
}
