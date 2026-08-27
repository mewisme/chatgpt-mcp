package hooks

type Hook struct {
	Pattern string
	Command string
}

type Manager struct{ Hooks []Hook }

func New() *Manager { return &Manager{} }
