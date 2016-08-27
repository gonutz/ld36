package game

type Game interface {
	Frame([]InputEvent)
}

type Resources interface {
	LoadImage(id string) Image
}

type Image interface {
	DrawAt(x, y int)
	DrawAtRotatedCW(x, y, degrees int)
	DrawAtFlipX(x, y int, flipX bool)
	Size() (width, height int)
}

func New(resources Resources) Game {
	g := &game{
		resources: resources,
	}
	g.init()
	return g
}

type game struct {
	resources Resources

	caveman Image
	rock    Image

	cavemanX     int
	cavemanFlipX bool

	rockX        int
	rockRotation int

	leftDown  bool
	rightDown bool
}

func (g *game) init() {
	g.caveman = g.resources.LoadImage("caveman_stand_left")
	g.rock = g.resources.LoadImage("rock")
}

func (g *game) Frame(events []InputEvent) {
	for _, e := range events {
		switch e.Key {
		case KeyLeft:
			g.leftDown = e.Down
		case KeyRight:
			g.rightDown = e.Down
		}
	}

	const speed = 8
	if g.leftDown && !g.rightDown {
		g.cavemanX -= speed
		g.cavemanFlipX = false
	}
	if g.rightDown && !g.leftDown {
		g.cavemanX += speed
		g.cavemanFlipX = true
	}

	g.caveman.DrawAtFlipX(g.cavemanX, 50, g.cavemanFlipX)
	g.rockRotation += 2
	g.rockX += 3
	g.rock.DrawAtRotatedCW(g.rockX, 50, g.rockRotation)
}
