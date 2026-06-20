// Package protocol defines the NTunl wire protocol exchanged over the
// host<->client WebSocket connection. Each WebSocket message carries exactly one
// JSON-encoded Command, so unlike the original .NET implementation there is no
// hand-rolled binary framing.
package protocol

import "encoding/json"

// CommandType identifies the kind of message carried by a Command.
type CommandType int

const (
	CmdEcho         CommandType = 1
	CmdHttpRequest  CommandType = 2
	CmdHttpResponse CommandType = 3
	CmdNtunlInfo    CommandType = 4
)

// Command is the envelope for every message on the tunnel socket. Data holds a
// nested JSON payload whose shape depends on CommandType (HttpRequestData,
// HttpResponseData, NtunlInfo) or a plain string for Echo.
type Command struct {
	CommandType    CommandType `json:"commandType"`
	ConversationId string      `json:"conversationId"`
	Data           string      `json:"data"`
}

// Marshal encodes the command to its on-wire JSON bytes.
func (c *Command) Marshal() ([]byte, error) {
	return json.Marshal(c)
}

// Unmarshal decodes a command from on-wire JSON bytes.
func Unmarshal(data []byte) (*Command, error) {
	var c Command
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	return &c, nil
}
