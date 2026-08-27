package logger

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/fatih/color"
)

type Level uint8

const (
	Debug Level = iota
	Info
	Warn
	Error
)

type Logger struct {
	level     Level
	out       io.Writer
	timestamp bool
}

func New(level Level) *Logger { return &Logger{level: level, out: color.Output, timestamp: true} }
func NewCLI() *Logger         { return &Logger{level: Info, out: color.Output} }
func NewWithWriter(level Level, writer io.Writer) *Logger {
	return &Logger{level: level, out: writer, timestamp: true}
}
func NewCLIWithWriter(writer io.Writer) *Logger { return &Logger{level: Info, out: writer} }

func (l *Logger) Debug(component, message string, fields ...any) {
	l.log(Debug, "DBG", component, message, fields...)
}
func (l *Logger) Info(component, message string, fields ...any) {
	l.log(Info, "INF", component, message, fields...)
}
func (l *Logger) Warn(component, message string, fields ...any) {
	l.log(Warn, "WRN", component, message, fields...)
}
func (l *Logger) Error(component, message string, fields ...any) {
	l.log(Error, "ERR", component, message, fields...)
}
func (l *Logger) Success(component, message string, fields ...any) {
	if Info < l.level {
		return
	}
	l.write("OK", component, message, fields...)
}
func (l *Logger) Detail(label string, value any) {
	labelText := styled(color.FgHiBlue, color.Bold).Sprintf("%-8s", strings.ToUpper(label))
	fmt.Fprintf(l.out, "    %s %v\n", labelText, value)
}

func (l *Logger) log(level Level, levelText, component, message string, fields ...any) {
	if level < l.level {
		return
	}
	l.write(levelText, component, message, fields...)
}

func (l *Logger) write(levelText, component, message string, fields ...any) {
	if l.timestamp {
		ts := styled(color.FgHiBlack).Sprint(time.Now().Format("15:04:05"))
		fmt.Fprint(l.out, ts, " ")
	}
	fmt.Fprint(l.out, levelStyle(levelText).Sprintf("%-3s", levelText), " ")
	fmt.Fprint(l.out, styled(color.FgHiBlue, color.Bold).Sprintf("%-8s", strings.ToUpper(component)))
	fmt.Fprint(l.out, message)
	for i := 0; i+1 < len(fields); i += 2 {
		fmt.Fprint(l.out, " ", styled(color.FgHiBlack).Sprintf("%v=", fields[i]), fields[i+1])
	}
	fmt.Fprintln(l.out)
}

func levelStyle(level string) *color.Color {
	switch level {
	case "OK":
		return styled(color.FgHiGreen, color.Bold)
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

func styled(attrs ...color.Attribute) *color.Color {
	c := color.New(attrs...)
	if os.Getenv("NO_COLOR") != "" {
		c.DisableColor()
	}
	return c
}
