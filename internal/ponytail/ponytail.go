package ponytail

type Mode string

const (
	Off   Mode = "off"
	Lite  Mode = "lite"
	Full  Mode = "full"
	Ultra Mode = "ultra"
)

type Session struct{ Mode Mode }
