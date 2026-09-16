package main

import (
	"flag"
	"fmt"
	"os"

	"go.mewis.me/chatgpt-mcp/internal/pluginbuild"
)

func main() {
	var templatePath, outputRoot, platform, repoRoot string
	flag.StringVar(&templatePath, "template", "plugins/ponytail/plugin.json", "plugin manifest template")
	flag.StringVar(&outputRoot, "output", "dist/plugins", "release output directory")
	flag.StringVar(&platform, "platform", "", "optional single platform such as linux/amd64")
	flag.StringVar(&repoRoot, "repo-root", ".", "repository root")
	flag.Parse()
	manifestPath, err := pluginbuild.Build(pluginbuild.Request{
		RepoRoot: repoRoot, TemplatePath: templatePath, OutputRoot: outputRoot, OnlyPlatform: platform,
		Package: "./plugins/ponytail/cmd/ponytail",
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("manifest=%s\n", manifestPath)
}
