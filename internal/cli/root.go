package cli

import (
	"fmt"
	"github.com/spf13/cobra"
	"go.mewis.me/chatgpt-mcp/internal/config"
)

var root = &cobra.Command{Use: "chatgpt-mcp"}

func init() {
	root.AddCommand(&cobra.Command{Use: "init", RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Save(config.Default()); err != nil { return err }
		fmt.Println(config.Path())
		return nil
	}})
	root.AddCommand(&cobra.Command{Use: "config-path", Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(config.Path())
	}})
}

func Execute() {
	if err := root.Execute(); err != nil { panic(err) }
}
