package protocol

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestCommandRoundTrip(t *testing.T) {
	orig := &Command{
		CommandType:    CmdHttpRequest,
		ConversationId: "11111111-2222-3333-4444-555555555555",
		Data:           `{"method":"GET","path":"/foo"}`,
	}

	raw, err := orig.Marshal()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	got, err := Unmarshal(raw)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.CommandType != orig.CommandType || got.ConversationId != orig.ConversationId || got.Data != orig.Data {
		t.Fatalf("round trip mismatch: %+v != %+v", got, orig)
	}
}

func TestHttpRequestDataBinaryBody(t *testing.T) {
	body := []byte{0x00, 0x01, 0xfe, 0xff}
	req := HttpRequestData{Method: "POST", Path: "/", Content: body}

	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got HttpRequestData
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !bytes.Equal(got.Content, body) {
		t.Fatalf("binary body not preserved: %v != %v", got.Content, body)
	}
}
