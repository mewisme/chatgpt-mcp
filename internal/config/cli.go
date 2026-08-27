package config

import "os"

func DefaultStore() *Store { return NewStore(DefaultPath()) }

func Env() string { return os.Getenv("CHATGPT_MCP_CONFIG") }
