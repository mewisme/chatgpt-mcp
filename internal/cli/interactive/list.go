package interactive

import (
	"context"
	"errors"
	"io"
	"os"

	tea "charm.land/bubbletea/v2"
	"golang.org/x/term"
)

type Confirmation struct {
	Action string
	Target string
}

func (c Confirmation) Active() bool                 { return c.Action != "" }
func (c *Confirmation) Start(action, target string) { c.Action, c.Target = action, target }
func (c *Confirmation) Clear()                      { c.Action, c.Target = "", "" }
func (c Confirmation) View() string {
	if !c.Active() {
		return ""
	}
	return c.Target + " [y/N]"
}

func TerminalIO(in io.Reader, out io.Writer) bool {
	input, inputOK := in.(*os.File)
	output, outputOK := out.(*os.File)
	return inputOK && outputOK && term.IsTerminal(int(input.Fd())) && term.IsTerminal(int(output.Fd()))
}

func ResolveMode(in io.Reader, out io.Writer, force, disable, json bool) (bool, error) {
	if force && disable {
		return false, errors.New("--interactive and --no-interactive cannot be used together")
	}
	if json || disable {
		return false, nil
	}
	terminal := TerminalIO(in, out)
	if force && !terminal {
		return false, errors.New("--interactive requires terminal stdin and stdout")
	}
	return force || terminal, nil
}

func Run(ctx context.Context, model tea.Model, in io.Reader, out io.Writer) (tea.Model, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return tea.NewProgram(model, tea.WithContext(ctx), tea.WithInput(in), tea.WithOutput(out)).Run()
}
