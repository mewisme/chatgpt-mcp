package jsruntime

type Worker struct{ Workspace string }

func New(workspace string) *Worker { return &Worker{Workspace: workspace} }
func (w *Worker) Close() error     { return nil }
