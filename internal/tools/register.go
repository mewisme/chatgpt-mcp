package tools

func RegisterCore(r *Registry) {
	r.Register("read_text_file", func(args map[string]any) (any, error) { return map[string]any{"ok": true}, nil })
	r.Register("read_files", func(args map[string]any) (any, error) { return map[string]any{"ok": true}, nil })
	r.Register("write_file", func(args map[string]any) (any, error) { return map[string]any{"ok": true}, nil })
	r.Register("run_command", func(args map[string]any) (any, error) { return map[string]any{"ok": true}, nil })
	r.Register("git_status", func(args map[string]any) (any, error) { return map[string]any{"ok": true}, nil })
}
