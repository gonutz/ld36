package game

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/gonutz/ld36/log"
	"github.com/gonutz/tiled"
)

const (
	objPlayerLeft = iota
	objPlayerRight
	objGateLeft
	objGateRight
	objRock
)

type Game interface {
	Frame([]InputEvent)
	SetScreenSize(width, height int)
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

type camera struct {
	offsetX, offsetY int
}

type cameraImage struct {
	Image
	camera *camera
}

func (img cameraImage) DrawAt(x, y int) {
	img.Image.DrawAt(x+img.camera.offsetX, y+img.camera.offsetY)
}

func (img cameraImage) DrawAtEx(x, y int, options DrawOptions) {
	img.Image.DrawAtEx(x+img.camera.offsetX, y+img.camera.offsetY, options)
}

func (img cameraImage) DrawRectAt(x, y int, source Rectangle) {
	img.Image.DrawRectAt(x+img.camera.offsetX, y+img.camera.offsetY, source)
}

type game struct {
	resources Resources

	screenW, screenH int
	camera           camera

	cavemanStand Image
	cavemanPush  Image
	cavemanFall  Image
	rock         Image
	gateGlowA    Image
	gateGlowB    Image
	tiles        Image

	gateGlowRatio float32
	gateGlowDelta float32

	cavemanX, cavemanY int
	cavemanFacesRight  bool
	cavemanHitBox      Rectangle

	exitX, exitY   int
	exitFacesRight bool

	rocks []rock

	leftDown  bool
	rightDown bool
	upDown    bool

	tileMap tileMap
}

type rock struct {
	x, y     int
	rotation int
}

func (g *game) loadImage(id string) Image {
	return cameraImage{
		Image:  g.resources.LoadImage(id),
		camera: &g.camera,
	}
}

func (g *game) init() {
	var info Info
	data := g.resources.LoadFile("info.json")
	err := json.NewDecoder(bytes.NewReader(data)).Decode(&info)
	if err != nil {
		log.Fatal("unable to decode game info json file: ", err)
	}
	g.cavemanHitBox = info.CavemanHitBox

	g.cavemanStand = g.loadImage("caveman_stand_left")
	g.cavemanPush = g.loadImage("caveman_push_left")
	g.cavemanFall = g.loadImage("caveman_fall_left")
	g.rock = g.loadImage("rock")
	g.gateGlowA = g.loadImage("gate_a")
	g.gateGlowB = g.loadImage("gate_b")
	g.tiles = g.loadImage("tiles")

	level, err := tiled.Read(bytes.NewReader(g.resources.LoadFile("level_0.tmx")))
	if err != nil {
		log.Fatal("unable to decode level_0.tmx: ", err)
	}

	g.tileMap.setSize(level.Width, level.Height)
	g.tileMap.tileW, g.tileMap.tileH = level.TileWidth, level.TileHeight
	tileSheetW, tileSheetH := g.tiles.Size()
	tileCountX := tileSheetW / g.tileMap.tileW
	tileCountY := tileSheetH / g.tileMap.tileH
	for i := range level.Layers {
		if level.Layers[i].Name == "objects" {
			objIndexOffset := 1 + tileCountX*tileCountY

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
					if id == 0 {
						continue
					}

					id -= objIndexOffset
					worldX, worldY := g.tileMap.toWorldXY(x, y)

					if id == objPlayerLeft {
						g.cavemanFacesRight = false
						g.cavemanX, g.cavemanY = worldX, worldY
					}
					if id == objPlayerRight {
						g.cavemanFacesRight = true
						g.cavemanX, g.cavemanY = worldX, worldY
					}
					if id == objGateLeft {
						g.exitFacesRight = false
						g.exitX, g.exitY = worldX, worldY
					}
					if id == objGateRight {
						g.exitFacesRight = true
						g.exitX, g.exitY = worldX, worldY
					}
					if id == objRock {
						r := rock{
							x: worldX,
							y: worldY,
						}
						g.rocks = append(g.rocks, r)
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

func (g *game) SetScreenSize(width, height int) {
	g.screenW, g.screenH = width, height
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
		g.cavemanFacesRight = false
	}
	if g.rightDown && !g.leftDown {
		cavemanDx = speed
		g.cavemanFacesRight = true
	}
	cavemanW, cavemanH := g.cavemanStand.Size()
	cavemanRect := Rectangle{
		g.cavemanX + g.cavemanHitBox.X,
		g.cavemanY + g.cavemanHitBox.Y,
		g.cavemanHitBox.W,
		g.cavemanHitBox.H,
	}
	cavemanRect, _ = g.tileMap.moveInX(cavemanRect, cavemanDx)
	cavemanRect, _ = g.tileMap.moveInY(cavemanRect, -5)
	g.cavemanX = cavemanRect.X - g.cavemanHitBox.X
	g.cavemanY = cavemanRect.Y - g.cavemanHitBox.Y
	g.camera.offsetX = -g.cavemanX + (g.screenW-cavemanW)/2
	g.camera.offsetY = -g.cavemanY + (g.screenH-cavemanH)/2

	g.gateGlowRatio += g.gateGlowDelta
	if g.gateGlowRatio < 0 {
		g.gateGlowRatio = 0
		g.gateGlowDelta = -g.gateGlowDelta
	}
	if g.gateGlowRatio > 1 {
		g.gateGlowRatio = 1
		g.gateGlowDelta = -g.gateGlowDelta
	}

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

	caveman := g.cavemanStand
	if g.upDown {
		caveman = g.cavemanFall
	}
	caveman.DrawAtEx(g.cavemanX, g.cavemanY, flipX(g.cavemanFacesRight))

	for i := range g.rocks {
		g.rock.DrawAtEx(
			g.rocks[i].x,
			g.rocks[i].y,
			centerRotation(g.rocks[i].rotation),
		)
	}

	g.gateGlowA.DrawAtEx(g.exitX, g.exitY, flipX(g.exitFacesRight))
	g.gateGlowB.DrawAtEx(g.exitX, g.exitY, flipX(g.exitFacesRight).opacity(g.gateGlowRatio))
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
	if tileX < 0 || tileY < 0 || tileX >= m.width || tileY >= m.height {
		return &tile{}
	}
	return &m.tiles[tileX+tileY*m.width]
}

func (m *tileMap) moveInX(start Rectangle, dx int) (end Rectangle, hitWall bool) {
	if dx < 0 {
		r := start
		r.X += dx
		r.W -= dx
		newX := r.X
		for tileY := m.toTileY(r.Y); tileY <= m.toTileY(r.Y+r.H-1); tileY++ {
			for tileX := m.toTileX(r.X); tileX <= m.toTileX(r.X+r.W-1); tileX++ {
				if m.tileAt(tileX, tileY).isSolid {
					right := m.toWorldX(tileX + 1)
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
		r := start
		r.W += dx
		newRight := r.X + r.W - 1
		for tileY := m.toTileY(r.Y); tileY <= m.toTileY(r.Y+r.H-1); tileY++ {
			for tileX := m.toTileX(r.X); tileX <= m.toTileX(r.X+r.W-1); tileX++ {
				if m.tileAt(tileX, tileY).isSolid {
					left := m.toWorldX(tileX) - 1
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

func (m *tileMap) moveInY(start Rectangle, dy int) (end Rectangle, hitWall bool) {
	if dy < 0 {
		r := start
		r.Y += dy
		r.H -= dy
		newY := r.Y
		for tileY := m.toTileY(r.Y); tileY <= m.toTileY(r.Y+r.H-1); tileY++ {
			for tileX := m.toTileX(r.X); tileX <= m.toTileX(r.X+r.W-1); tileX++ {
				if m.tileAt(tileX, tileY).isSolid {
					bottom := m.toWorldY(tileY + 1)
					if bottom > newY {
						newY = bottom
					}
				}
			}
		}
		if newY != r.Y {
			hitWall = true
		}
		start.Y = newY
	} else if dy > 0 {
		r := start
		r.H += dy
		newBottom := r.Y + r.H - 1
		for tileY := m.toTileY(r.Y); tileY <= m.toTileY(r.Y+r.H-1); tileY++ {
			for tileX := m.toTileX(r.X); tileX <= m.toTileX(r.X+r.W-1); tileX++ {
				if m.tileAt(tileX, tileY).isSolid {
					top := m.toWorldY(tileY) - 1
					if top < newBottom {
						newBottom = top
					}
				}
			}
		}
		if newBottom != r.Y+r.H-1 {
			hitWall = true
		}
		start.Y = newBottom - start.H + 1
	}

	end = start
	return
}
