package interactive

import (
	"context"
	"errors"
	"io"
	"os"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"golang.org/x/term"
)

type ListKeys struct {
	Up, Down, Open, Approve, Deny, Filter, Refresh, Quit key.Binding
}

func DefaultListKeys() ListKeys {
	return ListKeys{
		Up: key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("up/k", "up")), Down: key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("down/j", "down")),
		Open: key.NewBinding(key.WithKeys("enter", "v"), key.WithHelp("enter/v", "details")), Approve: key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "approve")), Deny: key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "deny")),
		Filter: key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")), Refresh: key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")), Quit: key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}

type Cursor struct{ Index int }

func (c *Cursor) Clamp(length int) {
	if length <= 0 {
		c.Index = 0
		return
	}
	if c.Index < 0 {
		c.Index = 0
	}
	if c.Index >= length {
		c.Index = length - 1
	}
}

func (c *Cursor) Move(delta, length int) {
	if length <= 0 {
		c.Index = 0
		return
	}
	c.Index = (c.Index + delta + length) % length
}

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
