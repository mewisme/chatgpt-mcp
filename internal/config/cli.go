package config

import "os"

func Env() string { return os.Getenv("CHATGPT_MCP_CONFIG") }
