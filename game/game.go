package game

type Game interface {
	Frame()
}

type Resources interface {
	LoadImage(id string) Image
}

type Image interface {
	DrawAt(x, y int)
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
}

func (g *game) init() {
	g.caveman = g.resources.LoadImage("caveman_stand_left")
	g.rock = g.resources.LoadImage("rock")
}

func (g *game) Frame() {
	g.caveman.DrawAt(500, 50)
	g.rock.DrawAt(350, 50)
}
