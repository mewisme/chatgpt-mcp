package logger

import (
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/fatih/color"
)

var spinnerFrames = [...]string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

type jsonEvent struct {
	Time         string         `json:"time"`
	Level        string         `json:"level"`
	Event        string         `json:"event"`
	Kind         string         `json:"kind"`
	Message      string         `json:"message"`
	Component    string         `json:"component,omitempty"`
	RunID        string         `json:"run_id,omitempty"`
	PID          int            `json:"pid,omitempty"`
	Managed      bool           `json:"managed,omitempty"`
	ServiceID    string         `json:"service_id,omitempty"`
	ServiceScope string         `json:"service_scope,omitempty"`
	Fields       map[string]any `json:"fields,omitempty"`
	Error        string         `json:"error,omitempty"`
}

func (l *Logger) renderText(event Event) {
	if l.mode == ModeDebug {
		l.renderMu.Lock()
		defer l.renderMu.Unlock()
		l.renderDebugText(event)
		return
	}
	if l.animate && event.Kind == KindAction {
		l.startSpinner(event)
		return
	}
	if l.animate {
		spinner := l.detachSpinner()
		if spinner != nil {
			terminal := spinner.event.Component == event.Component && terminalKind(event.Kind)
			l.stopSpinnerState(spinner, true)
			l.renderMu.Lock()
			l.renderTextEvent(event)
			l.renderMu.Unlock()
			if !terminal {
				l.startSpinner(spinner.event)
			}
			return
		}
	}
	l.renderMu.Lock()
	defer l.renderMu.Unlock()
	l.renderTextEvent(event)
}

func (l *Logger) renderTextEvent(event Event) {
	if event.Name == "cli.detail" {
		value := any("")
		if len(event.Fields) > 0 {
			value = event.Fields[0].Value
		}
		fmt.Fprintf(l.out, "    %s %v\n", styled(color.Faint).Sprint(event.Message+":"), value)
		return
	}
	if l.showTime() {
		fmt.Fprint(l.out, styled(color.Faint).Sprint(l.eventTime(event).Format("15:04:05")), " ")
	}
	fmt.Fprint(l.out, symbolStyle(event.Kind).Sprint(symbol(event.Kind)), " ", capitalizeIconMessage(event.Message))
	if event.Err != nil {
		fmt.Fprint(l.out, ": ", event.Err)
	}
	fmt.Fprintln(l.out)
	for _, field := range event.Fields {
		if field.Visibility > l.visibility() {
			continue
		}
		renderField(l.out, field.Key, field.Value)
	}
}

func terminalKind(kind Kind) bool {
	return kind == KindSuccess || kind == KindWarning || kind == KindError
}

func (l *Logger) startSpinner(event Event) {
	if previous := l.detachSpinner(); previous != nil {
		l.stopSpinnerState(previous, true)
	}
	state := &spinnerState{event: event, stop: make(chan struct{}), done: make(chan struct{})}
	l.spinMu.Lock()
	l.spinner = state
	l.spinMu.Unlock()
	go l.runSpinner(state)
}

func (l *Logger) runSpinner(state *spinnerState) {
	defer close(state.done)
	rate := l.spinRate
	if rate <= 0 {
		rate = defaultSpinnerRate
	}
	ticker := time.NewTicker(rate)
	defer ticker.Stop()
	frame := 0
	for {
		l.renderSpinnerFrame(state.event, spinnerFrames[frame%len(spinnerFrames)])
		frame++
		select {
		case <-state.stop:
			return
		case <-ticker.C:
		}
	}
}

func (l *Logger) renderSpinnerFrame(event Event, frame string) {
	l.renderMu.Lock()
	defer l.renderMu.Unlock()
	fmt.Fprint(l.out, "\r\x1b[2K")
	if l.showTime() {
		fmt.Fprint(l.out, styled(color.Faint).Sprint(l.eventTime(event).Format("15:04:05")), " ")
	}
	fmt.Fprint(l.out, symbolStyle(KindAction).Sprint(frame), " ", capitalizeIconMessage(event.Message))
}

func (l *Logger) detachSpinner() *spinnerState {
	l.spinMu.Lock()
	defer l.spinMu.Unlock()
	state := l.spinner
	l.spinner = nil
	return state
}

func (l *Logger) stopSpinner(clear bool) {
	if state := l.detachSpinner(); state != nil {
		l.stopSpinnerState(state, clear)
	}
}

func (l *Logger) stopSpinnerState(state *spinnerState, clear bool) {
	close(state.stop)
	<-state.done
	if !clear {
		return
	}
	l.renderMu.Lock()
	fmt.Fprint(l.out, "\r\x1b[2K")
	l.renderMu.Unlock()
}

func capitalizeIconMessage(message string) string {
	if message == "" {
		return message
	}
	r, size := utf8.DecodeRuneInString(message)
	if r == utf8.RuneError && size == 0 {
		return message
	}
	upper := unicode.ToUpper(r)
	if upper == r {
		return message
	}
	return string(upper) + message[size:]
}

func (l *Logger) renderDebugText(event Event) {
	level := levelCode(event.Level)
	if l.showTime() {
		fmt.Fprint(l.out, styled(color.Faint).Sprint(l.eventTime(event).Format("15:04:05")), " ")
	}
	fmt.Fprintf(l.out, "%s %-10s %s %s", levelStyle(level).Sprintf("%-3s", level), styled(color.FgHiBlue, color.Bold).Sprintf("%-10s", strings.ToUpper(event.Component)), styled(color.Faint).Sprint(event.Name), event.Message)
	for _, field := range event.Fields {
		fmt.Fprintf(l.out, " %s=%v", styled(color.Faint).Sprint(field.Key), field.Value)
	}
	if event.Err != nil {
		fmt.Fprintf(l.out, " %s=%v", styled(color.Faint).Sprint("error"), event.Err)
	}
	fmt.Fprintln(l.out)
}

func (l *Logger) renderJSON(event Event) {
	fields := make(map[string]any, len(event.Fields))
	for _, field := range event.Fields {
		if field.Visibility > l.visibility() || strings.TrimSpace(field.Key) == "" {
			continue
		}
		fields[field.Key] = jsonValue(field.Value)
	}
	value := jsonEvent{Time: l.eventTime(event).UTC().Format(time.RFC3339Nano), Level: event.Level.String(), Event: event.Name, Kind: event.Kind.String(), Message: event.Message, Component: event.Component, RunID: event.RunID, PID: event.PID, Managed: event.Managed, ServiceID: event.ServiceID, ServiceScope: event.ServiceScope, Fields: fields}
	if event.Err != nil {
		value.Error = event.Err.Error()
	}
	_ = json.NewEncoder(l.out).Encode(value)
}

func renderField(out io.Writer, key string, value any) {
	key = strings.TrimSpace(key)
	if key == "" {
		key = "value"
	}
	label := styled(color.Faint).Sprint(key + ":")
	if values, ok := stringSlice(value); ok && len(values) > 1 {
		fmt.Fprintf(out, "    %s\n", label)
		for _, item := range values {
			fmt.Fprintf(out, "      %s %s\n", styled(color.Faint).Sprint("-"), item)
		}
		return
	}
	if values, ok := stringSlice(value); ok && len(values) == 1 {
		value = values[0]
	}
	fmt.Fprintf(out, "    %s %v\n", label, value)
}

func stringSlice(value any) ([]string, bool) {
	if value == nil {
		return nil, false
	}
	if typed, ok := value.([]string); ok {
		return typed, true
	}
	ref := reflect.ValueOf(value)
	if ref.Kind() != reflect.Slice && ref.Kind() != reflect.Array {
		return nil, false
	}
	values := make([]string, 0, ref.Len())
	for i := 0; i < ref.Len(); i++ {
		values = append(values, fmt.Sprint(ref.Index(i).Interface()))
	}
	return values, true
}

func jsonValue(value any) any {
	if err, ok := value.(error); ok {
		return err.Error()
	}
	return value
}

func symbol(kind Kind) string {
	switch kind {
	case KindAction:
		return spinnerFrames[0]
	case KindSuccess:
		return "✓"
	case KindWarning:
		return "!"
	case KindError:
		return "×"
	default:
		return "·"
	}
}

func symbolStyle(kind Kind) *color.Color {
	switch kind {
	case KindAction:
		return styled(color.FgHiCyan, color.Bold)
	case KindSuccess:
		return styled(color.FgHiGreen, color.Bold)
	case KindWarning:
		return styled(color.FgHiYellow, color.Bold)
	case KindError:
		return styled(color.FgHiRed, color.Bold)
	default:
		return styled(color.FgHiBlack, color.Bold)
	}
}

func levelCode(level Level) string {
	switch level {
	case Debug:
		return "DBG"
	case Warn:
		return "WRN"
	case Error:
		return "ERR"
	default:
		return "INF"
	}
}

func levelStyle(level string) *color.Color {
	switch level {
	case "DBG":
		return styled(color.FgHiBlack)
	case "WRN":
		return styled(color.FgHiYellow, color.Bold)
	case "ERR":
		return styled(color.FgHiRed, color.Bold)
	default:
		return styled(color.FgHiCyan, color.Bold)
	}
}
