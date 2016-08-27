package game

type Key int

const (
	KeyLeft Key = 1 + iota
	KeyRight
	KeyUp
)

type InputEvent struct {
	Down bool
	Key  Key
}
