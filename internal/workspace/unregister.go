package workspace

import "fmt"

func (m *Manager) Unregister(id string) error {
	if err := m.ensureLoaded(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	canonical := m.canonicalIDLocked(id)
	item, ok := m.items[canonical]
	if !ok {
		return fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	delete(m.items, canonical)
	removedAliases := map[string]string{}
	for alias, target := range m.aliases {
		if target == canonical {
			removedAliases[alias] = target
			delete(m.aliases, alias)
		}
	}
	if err := m.saveLocked(); err != nil {
		m.items[canonical] = item
		for alias, target := range removedAliases {
			m.aliases[alias] = target
		}
		return err
	}
	return nil
}
