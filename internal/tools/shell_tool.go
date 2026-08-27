package tools

type ShellTool struct{}

func (ShellTool) List() []Definition {
	return []Definition{{Name: "run_command", Description: "Run shell command"}, {Name: "shell_status", Description: "Get shell status"}}
}

func (ShellTool) Call(name string, args map[string]any) (any, error) { return args, nil }
