package tools

import "fmt"

type Context struct {
	WorkingDirectory string
}

func RequireWorkingDirectory(args map[string]any) error {
	value, ok := args["working_directory"].(string)
	if !ok || value == "" {
		return fmt.Errorf("working_directory is required")
	}
	return nil
}
