package main

import (
	"fmt"\n\t"log"
	"net/http"

	"go.mewis.me/chatgpt-mcp/internal/app"
	"go.mewis.me/chatgpt-mcp/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	runtime := app.New(cfg)
	addr := cfg.Server.Host + ":" + fmt.Sprint(cfg.Server.Port)
	log.Printf("chatgpt-mcp listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, runtime.Handler()))
}
