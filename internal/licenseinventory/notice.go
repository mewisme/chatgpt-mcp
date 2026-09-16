package licenseinventory

import (
	"fmt"
	"strings"
)

const noticeCopyright = "Copyright 2026 Nguyễn Mậu Minh"

func renderNOTICE(inv Inventory) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "chatgpt-mcp %s\n%s\n\n", inv.Artifact, noticeCopyright)
	if inv.Artifact == "bash" {
		builder.WriteString("This artifact distributes Git for Windows PortableGit under GPL-2.0-only and bundled component licenses.\nCGM-authored plugin metadata remains Apache-2.0. Full texts are in licenses.txt.\n\n")
	} else {
		builder.WriteString("Licensed under the Apache License, Version 2.0.\nThis artifact includes third-party software. Full license texts are in licenses.txt.\n\n")
	}
	builder.WriteString("Third-party components:\n")
	for _, pkg := range inv.Packages {
		if pkg.Kind == KindRoot {
			continue
		}
		fmt.Fprintf(&builder, "\n- %s %s (%s)\n", pkg.Name, pkg.Version, pkg.License)
		if pkg.Source != "" {
			fmt.Fprintf(&builder, "  %s\n", pkg.Source)
		} else if pkg.Path != "" {
			fmt.Fprintf(&builder, "  %s\n", pkg.Path)
		}
	}
	for _, notice := range inv.Notices {
		builder.WriteString("\n")
		builder.WriteString(notice)
		builder.WriteString("\n")
	}
	return builder.String()
}
