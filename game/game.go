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
	LoadSound(id string) Sound
	LoadFile(id string) []byte
}

type DrawOptions struct {
	FlipX             bool
	Transparency      float32
	CenterRotationDeg float32
}

type Image interface {
	DrawAt(x, y int)
	DrawAtEx(x, y int, options DrawOptions)
	DrawRectAt(x, y int, source Rectangle)
	Size() (width, height int)
}

type Sound interface {
	Play()
	PlayLooping()
}

type Rectangle struct {
	X, Y, W, H int
}

func (r Rectangle) overlaps(s Rectangle) bool {
	return r.X+r.W > s.X && r.Y+r.H > s.Y && s.X+s.W > r.X && s.Y+s.H > r.Y
}

func New(resources Resources) Game {
	f := &gameFrame{
		resources: resources,
	}
	f.init()
	return f
}

type gameFrame struct {
	game             *game
	resources        Resources
	screenW, screenH int
	winImage         Image
	info             Info
	levelIndex       int
	won              bool
}

func (f *gameFrame) init() {
	data := f.resources.LoadFile("info.json")
	err := json.NewDecoder(bytes.NewReader(data)).Decode(&f.info)
	if err != nil {
		log.Fatal("unable to decode game info json file: ", err)
	}

	f.winImage = f.resources.LoadImage("win_screen")

	// start background music
	f.resources.LoadSound("back_music").PlayLooping()

	f.newGame()
}

func (f *gameFrame) newGame() {
	f.game = &game{
		resources:     f.resources,
		gateGlowDelta: 0.02,
	}
	f.game.init(f.info, f.levelIndex)
}

func (f *gameFrame) Frame(events []InputEvent) {
	if f.won {
		for _, e := range events {
			if e.Key == KeyRestart && !e.Down {
				f.won = false
				f.levelIndex = 0
				f.newGame()
				events = nil
				break
			}
		}

		w, h := f.winImage.Size()
		x := (f.screenW - w) / 2
		y := (f.screenH - h) / 2
		f.winImage.DrawAt(x, y)
		return
	}

	for _, e := range events {
		if e.Key == KeyRestart && !e.Down {
			f.newGame()
			events = nil
			break
		}
	}

	f.game.Frame(events)

	if f.game.levelFinished() {
		f.levelIndex++
		if f.levelIndex >= f.info.LevelCount {
			f.won = true
			return
		}
		f.newGame()
	}
}

func (f *gameFrame) SetScreenSize(width, height int) {
	f.screenW, f.screenH = width, height
	f.game.SetScreenSize(width, height)
}

type camera struct {
	offsetX, offsetY int
	screenW, screenH int
	worldW, worldH   int
}

func (c *camera) setWorldSize(w, h int) {
	c.worldW, c.worldH = w, h
}

func (c *camera) setScreenSize(w, h int) {
	c.screenW, c.screenH = w, h
}

func (c *camera) centerAround(x, y int) {
	c.offsetX, c.offsetY = c.screenW/2-x, c.screenH/2-y
	// clamp X
	if c.offsetX > 0 {
		c.offsetX = 0
	}
	minX := -(c.worldW - c.screenW)
	if c.offsetX < minX {
		c.offsetX = minX
	}
	if c.worldW < c.screenW {
		c.offsetX = (c.screenW - c.worldW) / 2
	}
	// clamp Y
	if c.offsetY > 0 {
		c.offsetY = 0
	}
	minY := -(c.worldH - c.screenH)
	if c.offsetY < minY {
		c.offsetY = minY
	}
	if c.worldH < c.screenH {
		c.offsetY = (c.screenH - c.worldH) / 2
	}
}

func (c *camera) transformXY(x, y int) (int, int) {
	return x + c.offsetX, y + c.offsetY
}

type cameraImage struct {
	Image
	camera *camera
}

func (img cameraImage) DrawAt(x, y int) {
	img.Image.DrawAt(img.camera.transformXY(x, y))
}

func (img cameraImage) DrawAtEx(x, y int, options DrawOptions) {
	x, y = img.camera.transformXY(x, y)
	img.Image.DrawAtEx(x, y, options)
}

func (img cameraImage) DrawRectAt(x, y int, source Rectangle) {
	x, y = img.camera.transformXY(x, y)
	img.Image.DrawRectAt(x, y, source)
}

type game struct {
	resources Resources

	camera camera

	levelDone         bool
	enteringGate      bool
	cloudDisappearing bool
	exitGlow          float32
	cloudSound        Sound

	helpImage    Image
	cavemanStand Image
	cavemanWalk  [4]Image
	cavemanPush  [4]Image
	cavemanFall  Image
	rock         Image
	gateGlowA    Image
	gateGlowB    Image
	gateCloud    Image
	tiles        Image

	gateGlowRatio float32
	gateGlowDelta float32

	pushFrameIndex      int
	nextPushFrameChange int

	cavemanX, cavemanY  int
	cavemanSpeedY       int
	cavemanIsOnGround   bool
	cavemanFacesRight   bool
	cavemanHitBox       Rectangle
	rockHitBox          Rectangle
	walkFrameIndex      int
	nextWalkFrameChange int

	exitX, exitY   int
	exitFacesRight bool

	rocks []rock

	leftDown  bool
	rightDown bool
	upDown    bool

	tileMap tileMap
}

type rock struct {
	Rectangle
	speedX   float32
	speedY   int
	rotation float32
}

func (r *rock) push(xDir int) {
	acceleration := float32(0.05)
	if xDir < 0 {
		acceleration = -acceleration
	}
	r.speedX += acceleration
	if r.speedX > 3 {
		r.speedX = 3
	}
	if r.speedX < -3 {
		r.speedX = -3
	}
}

func (r *rock) update(m *tileMap, caveman Rectangle, others []rock, myIndex int) {
	overlapsOther := func(r Rectangle) bool {
		for i := range others {
			if i == myIndex {
				continue
			}
			if others[i].overlaps(r) {
				return true
			}
		}
		return false
	}

	const xGravity = 0.025
	if r.speedX > 0 {
		r.speedX -= xGravity
		if r.speedX < 0 {
			r.speedX = 0
		}
	}
	if r.speedX < 0 {
		r.speedX += xGravity
		if r.speedX > 0 {
			r.speedX = 0
		}
	}

	round := func(x float32) int {
		if x < 0 {
			return int(x - 0.5)
		}
		return int(x + 0.5)
	}

	dx, hitWall := m.moveInX(r.Rectangle, round(r.speedX))
	backoff := 1
	if dx < 0 {
		backoff = -1
	}
	r.X += dx
	for r.overlaps(caveman) || overlapsOther(r.Rectangle) {
		if dx == 0 {
			break
		}
		dx -= backoff
		r.X -= backoff
	}
	r.rotation += float32(dx) * 0.667
	if hitWall {
		r.speedX = 0
	}

	r.speedY -= 2
	dy, hitMap := m.moveInY(r.Rectangle, r.speedY)
	backoff = 1
	if dy < 0 {
		backoff = -1
	}
	r.Y += dy
	for r.overlaps(caveman) || overlapsOther(r.Rectangle) {
		if dy == 0 {
			break
		}
		dy -= backoff
		r.Y -= backoff
	}
	if hitMap || dy == 0 {
		r.speedY = 0
	}
}

func (g *game) loadImage(id string) Image {
	return cameraImage{
		Image:  g.resources.LoadImage(id),
		camera: &g.camera,
	}
}

func (g *game) init(info Info, levelIndex int) {
	g.cavemanHitBox = info.CavemanHitBox
	g.rockHitBox = info.RockHitBox

	g.helpImage = g.resources.LoadImage("controls")
	g.cavemanStand = g.loadImage("caveman_stand_left")
	g.cavemanPush[0] = g.loadImage("caveman_push_left_0")
	g.cavemanPush[1] = g.loadImage("caveman_push_left_1")
	g.cavemanPush[2] = g.loadImage("caveman_push_left_2")
	g.cavemanPush[3] = g.loadImage("caveman_push_left_3")
	g.cavemanWalk[0] = g.loadImage("caveman_walk_left_0")
	g.cavemanWalk[1] = g.loadImage("caveman_walk_left_1")
	g.cavemanWalk[2] = g.loadImage("caveman_walk_left_2")
	g.cavemanWalk[3] = g.loadImage("caveman_walk_left_3")
	g.cavemanFall = g.loadImage("caveman_fall_left")
	g.rock = g.loadImage("rock")
	g.gateGlowA = g.loadImage("gate_a")
	g.gateGlowB = g.loadImage("gate_b")
	g.gateCloud = g.loadImage("gate_cloud")
	g.tiles = g.loadImage("tiles")

	g.cloudSound = g.resources.LoadSound("cloud")

	levelName := "level_" + strconv.Itoa(levelIndex) + ".tmx"
	level, err := tiled.Read(bytes.NewReader(g.resources.LoadFile(levelName)))
	if err != nil {
		log.Fatalf("unable to decode %v: ", levelName, err)
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
							Rectangle: Rectangle{
								X: worldX + g.rockHitBox.X,
								Y: worldY + g.rockHitBox.Y,
								W: g.rockHitBox.W,
								H: g.rockHitBox.H,
							},
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
	g.camera.setWorldSize(g.tileMap.worldSize())
}

func (g *game) SetScreenSize(width, height int) {
	g.camera.setScreenSize(width, height)
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

	if g.enteringGate {
		g.leftDown = false
		g.rightDown = false
		g.upDown = false
	}

	cavemanBounds := Rectangle{
		g.cavemanX + g.cavemanHitBox.X,
		g.cavemanY + g.cavemanHitBox.Y,
		g.cavemanHitBox.W,
		g.cavemanHitBox.H,
	}
	for i := range g.rocks {
		g.rocks[i].update(&g.tileMap, cavemanBounds, g.rocks, i)
	}

	cavemanPushing := false

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

	if g.cavemanIsOnGround && g.upDown {
		g.cavemanSpeedY = 20
	}

	g.cavemanSpeedY -= 1
	if g.cavemanSpeedY < -14 {
		g.cavemanSpeedY = -14
	}

	cavemanW, cavemanH := g.cavemanStand.Size()
	cavemanRect := Rectangle{
		g.cavemanX + g.cavemanHitBox.X,
		g.cavemanY + g.cavemanHitBox.Y,
		g.cavemanHitBox.W,
		g.cavemanHitBox.H,
	}
	var dx, dy int
	dx, _ = g.tileMap.moveInX(cavemanRect, cavemanDx)
	if dx != 0 {
		newDx, hitRock, rock := g.moveCavemanInX(cavemanRect, dx)
		if hitRock && g.cavemanIsOnGround {
			cavemanPushing = true
			rock.push(dx)
		}
		dx = newDx
	}
	cavemanRect.X += dx
	var hitMap, hitObj bool
	dy, hitMap = g.tileMap.moveInY(cavemanRect, g.cavemanSpeedY)
	dy, hitObj = g.moveCavemanInY(cavemanRect, dy)
	hitInY := hitMap || hitObj
	g.cavemanIsOnGround = false
	if hitInY {
		if g.cavemanSpeedY < 0 {
			g.cavemanIsOnGround = true
		} else {
			g.cavemanSpeedY = 0
		}
	}
	cavemanRect.Y += dy

	cavemanCenterX := cavemanRect.X + cavemanRect.W/2
	exitMinX, exitMaxX := g.exitX-100, g.exitX-20
	if g.exitFacesRight {
		w, _ := g.gateGlowA.Size()
		exitMinX, exitMaxX = g.exitX+w+20, g.exitX+w+100
	}
	if !g.enteringGate &&
		cavemanRect.Y == g.exitY &&
		cavemanCenterX > exitMinX && cavemanCenterX < exitMaxX {
		g.enteringGate = true
		g.cloudSound.Play()
	}

	g.cavemanX = cavemanRect.X - g.cavemanHitBox.X
	g.cavemanY = cavemanRect.Y - g.cavemanHitBox.Y
	g.camera.centerAround(
		g.cavemanX+cavemanW/2,
		g.cavemanY+cavemanH/2,
	)
	if g.cavemanIsOnGround {
		g.cavemanSpeedY = 0
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

	g.nextPushFrameChange++
	if g.nextPushFrameChange > 10 {
		g.nextPushFrameChange = 0
		g.pushFrameIndex = (g.pushFrameIndex + 1) % len(g.cavemanPush)
	}

	g.nextWalkFrameChange++
	if g.nextWalkFrameChange > 7 {
		g.nextWalkFrameChange = 0
		g.walkFrameIndex = (g.walkFrameIndex + 1) % len(g.cavemanWalk)
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

	for i := range g.rocks {
		g.rock.DrawAtEx(
			g.rocks[i].X-g.rockHitBox.X,
			g.rocks[i].Y-g.rockHitBox.Y,
			centerRotation(g.rocks[i].rotation),
		)
	}

	g.gateGlowA.DrawAtEx(g.exitX, g.exitY, flipX(g.exitFacesRight))
	g.gateGlowB.DrawAtEx(g.exitX, g.exitY, flipX(g.exitFacesRight).opacity(g.gateGlowRatio))

	caveman := g.cavemanStand
	if !g.cavemanIsOnGround {
		caveman = g.cavemanFall
	} else if cavemanPushing {
		caveman = g.cavemanPush[g.pushFrameIndex]
	} else if xor(g.leftDown, g.rightDown) {
		caveman = g.cavemanWalk[g.walkFrameIndex]
	}
	if !g.enteringGate || !g.cloudDisappearing {
		caveman.DrawAtEx(
			g.cavemanX,
			g.cavemanY,
			flipX(g.cavemanFacesRight).opacity(1-g.exitGlow),
		)
	}

	if g.enteringGate {
		const cloudSpeed = 0.0077
		if !g.cloudDisappearing {
			g.exitGlow += cloudSpeed
			if g.exitGlow > 1 {
				g.exitGlow = 1
				g.cloudDisappearing = true
			}
		} else {
			g.exitGlow -= cloudSpeed
			if g.exitGlow < 0 {
				g.exitGlow = 0
				g.levelDone = true
			}
		}

		x, y := g.exitX-200, g.exitY-20
		if g.exitFacesRight {
			w, _ := g.gateGlowA.Size()
			cloudW, _ := g.gateCloud.Size()
			x = g.exitX + w + 200 - cloudW
		}
		g.gateCloud.DrawAtEx(x, y, flipX(g.exitFacesRight).opacity(g.exitGlow))
	}

	g.helpImage.DrawAt(0, 0)
}

func xor(a, b bool) bool {
	return a && !b || !a && b
}

func (g *game) levelFinished() bool {
	return g.levelDone
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

func centerRotation(value float32) DrawOptions {
	return DrawOptions{CenterRotationDeg: value}
}

func (o DrawOptions) centerRotation(value float32) DrawOptions {
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

func (m *tileMap) worldSize() (int, int) {
	return m.width * m.tileW, m.height * m.tileH
}

func (g *game) moveCavemanInX(start Rectangle, dx int) (realDx int, hitObject bool, hit *rock) {
	startX := start.X
	if dx < 0 {
		r := start
		r.X += dx
		r.W -= dx
		newX := r.X
		for i := range g.rocks {
			bounds := g.rocks[i].Rectangle
			if r.overlaps(bounds) {
				right := bounds.X + bounds.W
				if right > newX {
					newX = right
					hit = &g.rocks[i]
				}
			}
		}
		if newX != r.X {
			hitObject = true
		}
		start.X = newX
	} else if dx > 0 {
		r := start
		r.W += dx
		newRight := r.X + r.W - 1
		for i := range g.rocks {
			bounds := g.rocks[i].Rectangle
			if r.overlaps(bounds) {
				left := bounds.X - 1
				if left < newRight {
					newRight = left
					hit = &g.rocks[i]
				}
			}
		}
		if newRight != r.X+r.W-1 {
			hitObject = true
		}
		start.X = newRight - start.W + 1
	}

	realDx = start.X - startX
	return
}

func (g *game) moveCavemanInY(start Rectangle, dy int) (realDy int, hitObject bool) {
	startY := start.Y
	if dy < 0 {
		r := start
		r.Y += dy
		r.H -= dy
		newY := r.Y
		for i := range g.rocks {
			bounds := g.rocks[i].Rectangle
			if r.overlaps(bounds) {
				bottom := bounds.Y + bounds.H
				if bottom > newY {
					newY = bottom
				}
			}
		}
		if newY != r.Y {
			hitObject = true
		}
		start.Y = newY
	} else if dy > 0 {
		r := start
		r.H += dy
		newBottom := r.Y + r.H - 1
		for i := range g.rocks {
			bounds := g.rocks[i].Rectangle
			if r.overlaps(bounds) {
				top := bounds.Y - 1
				if top < newBottom {
					newBottom = top
				}
			}
		}
		if newBottom != r.Y+r.H-1 {
			hitObject = true
		}
		start.Y = newBottom - start.H + 1
	}

	realDy = start.Y - startY
	return
}

func (m *tileMap) moveInX(start Rectangle, dx int) (realDx int, hitWall bool) {
	startX := start.X
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

	realDx = start.X - startX
	return
}

func (m *tileMap) moveInY(start Rectangle, dy int) (realDy int, hitWall bool) {
	startY := start.Y
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

	realDy = start.Y - startY
	return
}
