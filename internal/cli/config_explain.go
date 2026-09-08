package cli

import (
	"encoding/json"
	"strings"

	"github.com/spf13/cobra"
	"go.mewis.me/chatgpt-mcp/internal/config"
)

func configExplainCommand() *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "explain [key]",
		Short: "Explain a configuration key or subtree from the config schema",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := ""
			if len(args) > 0 {
				key = args[0]
			}
			explanation, err := config.Explain(key)
			if err != nil {
				return err
			}
			if jsonOutput {
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetIndent("", "  ")
				encoder.SetEscapeHTML(false)
				return encoder.Encode(explanation)
			}
			printConfigExplanation(cmd, explanation, 0)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output schema explanation as JSON")
	cmd.ValidArgsFunction = completeConfigSelection
	return cmd
}

func printConfigExplanation(cmd *cobra.Command, explanation config.Explanation, depth int) {
	indent := strings.Repeat("  ", depth)
	if depth == 0 {
		if explanation.Key != "" {
			cmd.Println(explanation.Key)
		}
		if explanation.Label != "" && explanation.Label != explanation.Key {
			cmd.Println(explanation.Label)
		}
		if explanation.Description != "" {
			cmd.Println()
			cmd.Println(explanation.Description)
		}
		if explanation.Details != "" {
			cmd.Println(explanation.Details)
		}
		if !explanation.Branch {
			printConfigFieldMetadata(cmd, explanation)
			return
		}
		if len(explanation.Children) > 0 {
			cmd.Println()
		}
	} else {
		cmd.Printf("%s%s\n", indent, explanation.Key)
		if explanation.Description != "" {
			cmd.Printf("%s  %s\n", indent, explanation.Description)
		}
	}
	for index, child := range explanation.Children {
		printConfigExplanation(cmd, child, depth+1)
		if depth == 0 && index < len(explanation.Children)-1 {
			cmd.Println()
		}
	}
}

func printConfigFieldMetadata(cmd *cobra.Command, explanation config.Explanation) {
	cmd.Println()
	cmd.Printf("Type: %s\n", explanation.Kind)
	cmd.Printf("Default: %s\n", formatExplainDefault(explanation.Default))
	cmd.Printf("Editable: %t\n", explanation.Editable)
	if explanation.Sensitive {
		cmd.Println("Sensitive: true")
	}
	if len(explanation.Values) > 0 {
		cmd.Println("Values:")
		for _, value := range explanation.Values {
			if value.Description == "" {
				cmd.Printf("  %s\n", value.Value)
			} else {
				cmd.Printf("  %-10s %s\n", value.Value, value.Description)
			}
		}
	}
	if explanation.Guidance != "" {
		cmd.Printf("Guidance: %s\n", explanation.Guidance)
	}
	if len(explanation.Related) > 0 {
		cmd.Println("Related:")
		for _, key := range explanation.Related {
			cmd.Printf("  %s\n", key)
		}
	}
}

func formatExplainDefault(value string) string {
	if value == "" {
		return "<empty>"
	}
	return value
}
