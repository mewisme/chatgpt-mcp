package workspace

type Context struct {
	Workspace Workspace
	Root      string
}

func (m *Manager) ResolveContext(id string) (Context, error) {
	item, err := m.Get(id)
	if err != nil {
		return Context{}, err
	}
	return Context{Workspace: item, Root: item.Path}, nil
}

func (m *Manager) ResolveWorkspacePath(id, input string, mustExist bool) (Context, string, error) {
	ctx, err := m.ResolveContext(id)
	if err != nil {
		return Context{}, "", err
	}
	resolved, err := m.ResolvePath(id, ctx.Root, input, mustExist)
	if err != nil {
		return Context{}, "", err
	}
	return ctx, resolved, nil
}
