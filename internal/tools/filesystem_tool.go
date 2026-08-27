package tools

import "encoding/json"

type FileTool struct{ Service FileService }

func (t FileTool) List() []Definition {
	return []Definition{{Name: "read_text_file", Description: "Read text file"}, {Name: "read_files", Description: "Read multiple files"}, {Name: "write_file", Description: "Write file"}}
}

func (t FileTool) Call(name string, args map[string]any) (any, error) {
	data, _ := json.Marshal(args)
	return string(data), nil
}
