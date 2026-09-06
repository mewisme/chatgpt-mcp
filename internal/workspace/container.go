package workspace

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

var ErrContainerNotFound = errors.New("workspace container not found")

func (m *Manager) CreateContainer(name string) (WorkspaceContainer, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return WorkspaceContainer{}, errors.New("workspace container name is required")
	}
	if err := m.ensureLoaded(); err != nil {
		return WorkspaceContainer{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for attempts := 0; attempts < 8; attempts++ {
		id, err := workspaceContainerID()
		if err != nil {
			return WorkspaceContainer{}, err
		}
		if _, exists := m.containers[id]; exists {
			continue
		}
		container := WorkspaceContainer{ID: id, Name: name, WorkspaceIDs: []string{}}
		m.containers[id] = container
		if err := m.saveLocked(); err != nil {
			delete(m.containers, id)
			return WorkspaceContainer{}, err
		}
		return container, nil
	}
	return WorkspaceContainer{}, errors.New("failed to allocate unique workspace container id")
}

func (m *Manager) GetContainer(id string) (WorkspaceContainer, error) {
	if err := m.ensureLoaded(); err != nil {
		return WorkspaceContainer{}, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	container, ok := m.containers[strings.TrimSpace(id)]
	if !ok {
		return WorkspaceContainer{}, fmt.Errorf("%w: %s", ErrContainerNotFound, id)
	}
	container.WorkspaceIDs = append([]string(nil), container.WorkspaceIDs...)
	return container, nil
}

func (m *Manager) ListContainers() ([]WorkspaceContainer, error) {
	if err := m.ensureLoaded(); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	values := make([]WorkspaceContainer, 0, len(m.containers))
	for _, container := range m.containers {
		container.WorkspaceIDs = append([]string(nil), container.WorkspaceIDs...)
		values = append(values, container)
	}
	sort.Slice(values, func(i, j int) bool {
		left, right := strings.ToLower(values[i].Name), strings.ToLower(values[j].Name)
		if left == right {
			return values[i].ID < values[j].ID
		}
		return left < right
	})
	return values, nil
}

func (m *Manager) RenameContainer(id, name string) (WorkspaceContainer, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return WorkspaceContainer{}, errors.New("workspace container name is required")
	}
	if err := m.ensureLoaded(); err != nil {
		return WorkspaceContainer{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	id = strings.TrimSpace(id)
	container, ok := m.containers[id]
	if !ok {
		return WorkspaceContainer{}, fmt.Errorf("%w: %s", ErrContainerNotFound, id)
	}
	previous := container
	container.Name = name
	m.containers[id] = container
	if err := m.saveLocked(); err != nil {
		m.containers[id] = previous
		return WorkspaceContainer{}, err
	}
	return container, nil
}

func (m *Manager) DeleteContainer(id string) error {
	if err := m.ensureLoaded(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	id = strings.TrimSpace(id)
	container, ok := m.containers[id]
	if !ok {
		return fmt.Errorf("%w: %s", ErrContainerNotFound, id)
	}
	delete(m.containers, id)
	if err := m.saveLocked(); err != nil {
		m.containers[id] = container
		return err
	}
	return nil
}

func (m *Manager) AddWorkspaceToContainer(containerID, workspaceID string) (WorkspaceContainer, error) {
	return m.updateWorkspaceContainerMembership(containerID, []string{workspaceID}, true)
}

func (m *Manager) RemoveWorkspaceFromContainer(containerID, workspaceID string) (WorkspaceContainer, error) {
	return m.updateWorkspaceContainerMembership(containerID, []string{workspaceID}, false)
}

func (m *Manager) AddWorkspacesToContainer(containerID string, workspaceIDs []string) (WorkspaceContainer, error) {
	return m.updateWorkspaceContainerMembership(containerID, workspaceIDs, true)
}

func (m *Manager) RemoveWorkspacesFromContainer(containerID string, workspaceIDs []string) (WorkspaceContainer, error) {
	return m.updateWorkspaceContainerMembership(containerID, workspaceIDs, false)
}

func (m *Manager) AddWorkspaceToContainers(workspaceID string, containerIDs []string) ([]WorkspaceContainer, error) {
	return m.updateWorkspaceContainersForWorkspace(workspaceID, containerIDs, true)
}

func (m *Manager) RemoveWorkspaceFromContainers(workspaceID string, containerIDs []string) ([]WorkspaceContainer, error) {
	return m.updateWorkspaceContainersForWorkspace(workspaceID, containerIDs, false)
}

func (m *Manager) ContainersForWorkspace(workspaceID string) ([]WorkspaceContainer, error) {
	if err := m.ensureLoaded(); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	canonical := m.canonicalIDLocked(strings.TrimSpace(workspaceID))
	if _, ok := m.items[canonical]; !ok {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, workspaceID)
	}
	values := []WorkspaceContainer{}
	for _, container := range m.containers {
		if containsString(container.WorkspaceIDs, canonical) {
			container.WorkspaceIDs = append([]string(nil), container.WorkspaceIDs...)
			values = append(values, container)
		}
	}
	sort.Slice(values, func(i, j int) bool {
		left, right := strings.ToLower(values[i].Name), strings.ToLower(values[j].Name)
		if left == right {
			return values[i].ID < values[j].ID
		}
		return left < right
	})
	return values, nil
}

func (m *Manager) WorkspacesForContainer(containerID string) ([]Workspace, error) {
	if err := m.ensureLoaded(); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	container, ok := m.containers[strings.TrimSpace(containerID)]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrContainerNotFound, containerID)
	}
	values := make([]Workspace, 0, len(container.WorkspaceIDs))
	for _, id := range container.WorkspaceIDs {
		if item, exists := m.items[id]; exists {
			values = append(values, item)
		}
	}
	sort.Slice(values, func(i, j int) bool { return values[i].Path < values[j].Path })
	return values, nil
}

func (m *Manager) updateWorkspaceContainerMembership(containerID string, workspaceIDs []string, add bool) (WorkspaceContainer, error) {
	if err := m.ensureLoaded(); err != nil {
		return WorkspaceContainer{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	containerID = strings.TrimSpace(containerID)
	container, ok := m.containers[containerID]
	if !ok {
		return WorkspaceContainer{}, fmt.Errorf("%w: %s", ErrContainerNotFound, containerID)
	}
	canonical, err := m.validateWorkspaceIDsLocked(workspaceIDs)
	if err != nil {
		return WorkspaceContainer{}, err
	}
	previous := append([]string(nil), container.WorkspaceIDs...)
	if add {
		container.WorkspaceIDs = append(container.WorkspaceIDs, canonical...)
	} else {
		container.WorkspaceIDs = removeStrings(container.WorkspaceIDs, canonical)
	}
	container.WorkspaceIDs = normalizeContainerWorkspaceIDs(container.WorkspaceIDs, m.items)
	m.containers[containerID] = container
	if err := m.saveLocked(); err != nil {
		container.WorkspaceIDs = previous
		m.containers[containerID] = container
		return WorkspaceContainer{}, err
	}
	return container, nil
}

func (m *Manager) updateWorkspaceContainersForWorkspace(workspaceID string, containerIDs []string, add bool) ([]WorkspaceContainer, error) {
	if err := m.ensureLoaded(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	canonical := m.canonicalIDLocked(strings.TrimSpace(workspaceID))
	if _, ok := m.items[canonical]; !ok {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, workspaceID)
	}
	ids := normalizeIDs(containerIDs, "")
	for _, id := range ids {
		if _, ok := m.containers[id]; !ok {
			return nil, fmt.Errorf("%w: %s", ErrContainerNotFound, id)
		}
	}
	previous := make(map[string]WorkspaceContainer, len(ids))
	for _, id := range ids {
		container := m.containers[id]
		previous[id] = container
		if add {
			container.WorkspaceIDs = append(container.WorkspaceIDs, canonical)
		} else {
			container.WorkspaceIDs = removeStrings(container.WorkspaceIDs, []string{canonical})
		}
		container.WorkspaceIDs = normalizeContainerWorkspaceIDs(container.WorkspaceIDs, m.items)
		m.containers[id] = container
	}
	if err := m.saveLocked(); err != nil {
		for id, container := range previous {
			m.containers[id] = container
		}
		return nil, err
	}
	result := make([]WorkspaceContainer, 0, len(ids))
	for _, id := range ids {
		result = append(result, m.containers[id])
	}
	return result, nil
}

func (m *Manager) validateWorkspaceIDsLocked(ids []string) ([]string, error) {
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		canonical := m.canonicalIDLocked(strings.TrimSpace(id))
		if canonical == "" {
			continue
		}
		if _, ok := m.items[canonical]; !ok {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, id)
		}
		result = appendUniqueString(result, canonical)
	}
	sort.Strings(result)
	return result, nil
}

func normalizeContainerWorkspaceIDs(ids []string, workspaces map[string]Workspace) []string {
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := workspaces[id]; !ok {
			continue
		}
		result = appendUniqueString(result, id)
	}
	sort.Strings(result)
	if len(result) == 0 {
		return []string{}
	}
	return result
}

func removeStrings(values, removals []string) []string {
	if len(values) == 0 || len(removals) == 0 {
		return append([]string(nil), values...)
	}
	remove := make(map[string]struct{}, len(removals))
	for _, value := range removals {
		remove[value] = struct{}{}
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := remove[value]; !ok {
			result = append(result, value)
		}
	}
	return result
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
