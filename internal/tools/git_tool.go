package tools

type GitTool struct{}

func (GitTool) List() []Definition {
	return []Definition{{Name: "git_status", Description: "Get git status"}, {Name: "git_diff", Description: "Get git diff"}}
}

func (GitTool) Call(name string, args map[string]any) (any, error) { return args, nil }
