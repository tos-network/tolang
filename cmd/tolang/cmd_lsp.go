package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"

	"github.com/tos-network/tolang/tol/diag"
	"github.com/tos-network/tolang/tol/parser"
	"github.com/tos-network/tolang/tol/sema"
)

func cmdLSP(args []string) int {
	server := &lspServer{
		files: make(map[string]string),
	}
	return server.run()
}

type lspServer struct {
	mu    sync.Mutex
	files map[string]string // uri → content
	w     io.Writer         // stdout, guarded by wmu
	wmu   sync.Mutex
}

// jsonRPCRequest is a minimal JSON-RPC 2.0 request/notification envelope.
type jsonRPCRequest struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method"`
	Params  json.RawMessage  `json:"params,omitempty"`
}

func (s *lspServer) run() int {
	s.w = os.Stdout
	reader := bufio.NewReader(os.Stdin)
	for {
		body, err := readMessage(reader)
		if err != nil {
			if err == io.EOF {
				return 0
			}
			s.logf("read error: %v", err)
			return 1
		}
		s.handleMessage(body)
	}
}

func readMessage(r *bufio.Reader) ([]byte, error) {
	contentLength := -1
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if strings.HasPrefix(line, "Content-Length:") {
			val := strings.TrimSpace(strings.TrimPrefix(line, "Content-Length:"))
			n, err := strconv.Atoi(val)
			if err != nil {
				return nil, fmt.Errorf("bad Content-Length: %w", err)
			}
			contentLength = n
		}
		// Ignore other headers (e.g. Content-Type).
	}
	if contentLength < 0 {
		return nil, fmt.Errorf("missing Content-Length header")
	}
	body := make([]byte, contentLength)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}
	return body, nil
}

func (s *lspServer) handleMessage(body []byte) {
	defer func() {
		if r := recover(); r != nil {
			s.logf("panic handling message: %v\n%s", r, debug.Stack())
		}
	}()

	var req jsonRPCRequest
	if err := json.Unmarshal(body, &req); err != nil {
		s.logf("invalid JSON-RPC message: %v", err)
		return
	}

	switch req.Method {
	case "initialize":
		s.handleInitialize(req)
	case "initialized":
		// No-op.
	case "shutdown":
		s.handleShutdown(req)
	case "exit":
		os.Exit(0)
	case "textDocument/didOpen":
		s.handleDidOpen(req)
	case "textDocument/didChange":
		s.handleDidChange(req)
	case "textDocument/didSave":
		s.handleDidSave(req)
	case "textDocument/didClose":
		s.handleDidClose(req)
	case "textDocument/hover":
		s.handleHover(req)
	default:
		// Unknown method — if it has an ID it's a request, send method-not-found.
		if req.ID != nil {
			s.respondError(req.ID, -32601, "method not found: "+req.Method)
		}
	}
}

func (s *lspServer) handleInitialize(req jsonRPCRequest) {
	result := map[string]interface{}{
		"capabilities": map[string]interface{}{
			"textDocumentSync": 1, // Full sync
			"hoverProvider":    true,
		},
		"serverInfo": map[string]interface{}{
			"name":    "tolang-lsp",
			"version": "0.1.0",
		},
	}
	s.respond(req.ID, result)
}

func (s *lspServer) handleShutdown(req jsonRPCRequest) {
	s.respond(req.ID, nil)
}

func (s *lspServer) handleDidOpen(req jsonRPCRequest) {
	var params struct {
		TextDocument struct {
			URI  string `json:"uri"`
			Text string `json:"text"`
		} `json:"textDocument"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.logf("didOpen parse error: %v", err)
		return
	}
	uri := params.TextDocument.URI
	content := params.TextDocument.Text
	s.mu.Lock()
	s.files[uri] = content
	s.mu.Unlock()
	s.publishDiagnostics(uri, content)
}

func (s *lspServer) handleDidChange(req jsonRPCRequest) {
	var params struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
		ContentChanges []struct {
			Text string `json:"text"`
		} `json:"contentChanges"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.logf("didChange parse error: %v", err)
		return
	}
	uri := params.TextDocument.URI
	if len(params.ContentChanges) == 0 {
		return
	}
	// Full sync mode: last change contains entire content.
	content := params.ContentChanges[len(params.ContentChanges)-1].Text
	s.mu.Lock()
	s.files[uri] = content
	s.mu.Unlock()
	s.publishDiagnostics(uri, content)
}

func (s *lspServer) handleDidSave(req jsonRPCRequest) {
	var params struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
		Text *string `json:"text,omitempty"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.logf("didSave parse error: %v", err)
		return
	}
	uri := params.TextDocument.URI
	s.mu.Lock()
	content, ok := s.files[uri]
	s.mu.Unlock()
	if params.Text != nil {
		content = *params.Text
		s.mu.Lock()
		s.files[uri] = content
		s.mu.Unlock()
		ok = true
	}
	if ok {
		s.publishDiagnostics(uri, content)
	}
}

func (s *lspServer) handleDidClose(req jsonRPCRequest) {
	var params struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.logf("didClose parse error: %v", err)
		return
	}
	uri := params.TextDocument.URI
	s.mu.Lock()
	delete(s.files, uri)
	s.mu.Unlock()
	// Clear diagnostics for the closed file.
	s.notify("textDocument/publishDiagnostics", map[string]interface{}{
		"uri":         uri,
		"diagnostics": []interface{}{},
	})
}

func (s *lspServer) handleHover(req jsonRPCRequest) {
	var params struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
		Position struct {
			Line      int `json:"line"`
			Character int `json:"character"`
		} `json:"position"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.respond(req.ID, nil)
		return
	}
	uri := params.TextDocument.URI
	s.mu.Lock()
	content, ok := s.files[uri]
	s.mu.Unlock()
	if !ok {
		s.respond(req.ID, nil)
		return
	}

	// Extract the word under cursor for basic hover info.
	word := wordAtPosition(content, params.Position.Line, params.Position.Character)
	if word == "" {
		s.respond(req.ID, nil)
		return
	}

	info := hoverInfo(word)
	if info == "" {
		s.respond(req.ID, nil)
		return
	}

	s.respond(req.ID, map[string]interface{}{
		"contents": map[string]interface{}{
			"kind":  "markdown",
			"value": info,
		},
	})
}

func (s *lspServer) publishDiagnostics(uri, content string) {
	filename := uriToPath(uri)
	var allDiags diag.Diagnostics

	// Parse the source.
	mod, parseDiags := parser.ParseFile(filename, []byte(content))
	allDiags = append(allDiags, parseDiags...)

	// Run semantic checks if parse produced a module and no parse errors.
	if mod != nil && !parseDiags.HasErrors() {
		_, semaDiags := sema.Check(filename, mod)
		allDiags = append(allDiags, semaDiags...)
	}

	// Convert to LSP diagnostics.
	lspDiags := make([]interface{}, 0, len(allDiags))
	for _, d := range allDiags {
		severity := 1 // Error
		if d.Severity == diag.SeverityWarning {
			severity = 2 // Warning
		}
		startLine := d.Span.Start.Line - 1
		if startLine < 0 {
			startLine = 0
		}
		startChar := d.Span.Start.Column - 1
		if startChar < 0 {
			startChar = 0
		}
		endLine := d.Span.End.Line - 1
		if endLine < startLine {
			endLine = startLine
		}
		endChar := d.Span.End.Column - 1
		if endChar < 0 {
			endChar = 1000 // extend to end of line
		}
		lspDiags = append(lspDiags, map[string]interface{}{
			"range": map[string]interface{}{
				"start": map[string]interface{}{"line": startLine, "character": startChar},
				"end":   map[string]interface{}{"line": endLine, "character": endChar},
			},
			"severity": severity,
			"source":   "tolang",
			"code":     d.Code,
			"message":  d.Message,
		})
	}

	s.notify("textDocument/publishDiagnostics", map[string]interface{}{
		"uri":         uri,
		"diagnostics": lspDiags,
	})
}

// respond sends a JSON-RPC response.
func (s *lspServer) respond(id *json.RawMessage, result interface{}) {
	msg := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	}
	s.writeMessage(msg)
}

// respondError sends a JSON-RPC error response.
func (s *lspServer) respondError(id *json.RawMessage, code int, message string) {
	msg := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]interface{}{
			"code":    code,
			"message": message,
		},
	}
	s.writeMessage(msg)
}

// notify sends a JSON-RPC notification (no id).
func (s *lspServer) notify(method string, params interface{}) {
	msg := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	}
	s.writeMessage(msg)
}

func (s *lspServer) writeMessage(msg interface{}) {
	data, err := json.Marshal(msg)
	if err != nil {
		s.logf("marshal error: %v", err)
		return
	}
	s.wmu.Lock()
	defer s.wmu.Unlock()
	fmt.Fprintf(s.w, "Content-Length: %d\r\n\r\n%s", len(data), data)
}

func (s *lspServer) logf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "[tolang-lsp] "+format+"\n", args...)
}

// uriToPath converts a file:// URI to a filesystem path.
func uriToPath(uri string) string {
	if strings.HasPrefix(uri, "file://") {
		u, err := url.Parse(uri)
		if err == nil {
			return u.Path
		}
	}
	return uri
}

// wordAtPosition extracts the word at the given 0-based line/character position.
func wordAtPosition(content string, line, character int) string {
	lines := strings.Split(content, "\n")
	if line < 0 || line >= len(lines) {
		return ""
	}
	l := lines[line]
	if character < 0 || character >= len(l) {
		return ""
	}
	// Find word boundaries.
	start := character
	for start > 0 && isIdentChar(l[start-1]) {
		start--
	}
	end := character
	for end < len(l) && isIdentChar(l[end]) {
		end++
	}
	if start == end {
		return ""
	}
	return l[start:end]
}

func isIdentChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_'
}

// hoverInfo returns markdown hover information for known TOL keywords and types.
func hoverInfo(word string) string {
	switch word {
	// Types
	case "u256":
		return "**u256** — 256-bit unsigned integer type"
	case "i256":
		return "**i256** — 256-bit signed integer type"
	case "u128":
		return "**u128** — 128-bit unsigned integer type"
	case "i128":
		return "**i128** — 128-bit signed integer type"
	case "u64":
		return "**u64** — 64-bit unsigned integer type"
	case "i64":
		return "**i64** — 64-bit signed integer type"
	case "u32":
		return "**u32** — 32-bit unsigned integer type"
	case "i32":
		return "**i32** — 32-bit signed integer type"
	case "u16":
		return "**u16** — 16-bit unsigned integer type"
	case "i16":
		return "**i16** — 16-bit signed integer type"
	case "u8":
		return "**u8** — 8-bit unsigned integer type"
	case "i8":
		return "**i8** — 8-bit signed integer type"
	case "bool":
		return "**bool** — boolean type (true/false)"
	case "address":
		return "**address** — 20-byte account address"
	case "bytes32":
		return "**bytes32** — fixed 32-byte value"
	case "string":
		return "**string** — dynamic-length UTF-8 string"
	case "bytes":
		return "**bytes** — dynamic-length byte array"
	case "uno":
		return "**uno** — encrypted value type (Twisted ElGamal)"

	// Keywords
	case "contract":
		return "**contract** — declares a TOL smart contract"
	case "interface":
		return "**interface** — declares a TOL interface"
	case "function":
		return "**function** — declares a function"
	case "event":
		return "**event** — declares an event that can be emitted"
	case "modifier":
		return "**modifier** — declares a function modifier"
	case "mapping":
		return "**mapping** — key-value storage mapping type"
	case "storage":
		return "**storage** — declares a persistent storage slot"
	case "constructor":
		return "**constructor** — contract constructor, runs once at deployment"
	case "emit":
		return "**emit** — emit an event"
	case "revert":
		return "**revert** — revert the current transaction"
	case "require":
		return "**require** — assert a condition, revert if false"
	case "public":
		return "**public** — function is externally callable"
	case "external":
		return "**external** — function is callable only from outside"
	case "internal":
		return "**internal** — function is callable only from this contract and derived contracts"
	case "view":
		return "**view** — function reads but does not modify state"
	case "pure":
		return "**pure** — function does not read or modify state"
	case "payable":
		return "**payable** — function can receive TOS value"
	case "immutable":
		return "**immutable** — variable set once in constructor, cannot be changed"
	case "constant":
		return "**constant** — compile-time constant value"
	case "abstract":
		return "**abstract** — contract that cannot be deployed directly"
	case "struct":
		return "**struct** — user-defined composite value type"
	case "import":
		return "**import** — import declarations from another file"
	default:
		return ""
	}
}
