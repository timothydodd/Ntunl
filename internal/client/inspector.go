package client

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"time"

	"github.com/timothydodd/ntunl/internal/compress"
)

//go:embed templates/requests.html
var inspectorTemplate string

type inspectorRequest struct {
	Method  string
	Url     string
	Headers map[string]string
}

type inspectorResponse struct {
	StatusCode int
	Headers    map[string]string
	Content    string
}

type inspectorLog struct {
	Index    int
	Request  inspectorRequest
	Response inspectorResponse
}

type inspectorView struct {
	Logs []inspectorLog
}

// runInspector serves the request/response log page until ctx is cancelled.
func runInspector(ctx context.Context, log *slog.Logger, handler *messageHandler, port int) error {
	tmpl, err := template.New("requests").Parse(inspectorTemplate)
	if err != nil {
		return fmt.Errorf("parse template: %w", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.Execute(w, buildView(handler)); err != nil {
			log.Error("render inspector", "err", err)
		}
	})

	addr := fmt.Sprintf("localhost:%d", port)
	srv := &http.Server{Addr: addr, Handler: mux}

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()

	log.Info("Inspector listening", "url", "http://"+addr+"/")
	err = srv.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func buildView(handler *messageHandler) inspectorView {
	logs := handler.snapshot()
	view := inspectorView{Logs: make([]inspectorLog, 0, len(logs))}

	for i, l := range logs {
		var content string
		if enc, ok := l.Response.ContentHeaders["Content-Encoding"]; ok && enc == "br" {
			if dec, err := compress.BrotliDecompress(l.Response.Content); err == nil {
				content = string(dec)
			}
		} else {
			content = string(l.Response.Content)
		}

		headers := map[string]string{}
		for k, v := range l.Response.Headers {
			headers[k] = v
		}
		for k, v := range l.Response.ContentHeaders {
			if _, exists := headers[k]; !exists {
				headers[k] = v
			}
		}

		view.Logs = append(view.Logs, inspectorLog{
			Index: i,
			Request: inspectorRequest{
				Method:  l.Request.Method,
				Url:     l.Request.Path,
				Headers: l.Request.Headers,
			},
			Response: inspectorResponse{
				StatusCode: l.Response.StatusCode,
				Headers:    headers,
				Content:    content,
			},
		})
	}
	return view
}
