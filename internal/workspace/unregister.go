package workspace

import "fmt"

func (m *Manager) Unregister(id string) error {
	if err := m.ensureLoaded(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.items[id]
	if !ok {
		return fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	delete(m.items, id)
	if err := m.saveLocked(); err != nil {
		m.items[id] = item
		return err
	}
	return nil
}
