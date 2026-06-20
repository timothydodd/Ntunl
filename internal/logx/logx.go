// Package logx provides a colored console slog.Handler matching the format of the
// original .NET LogFormatter: "HH:mm:ss  level: message" with ANSI colors.
package logx

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
)

const (
	colorReset  = "[0m"
	colorRed    = "[31m"
	colorGreen  = "[32m"
	colorYellow = "[33m"
	colorCyan   = "[36m"
)

type handler struct {
	mu    *sync.Mutex
	w     io.Writer
	level slog.Level
	attrs []slog.Attr
}

// NewHandler returns a slog.Handler writing colored single-line output to w.
func NewHandler(w io.Writer, level slog.Level) slog.Handler {
	return &handler{mu: &sync.Mutex{}, w: w, level: level}
}

// New is a convenience that returns a *slog.Logger using NewHandler.
func New(w io.Writer, level slog.Level) *slog.Logger {
	return slog.New(NewHandler(w, level))
}

func (h *handler) Enabled(_ context.Context, l slog.Level) bool { return l >= h.level }

func (h *handler) Handle(_ context.Context, r slog.Record) error {
	label, color := levelLabel(r.Level)

	var sb strings.Builder
	sb.WriteString(r.Time.Format("15:04:05 "))
	if color != "" {
		sb.WriteString(color + label + colorReset)
	} else {
		sb.WriteString(label)
	}
	sb.WriteString(r.Message)

	writeAttr := func(a slog.Attr) bool {
		sb.WriteString(" ")
		sb.WriteString(a.Key)
		sb.WriteString("=")
		sb.WriteString(a.Value.String())
		return true
	}
	for _, a := range h.attrs {
		writeAttr(a)
	}
	r.Attrs(func(a slog.Attr) bool { return writeAttr(a) })
	sb.WriteString("\n")

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := io.WriteString(h.w, sb.String())
	return err
}

func (h *handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	nh := *h
	nh.attrs = append(append([]slog.Attr{}, h.attrs...), attrs...)
	return &nh
}

func (h *handler) WithGroup(string) slog.Handler { return h }

func levelLabel(l slog.Level) (label, color string) {
	switch {
	case l <= slog.LevelDebug:
		return "debug: ", colorCyan
	case l < slog.LevelWarn:
		return "info: ", colorGreen
	case l < slog.LevelError:
		return "warn: ", colorYellow
	default:
		return "error: ", colorRed
	}
}

// ParseLevel maps the appsettings-style names to slog levels.
func ParseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "trace", "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error", "critical":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// PrintLogo writes the NTunl ASCII banner (Extensions.DisplayNtunlLogo).
func PrintLogo(w io.Writer) {
	const logo = ` _______   __               .__
 \      \_/  |_ __ __  ____ |  |
 /   |   \   __\  |  \/    \|  |
/    |    \  | |  |  /   |  \  |__
\____|__  /__| |____/|___|  /____/
        \/                \/
`
	fmt.Fprint(w, logo)
}
