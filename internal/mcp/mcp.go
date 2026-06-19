package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/zionboggan/agent-time-ledger/internal/clock"
	"github.com/zionboggan/agent-time-ledger/internal/ledger"
)

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id,omitempty"`
	Result  any       `json:"result,omitempty"`
	Error   *rpcError `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type toolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

type callResult struct {
	Content []content `json:"content"`
	IsError bool      `json:"isError,omitempty"`
}

type content struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func Serve(stdin io.Reader, stdout, stderr io.Writer, service *ledger.Service) error {
	reader := bufio.NewReader(stdin)
	for {
		body, err := readMessage(reader)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			fmt.Fprintln(stderr, err)
			return err
		}
		if len(bytes.TrimSpace(body)) == 0 {
			continue
		}

		var req request
		if err := json.Unmarshal(body, &req); err != nil {
			_ = writeMessage(stdout, response{
				JSONRPC: "2.0",
				Error:   &rpcError{Code: -32700, Message: "parse error"},
			})
			continue
		}
		if strings.HasPrefix(req.Method, "notifications/") {
			continue
		}
		resp := handle(req, service)
		if err := writeMessage(stdout, resp); err != nil {
			return err
		}
	}
}

func handle(req request, service *ledger.Service) response {
	resp := response{JSONRPC: "2.0", ID: req.ID}
	switch req.Method {
	case "initialize":
		resp.Result = map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]any{
				"tools": map[string]any{},
			},
			"serverInfo": map[string]any{
				"name":    "agent-time-ledger",
				"version": "0.1.0",
			},
		}
	case "tools/list":
		resp.Result = map[string]any{"tools": tools()}
	case "tools/call":
		result, err := handleToolCall(req.Params, service)
		if err != nil {
			resp.Result = callResult{
				Content: []content{{Type: "text", Text: err.Error()}},
				IsError: true,
			}
		} else {
			resp.Result = result
		}
	default:
		resp.Error = &rpcError{Code: -32601, Message: "method not found"}
	}
	return resp
}

func handleToolCall(raw json.RawMessage, service *ledger.Service) (callResult, error) {
	var params toolCallParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return callResult{}, fmt.Errorf("invalid tool call params: %w", err)
	}
	args := map[string]any{}
	if len(params.Arguments) > 0 {
		if err := json.Unmarshal(params.Arguments, &args); err != nil {
			return callResult{}, fmt.Errorf("invalid tool arguments: %w", err)
		}
	}

	var result any
	var err error
	switch params.Name {
	case "time_now":
		result = service.NowResponse()
	case "session_status":
		result, err = service.SessionStatus()
	case "mark_start":
		result, err = service.StartMark(requiredString(args, "name"))
	case "mark_elapsed":
		result, err = service.MarkElapsed(requiredString(args, "name"))
	case "stale_check":
		result, err = staleCheck(args, service)
	case "ledger_event":
		note, _ := args["note"].(string)
		err = service.LedgerEvent(note)
		result = map[string]any{"ok": err == nil, "confidence": clock.ConfidenceWallFallback}
	case "ledger_report":
		result, err = service.ReportToday()
	default:
		return callResult{}, fmt.Errorf("unknown tool %q", params.Name)
	}
	if err != nil {
		return callResult{}, err
	}
	text, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return callResult{}, err
	}
	return callResult{Content: []content{{Type: "text", Text: string(text)}}}, nil
}

func staleCheck(args map[string]any, service *ledger.Service) (ledger.StaleResponse, error) {
	ttlText := requiredString(args, "ttl")
	ttl, err := clock.ParseDuration(ttlText)
	if err != nil {
		return ledger.StaleResponse{}, err
	}
	if markName, ok := args["mark"].(string); ok && markName != "" {
		return service.StaleFromMark(markName, ttl)
	}
	timestampText := requiredString(args, "timestamp")
	timestamp, err := clock.ParseRFC3339(timestampText)
	if err != nil {
		return ledger.StaleResponse{}, err
	}
	return service.StaleFromTimestamp(timestamp, ttl)
}

func requiredString(args map[string]any, name string) string {
	value, _ := args[name].(string)
	return value
}

func tools() []map[string]any {
	return []map[string]any{
		tool("time_now", "Return the host clock time.", map[string]any{}),
		tool("session_status", "Return active session status and elapsed time.", map[string]any{}),
		tool("mark_start", "Start or reset a named mark.", map[string]any{
			"name": stringSchema("Mark name"),
		}, "name"),
		tool("mark_elapsed", "Return elapsed time for a named mark.", map[string]any{
			"name": stringSchema("Mark name"),
		}, "name"),
		tool("stale_check", "Check whether a timestamp or mark is stale for a TTL.", map[string]any{
			"timestamp": stringSchema("RFC3339 timestamp"),
			"mark":      stringSchema("Existing mark name"),
			"ttl":       stringSchema("Duration such as 15s, 30m, 2h, or 1d"),
		}, "ttl"),
		tool("ledger_event", "Append a local manual_note ledger event.", map[string]any{
			"note": stringSchema("Short local note"),
		}),
		tool("ledger_report", "Return today's local ledger report.", map[string]any{}),
	}
}

func tool(name, description string, properties map[string]any, required ...string) map[string]any {
	if required == nil {
		required = []string{}
	}
	return map[string]any{
		"name":        name,
		"description": description,
		"inputSchema": map[string]any{
			"type":                 "object",
			"properties":           properties,
			"required":             required,
			"additionalProperties": false,
		},
	}
}

func stringSchema(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func readMessage(reader *bufio.Reader) ([]byte, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		if err == io.EOF && strings.TrimSpace(line) != "" {
			return []byte(line), nil
		}
		return nil, err
	}
	line = strings.TrimRight(line, "\r\n")
	if strings.TrimSpace(line) == "" {
		return []byte{}, nil
	}
	if !strings.HasPrefix(strings.ToLower(line), "content-length:") {
		return []byte(line), nil
	}

	lengthText := strings.TrimSpace(strings.TrimPrefix(strings.ToLower(line), "content-length:"))
	length, err := strconv.Atoi(lengthText)
	if err != nil {
		return nil, fmt.Errorf("invalid Content-Length: %w", err)
	}
	for {
		header, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(header) == "" {
			break
		}
	}
	body := make([]byte, length)
	_, err = io.ReadFull(reader, body)
	return body, err
}

func writeMessage(w io.Writer, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "Content-Length: %d\r\n\r\n%s", len(data), data)
	return err
}
