// lsp_tool.go — SIBLING ext (deletable, NON-frozen): kecerdasan kode SEMANTIK lewat
// Language Server Protocol (gopls). "Level berikutnya" dari codemap statis: definition,
// references, hover (tipe/dok asli, lintas-file, presisi). Roadmap "buka lock".
//
// Plug-and-play: init() self-register (NOL sentuh builtins.go frozen). Klien LSP minimal
// (JSON-RPC stdio, Content-Length framing) → spawn gopls persisten per workspace-root,
// initialize sekali, reuse. Isolasi: root = workspace agent (FromSharedDir).
//
// KEAMANAN/DEP: default OFF (switch FLOWORK_LSP=1). NOL modul Go baru (gopls = binary
// eksternal, dideteksi; ga ada = error sopan). 📄 Dok: lock/lsp-code.md
package builtins

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go/scanner"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"flowork-gui/internal/tools"
)

func lspEnabled() bool {
	v := strings.TrimSpace(os.Getenv("FLOWORK_LSP"))
	return v == "1" || strings.EqualFold(v, "true")
}

func init() {
	if lspEnabled() {
		tools.Register(&lspTool{})
	}
}

// goplsPath — cari binary gopls (switch FLOWORK_GOPLS_BIN > PATH > GOBIN/GOPATH/bin).
func goplsPath() string {
	if p := strings.TrimSpace(os.Getenv("FLOWORK_GOPLS_BIN")); p != "" {
		return p
	}
	if p, err := exec.LookPath("gopls"); err == nil {
		return p
	}
	for _, base := range []string{os.Getenv("GOBIN"), filepath.Join(os.Getenv("GOPATH"), "bin"), filepath.Join(os.Getenv("HOME"), "go", "bin")} {
		if base == "" {
			continue
		}
		cand := filepath.Join(base, "gopls")
		if st, err := os.Stat(cand); err == nil && !st.IsDir() {
			return cand
		}
	}
	return ""
}

// =============================================================================
// Klien LSP minimal (JSON-RPC 2.0 over stdio, LSP Content-Length framing)
// =============================================================================

type lspClient struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	mu      sync.Mutex
	seq     int64
	pending map[int64]chan json.RawMessage
	root    string
	opened  map[string]bool // uri yg udah didOpen
	dead    bool
}

var (
	lspMu      sync.Mutex
	lspClients = map[string]*lspClient{} // root → client (reuse gopls per workspace)
)

func lspFileURI(path string) string { return "file://" + path }

// getLSPClient — reuse gopls yg udah jalan buat root ini, atau spawn+initialize baru.
func getLSPClient(root string) (*lspClient, error) {
	lspMu.Lock()
	if c, ok := lspClients[root]; ok && !c.dead {
		lspMu.Unlock()
		return c, nil
	}
	lspMu.Unlock()

	bin := goplsPath()
	if bin == "" {
		return nil, fmt.Errorf("gopls ga ketemu (install: `go install golang.org/x/tools/gopls@latest`, atau set FLOWORK_GOPLS_BIN)")
	}
	cmd := exec.Command(bin, "serve")
	cmd.Dir = root
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start gopls: %w", err)
	}
	c := &lspClient{cmd: cmd, stdin: stdin, pending: map[int64]chan json.RawMessage{}, root: root, opened: map[string]bool{}}
	go c.readLoop(stdout)

	if err := c.initialize(); err != nil {
		c.shutdown()
		return nil, fmt.Errorf("initialize gopls: %w", err)
	}
	lspMu.Lock()
	lspClients[root] = c
	lspMu.Unlock()
	return c, nil
}

// readLoop — parse pesan berframe Content-Length, salurin response ke pending id.
func (c *lspClient) readLoop(r io.Reader) {
	br := bufio.NewReader(r)
	for {
		length := 0
		for {
			line, err := br.ReadString('\n')
			if err != nil {
				c.markDead()
				return
			}
			line = strings.TrimRight(line, "\r\n")
			if line == "" {
				break // akhir header
			}
			if n, ok := strings.CutPrefix(line, "Content-Length:"); ok {
				length, _ = strconv.Atoi(strings.TrimSpace(n))
			}
		}
		if length <= 0 {
			continue
		}
		body := make([]byte, length)
		if _, err := io.ReadFull(br, body); err != nil {
			c.markDead()
			return
		}
		var msg struct {
			ID     *int64          `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  json.RawMessage `json:"error"`
		}
		if json.Unmarshal(body, &msg) != nil || msg.ID == nil {
			continue // notifikasi (mis. publishDiagnostics) → skip
		}
		c.mu.Lock()
		ch, ok := c.pending[*msg.ID]
		delete(c.pending, *msg.ID)
		c.mu.Unlock()
		if ok {
			if len(msg.Error) > 0 {
				ch <- json.RawMessage(`{"__lsp_error":` + string(msg.Error) + `}`)
			} else {
				ch <- msg.Result
			}
		}
	}
}

func (c *lspClient) markDead() {
	c.mu.Lock()
	c.dead = true
	for id, ch := range c.pending {
		close(ch)
		delete(c.pending, id)
	}
	c.mu.Unlock()
}

func (c *lspClient) writeMsg(payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.dead {
		return fmt.Errorf("gopls mati")
	}
	_, err = fmt.Fprintf(c.stdin, "Content-Length: %d\r\n\r\n%s", len(b), b)
	return err
}

func (c *lspClient) notify(method string, params any) error {
	return c.writeMsg(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

// call — request + tunggu response (timeout).
func (c *lspClient) call(method string, params any, timeout time.Duration) (json.RawMessage, error) {
	c.mu.Lock()
	c.seq++
	id := c.seq
	ch := make(chan json.RawMessage, 1)
	c.pending[id] = ch
	c.mu.Unlock()

	if err := c.writeMsg(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
		return nil, err
	}
	select {
	case res, ok := <-ch:
		if !ok {
			return nil, fmt.Errorf("gopls mati sebelum balas")
		}
		if bytes.HasPrefix(res, []byte(`{"__lsp_error":`)) {
			return nil, fmt.Errorf("gopls error: %s", strings.TrimSuffix(strings.TrimPrefix(string(res), `{"__lsp_error":`), `}`))
		}
		return res, nil
	case <-time.After(timeout):
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("gopls timeout (%s) di %s", timeout, method)
	}
}

func (c *lspClient) initialize() error {
	params := map[string]any{
		"processId":    os.Getpid(),
		"rootUri":      lspFileURI(c.root),
		"capabilities": map[string]any{},
	}
	if _, err := c.call("initialize", params, 20*time.Second); err != nil {
		return err
	}
	return c.notify("initialized", map[string]any{})
}

func (c *lspClient) ensureOpen(path string) error {
	uri := lspFileURI(path)
	c.mu.Lock()
	already := c.opened[uri]
	c.mu.Unlock()
	if already {
		return nil
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := c.notify("textDocument/didOpen", map[string]any{
		"textDocument": map[string]any{"uri": uri, "languageId": "go", "version": 1, "text": string(src)},
	}); err != nil {
		return err
	}
	c.mu.Lock()
	c.opened[uri] = true
	c.mu.Unlock()
	time.Sleep(300 * time.Millisecond) // kasih gopls waktu index awal
	return nil
}

func (c *lspClient) shutdown() {
	_, _ = c.call("shutdown", nil, 3*time.Second)
	_ = c.notify("exit", nil)
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	c.markDead()
}

// =============================================================================
// Tool lsp
// =============================================================================

type lspTool struct{}

func (lspTool) Name() string       { return "lsp" }
func (lspTool) Capability() string { return "code:analyze" }
func (lspTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Semantic code intelligence via a language server (gopls, Go). Beyond the static " +
			"codemap: precise go-to-definition, find-references, and hover (real types/docs, cross-file). " +
			"Point at a symbol by name in a file — the tool locates it and queries gopls.",
		Params: []tools.Param{
			{Name: "file_path", Type: tools.ParamString, Description: "relative .go file in your workspace, e.g. 'internal/x/y.go'", Required: true},
			{Name: "symbol", Type: tools.ParamString, Description: "identifier to inspect (e.g. 'DetectAll'). First occurrence in the file is used unless occurrence set.", Required: true},
			{Name: "operation", Type: tools.ParamString, Description: "definition | references | hover (default definition)"},
			{Name: "occurrence", Type: tools.ParamInt, Description: "which occurrence of symbol in the file (1-based, default 1)"},
		},
		Returns: "{operation, symbol, results:[{file, line, snippet}] | hover text}",
	}
}

func (lspTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	// Resolusi path workspace-confined (reuse resolver file tools).
	abs, rel, err := resolveWorkspaceRel(ctx, nbArgString(args, "file_path"))
	if err != nil {
		return tools.Result{}, err
	}
	root := deriveGoRoot(abs)
	if !strings.HasSuffix(strings.ToLower(abs), ".go") {
		return tools.Result{}, fmt.Errorf("lsp: file_path harus .go (dapet %q)", rel)
	}
	symbol := strings.TrimSpace(nbArgString(args, "symbol"))
	if symbol == "" {
		return tools.Result{}, fmt.Errorf("lsp: symbol wajib")
	}
	occ := 1
	if v, ok := argInt(args, "occurrence"); ok && v > 0 {
		occ = int(v)
	}
	op := strings.ToLower(strings.TrimSpace(nbArgString(args, "operation")))
	if op == "" {
		op = "definition"
	}

	line, char, snip, ferr := findSymbolPos(abs, symbol, occ)
	if ferr != nil {
		return tools.Result{}, ferr
	}

	c, err := getLSPClient(root)
	if err != nil {
		return tools.Result{}, err
	}
	if err := c.ensureOpen(abs); err != nil {
		return tools.Result{}, fmt.Errorf("lsp didOpen: %w", err)
	}
	pos := map[string]any{
		"textDocument": map[string]any{"uri": lspFileURI(abs)},
		"position":     map[string]any{"line": line, "character": char},
	}

	switch op {
	case "definition", "references":
		method := "textDocument/definition"
		if op == "references" {
			method = "textDocument/references"
			pos["context"] = map[string]any{"includeDeclaration": true}
		}
		raw, err := c.call(method, pos, 15*time.Second)
		if err != nil {
			return tools.Result{}, err
		}
		locs := parseLocations(raw)
		return tools.Result{
			Output: map[string]any{"operation": op, "symbol": symbol, "at": snip, "results": locs, "count": len(locs)},
			Note:   fmt.Sprintf("lsp %s %q → %d hasil", op, symbol, len(locs)),
		}, nil
	case "hover":
		raw, err := c.call("textDocument/hover", pos, 15*time.Second)
		if err != nil {
			return tools.Result{}, err
		}
		return tools.Result{
			Output: map[string]any{"operation": "hover", "symbol": symbol, "hover": parseHover(raw)},
		}, nil
	default:
		return tools.Result{}, fmt.Errorf("lsp: operation harus definition|references|hover (dapet %q)", op)
	}
}

// findSymbolPos — cari occurrence ke-N IDENTIFIER 'symbol' di file → (line, char)
// 0-based + snippet. Pakai go/scanner biar cuma match token IDENT beneran (skip
// komentar & string literal otomatis — anti "no identifier found" dari gopls).
// char = kolom byte-1 (≈ UTF-16 buat identifier ASCII, cukup buat kode).
func findSymbolPos(abs, symbol string, occ int) (int, int, string, error) {
	src, err := os.ReadFile(abs)
	if err != nil {
		return 0, 0, "", fmt.Errorf("lsp: baca file: %w", err)
	}
	fset := token.NewFileSet()
	f := fset.AddFile(abs, fset.Base(), len(src))
	var sc scanner.Scanner
	sc.Init(f, src, func(token.Position, string) {}, 0) // mode 0 = ga emit komentar
	lines := strings.Split(string(src), "\n")
	seen := 0
	for {
		pos, tok, lit := sc.Scan()
		if tok == token.EOF {
			break
		}
		if tok == token.IDENT && lit == symbol {
			seen++
			if seen == occ {
				p := fset.Position(pos)
				snip := ""
				if p.Line-1 >= 0 && p.Line-1 < len(lines) {
					snip = strings.TrimSpace(lines[p.Line-1])
				}
				return p.Line - 1, p.Column - 1, snip, nil // 0-based buat LSP
			}
		}
	}
	return 0, 0, "", fmt.Errorf("lsp: identifier %q occurrence %d ga ketemu di file", symbol, occ)
}

// deriveGoRoot — root modul = dir terdekat ke atas yang ada go.mod (biar gopls dapet
// modul beneran). Fallback: dir file. Dibatasi ga keluar workspace (abs udah confined).
func deriveGoRoot(abs string) string {
	dir := filepath.Dir(abs)
	for i := 0; i < 40; i++ {
		if st, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil && !st.IsDir() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return filepath.Dir(abs)
}

func parseLocations(raw json.RawMessage) []map[string]any {
	// gopls balik Location | []Location.
	type lspPos struct {
		Line      int `json:"line"`
		Character int `json:"character"`
	}
	type lspLoc struct {
		URI   string `json:"uri"`
		Range struct {
			Start lspPos `json:"start"`
		} `json:"range"`
	}
	var many []lspLoc
	if json.Unmarshal(raw, &many) != nil {
		var one lspLoc
		if json.Unmarshal(raw, &one) == nil && one.URI != "" {
			many = []lspLoc{one}
		}
	}
	out := make([]map[string]any, 0, len(many))
	for _, l := range many {
		path := strings.TrimPrefix(l.URI, "file://")
		out = append(out, map[string]any{
			"file":    path,
			"line":    l.Range.Start.Line + 1, // 1-based buat manusia
			"snippet": lineSnippet(path, l.Range.Start.Line),
		})
	}
	return out
}

func lineSnippet(path string, line0 int) string {
	src, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.Split(string(src), "\n")
	if line0 >= 0 && line0 < len(lines) {
		return strings.TrimSpace(lines[line0])
	}
	return ""
}

func parseHover(raw json.RawMessage) string {
	var h struct {
		Contents struct {
			Value string `json:"value"`
		} `json:"contents"`
	}
	if json.Unmarshal(raw, &h) == nil && h.Contents.Value != "" {
		return h.Contents.Value
	}
	// contents bisa string polos.
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return ""
}
