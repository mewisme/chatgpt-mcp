package cftunnel

type LifecycleState string

const (
	LifecycleConnecting   LifecycleState = "connecting"
	LifecycleReconnecting LifecycleState = "reconnecting"
	LifecycleReady        LifecycleState = "ready"
	LifecycleDegraded     LifecycleState = "degraded"
	LifecycleStopped      LifecycleState = "stopped"
)

type LifecycleEvent struct {
	State  LifecycleState
	Target string
	URL    string
	Origin string
	Error  string
}

type LifecycleObserver func(LifecycleEvent)

func (s TargetStatus) Line() string {
	switch {
	case s.Restarting:
		if s.LastError != "" {
			return "reconnecting · " + s.LastError
		}
		return "reconnecting"
	case s.LastError != "":
		return "degraded · " + s.LastError
	case s.Ready && s.URL != "":
		return s.URL + " · ephemeral"
	case s.Running:
		return "connecting"
	case s.Desired:
		return "offline"
	default:
		return "disabled"
	}
}
