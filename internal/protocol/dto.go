package protocol

// HttpRequestData is the forwarded request the host sends to the client. Content
// is JSON-encoded as base64 (Go's default for []byte), matching the byte[]
// behavior of the original implementation.
type HttpRequestData struct {
	Headers        map[string]string `json:"headers"`
	Method         string            `json:"method"`
	Path           string            `json:"path"`
	Content        []byte            `json:"content,omitempty"`
	ContentHeaders map[string]string `json:"contentHeaders"`
}

// HttpResponseData is the client's reply sent back to the host. StatusCode is the
// numeric HTTP status (the .NET version serialized the HttpStatusCode enum, which
// also produced the integer value).
type HttpResponseData struct {
	StatusCode     int               `json:"statusCode"`
	Content        []byte            `json:"content,omitempty"`
	ContentHeaders map[string]string `json:"contentHeaders"`
	Headers        map[string]string `json:"headers"`
}

// NtunlInfo is pushed to a client on connect with its public URL.
type NtunlInfo struct {
	Url string `json:"url"`
}
