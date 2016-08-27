package game

type Game interface {
	Frame([]InputEvent)
}

type Resources interface {
	LoadImage(id string) Image
}

type DrawOptions struct {
	FlipX             bool
	Transparency      float32
	CenterRotationDeg int
}

type Image interface {
	DrawAt(x, y int)
	DrawAtEx(x, y int, options DrawOptions)
	Size() (width, height int)
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

	caveman       Image
	cavemanPush   Image
	rock          Image
	gateGlowA     Image
	gateGlowB     Image
	gateGlowRatio float32
	gateGlowDelta float32

	cavemanX     int
	cavemanFlipX bool

	rockX        int
	rockRotation int

	leftDown  bool
	rightDown bool
	upDown    bool
}

func (g *game) init() {
	g.caveman = g.resources.LoadImage("caveman_stand_left")
	g.cavemanPush = g.resources.LoadImage("caveman_push_left")
	g.rock = g.resources.LoadImage("rock")
	g.gateGlowA = g.resources.LoadImage("gate_a")
	g.gateGlowB = g.resources.LoadImage("gate_b")
}

func (g *game) Frame(events []InputEvent) {
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

	const speed = 3
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
