package tools

func RegisterCore(r *Registry) {
	r.Register("read_text_file", DefaultSchema("read_text_file", "Read a text file"), func(args map[string]any) (any, error) { return nil, nil })
	r.Register("read_files", DefaultSchema("read_files", "Read multiple files"), func(args map[string]any) (any, error) { return nil, nil })
	r.Register("write_file", DefaultSchema("write_file", "Write a file"), func(args map[string]any) (any, error) { return nil, nil })
	r.Register("run_command", DefaultSchema("run_command", "Run command"), func(args map[string]any) (any, error) { return nil, nil })
	r.Register("git_status", DefaultSchema("git_status", "Git status"), func(args map[string]any) (any, error) { return nil, nil })
}
