// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package agentmgr

import (
	"archive/zip"
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"flowork-gui/internal/agentdb"
	"flowork-gui/internal/httpx"
	"flowork-gui/internal/kernel/loader"
	"flowork-gui/internal/slashcmd"
	"flowork-gui/internal/tools"
	"flowork-gui/internal/toolsidecar"
)

var reID = regexp.MustCompile(`^[a-z][a-z0-9-]{2,31}$`)

var Reload func(agentID string) error

func agentFolder(id string) string {
	return filepath.Join(loader.AgentsDir(), id+".fwagent")
}

func UploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "parse form: " + err.Error()})
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "missing file field"})
		return
	}
	defer file.Close()

	lower := strings.ToLower(hdr.Filename)
	if !strings.HasSuffix(lower, ".zip") {
		httpx.WriteJSON(w, map[string]any{"error": "expected .fwagent.zip filename"})
		return
	}

	raw, err := io.ReadAll(io.LimitReader(file, 64<<20))
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "read upload: " + err.Error()})
		return
	}
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "not a valid zip: " + err.Error()})
		return
	}

	manifestEntry, rootPrefix, err := findManifest(zr)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	manifestBody, err := readZipEntry(manifestEntry)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "read manifest: " + err.Error()})
		return
	}
	var manifest struct {
		ID    string `json:"id"`
		Kind  string `json:"kind"`
		Entry string `json:"entry"`
	}
	if err := json.Unmarshal(manifestBody, &manifest); err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "manifest parse: " + err.Error()})
		return
	}
	if !reID.MatchString(manifest.ID) {
		httpx.WriteJSON(w, map[string]any{"error": "invalid manifest.id"})
		return
	}

	targetDir := agentFolder(manifest.ID)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "mkdir target: " + err.Error()})
		return
	}
	written := 0
	for _, f := range zr.File {
		rel := f.Name
		if rootPrefix != "" {
			if !strings.HasPrefix(rel, rootPrefix) {
				continue
			}
			rel = strings.TrimPrefix(rel, rootPrefix)
		}
		if rel == "" || strings.HasSuffix(rel, "/") {
			continue
		}
		dest := filepath.Join(targetDir, filepath.FromSlash(rel))
		clean, err := filepath.Rel(targetDir, dest)
		if err != nil || strings.HasPrefix(clean, "..") {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			httpx.WriteJSON(w, map[string]any{"error": "mkdir: " + err.Error()})
			return
		}
		if err := extractFile(f, dest); err != nil {
			httpx.WriteJSON(w, map[string]any{"error": "extract " + rel + ": " + err.Error()})
			return
		}
		written++
	}
	httpx.WriteJSON(w, map[string]any{
		"ok":            true,
		"agent_id":      manifest.ID,
		"kind":          manifest.Kind,
		"entry":         manifest.Entry,
		"files_written": written,
		"target_dir":    targetDir,
		"next":          "kernel hot-reload pickup otomatis",
	})
}

func DownloadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if !reID.MatchString(id) {
		httpx.WriteJSON(w, map[string]any{"error": "invalid id"})
		return
	}

	srcDir := ""
	if cwd, err := os.Getwd(); err == nil {
		cand := filepath.Join(cwd, "agents", id)
		if stat, err := os.Stat(cand); err == nil && stat.IsDir() {
			srcDir = cand
		}
	}
	if srcDir == "" {
		srcDir = agentFolder(id)
	}
	if _, err := os.Stat(srcDir); err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "agent not found: " + id})
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.fwagent.zip"`, id))
	w.Header().Set("Cache-Control", "no-store")

	zw := zip.NewWriter(w)
	defer zw.Close()

	_ = filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil || rel == "." {
			return nil
		}

		base := filepath.Base(rel)
		if base == ".git" || base == "node_modules" || base == ".reload-trigger" {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		zipPath := filepath.ToSlash(rel)
		if info.IsDir() {

			_, _ = zw.Create(zipPath + "/")
			return nil
		}
		f, err := zw.Create(zipPath)
		if err != nil {
			return nil
		}
		src, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer src.Close()
		_, _ = io.Copy(f, src)
		return nil
	})
}

func ToggleHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if !reID.MatchString(id) {
		httpx.WriteJSON(w, map[string]any{"error": "invalid id"})
		return
	}

	dir, ok := resolveAgentDir(id)
	if !ok {
		httpx.WriteJSON(w, map[string]any{"error": "agent not found"})
		return
	}
	disabledFlag := strings.TrimSpace(r.URL.Query().Get("disabled"))
	disabled := disabledFlag == "1" || strings.EqualFold(disabledFlag, "true")

	dbPath := agentdb.Resolve(id, dir)
	store, err := agentdb.Open(dbPath)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "open db: " + err.Error()})
		return
	}
	if err := store.SetDisabled(disabled); err != nil {
		store.Close()
		httpx.WriteJSON(w, map[string]any{"error": "set: " + err.Error()})
		return
	}
	store.Close()

	reloadErr := ""
	if Reload != nil {
		if err := Reload(id); err != nil {
			reloadErr = err.Error()
			log.Printf("toggle: reload %s: %v", id, err)
		}
	}
	httpx.WriteJSON(w, map[string]any{
		"ok":         true,
		"id":         id,
		"enabled":    !disabled,
		"reload_err": reloadErr,
	})
}

func DBResetHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if !reID.MatchString(id) {
		httpx.WriteJSON(w, map[string]any{"error": "invalid id"})
		return
	}
	dbPath := agentdb.Resolve(id, agentFolder(id))
	if _, err := os.Stat(dbPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			httpx.WriteJSON(w, map[string]any{"ok": true, "note": "db belum ada, ngga ada yang di-reset"})
			return
		}
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	if err := os.Remove(dbPath); err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "remove: " + err.Error()})
		return
	}

	if f, err := os.OpenFile(dbPath, os.O_CREATE|os.O_RDWR, 0o644); err == nil {
		_ = f.Close()
	}
	httpx.WriteJSON(w, map[string]any{"ok": true, "path": dbPath})
}

var WorkspaceRebuildIndex func(agentID string) (any, error)

func WorkspaceMetaHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if !reID.MatchString(id) {
		httpx.WriteJSON(w, map[string]any{"error": "invalid id"})
		return
	}
	dbPath := agentdb.Resolve(id, agentFolder(id))
	if _, err := os.Stat(dbPath); err != nil {
		if r.Method == http.MethodGet {
			httpx.WriteJSON(w, map[string]any{"items": []any{}, "note": "db belum ada"})
			return
		}
		httpx.WriteJSON(w, map[string]any{"error": "db belum ada — boot agent dulu"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		store, err := agentdb.Open(dbPath)
		if err != nil {
			httpx.WriteJSON(w, map[string]any{"error": "open db: " + err.Error()})
			return
		}
		defer store.Close()
		category := strings.TrimSpace(r.URL.Query().Get("category"))
		limit := 100
		if s := strings.TrimSpace(r.URL.Query().Get("limit")); s != "" {
			if n, perr := strconv.Atoi(s); perr == nil && n > 0 && n <= 500 {
				limit = n
			}
		}
		items, err := store.ListMeta(category, limit)
		if err != nil {
			httpx.WriteJSON(w, map[string]any{"error": "list: " + err.Error()})
			return
		}
		httpx.WriteJSON(w, map[string]any{"items": items, "count": len(items)})

	case http.MethodPost:
		action := strings.TrimSpace(r.URL.Query().Get("action"))
		if action != "rebuild" {
			httpx.WriteJSON(w, map[string]any{"error": "unsupported action (only ?action=rebuild)"})
			return
		}
		if WorkspaceRebuildIndex == nil {
			httpx.WriteJSON(w, map[string]any{"error": "rebuild index not wired"})
			return
		}
		report, err := WorkspaceRebuildIndex(id)
		if err != nil {
			httpx.WriteJSON(w, map[string]any{"error": err.Error()})
			return
		}
		httpx.WriteJSON(w, map[string]any{"ok": true, "report": report})

	default:
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
	}
}

func KarmaHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed (use GET)"})
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if !reID.MatchString(id) {
		httpx.WriteJSON(w, map[string]any{"error": "invalid id"})
		return
	}
	dbPath := agentdb.Resolve(id, agentFolder(id))
	if _, err := os.Stat(dbPath); err != nil {
		httpx.WriteJSON(w, map[string]any{"items": []any{}, "note": "db belum ada"})
		return
	}
	store, err := agentdb.Open(dbPath)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "open db: " + err.Error()})
		return
	}
	defer store.Close()

	key := strings.TrimSpace(r.URL.Query().Get("key"))
	if key != "" {
		k, err := store.GetKarma(key)
		if err != nil {
			httpx.WriteJSON(w, map[string]any{"error": "get: " + err.Error()})
			return
		}
		httpx.WriteJSON(w, k)
		return
	}
	items, err := store.ListKarma()
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "list: " + err.Error()})
		return
	}
	httpx.WriteJSON(w, map[string]any{"items": items, "count": len(items)})
}

func DeathLetterHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if !reID.MatchString(id) {
		httpx.WriteJSON(w, map[string]any{"error": "invalid id"})
		return
	}
	dbPath := agentdb.Resolve(id, agentFolder(id))
	if _, err := os.Stat(dbPath); err != nil {
		if r.Method == http.MethodGet {
			httpx.WriteJSON(w, map[string]any{"items": []any{}, "note": "db belum ada"})
			return
		}
		httpx.WriteJSON(w, map[string]any{"error": "db belum ada — boot agent dulu"})
		return
	}
	store, err := agentdb.Open(dbPath)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "open db: " + err.Error()})
		return
	}
	defer store.Close()

	switch r.Method {
	case http.MethodGet:
		recipient := strings.TrimSpace(r.URL.Query().Get("recipient"))
		sealedOnly := r.URL.Query().Get("sealed") == "1"
		limit := 50
		if s := strings.TrimSpace(r.URL.Query().Get("limit")); s != "" {
			if n, perr := strconv.Atoi(s); perr == nil && n > 0 && n <= 500 {
				limit = n
			}
		}
		items, err := store.ReadLetters(recipient, sealedOnly, limit)
		if err != nil {
			httpx.WriteJSON(w, map[string]any{"error": "read: " + err.Error()})
			return
		}
		httpx.WriteJSON(w, map[string]any{"items": items, "count": len(items)})

	case http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
		var body struct {
			LetterType string `json:"letter_type"`
			Recipient  string `json:"recipient"`
			Subject    string `json:"subject"`
			Body       string `json:"body"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteJSON(w, map[string]any{"error": "decode: " + err.Error()})
			return
		}
		newID, err := store.WriteLetter(body.LetterType, body.Recipient, body.Subject, body.Body)
		if err != nil {
			httpx.WriteJSON(w, map[string]any{"error": err.Error()})
			return
		}
		httpx.WriteJSON(w, map[string]any{"ok": true, "id": newID})

	case http.MethodPut:
		letterID, perr := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("letter_id")), 10, 64)
		if perr != nil || letterID <= 0 {
			httpx.WriteJSON(w, map[string]any{"error": "letter_id required"})
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
		var body struct {
			Subject string `json:"subject"`
			Body    string `json:"body"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteJSON(w, map[string]any{"error": "decode: " + err.Error()})
			return
		}
		if err := store.UpdateUnsealedLetter(letterID, body.Subject, body.Body); err != nil {
			httpx.WriteJSON(w, map[string]any{"error": err.Error()})
			return
		}
		httpx.WriteJSON(w, map[string]any{"ok": true, "id": letterID})

	case http.MethodPatch:
		action := strings.TrimSpace(r.URL.Query().Get("action"))
		if action != "seal" {
			httpx.WriteJSON(w, map[string]any{"error": "unsupported action (only ?action=seal)"})
			return
		}
		letterID, perr := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("letter_id")), 10, 64)
		if perr != nil || letterID <= 0 {
			httpx.WriteJSON(w, map[string]any{"error": "letter_id required"})
			return
		}
		if err := store.SealLetter(letterID); err != nil {
			httpx.WriteJSON(w, map[string]any{"error": err.Error()})
			return
		}
		httpx.WriteJSON(w, map[string]any{"ok": true, "sealed": letterID})

	default:
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
	}
}

func SlashRunHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed (use POST)"})
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if !reID.MatchString(id) {
		httpx.WriteJSON(w, map[string]any{"error": "invalid id"})
		return
	}
	dbPath := agentdb.Resolve(id, agentFolder(id))
	if _, err := os.Stat(dbPath); err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "db belum ada — boot agent dulu"})
		return
	}
	store, err := agentdb.Open(dbPath)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "open db: " + err.Error()})
		return
	}
	defer store.Close()

	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
	var body struct {
		Text   string `json:"text"`
		Caller string `json:"caller"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "decode: " + err.Error()})
		return
	}
	text := strings.TrimSpace(body.Text)
	if text == "" {
		httpx.WriteJSON(w, map[string]any{"error": "text required"})
		return
	}
	caller := strings.TrimSpace(body.Caller)
	if caller == "" {
		caller = "http-admin"
	}

	ctx := slashcmd.WithStore(r.Context(), store)
	ctx = slashcmd.WithCaller(ctx, caller)
	ctx = slashcmd.WithAgent(ctx, id)

	t0 := time.Now()
	result, cmdName, runErr := slashcmd.Dispatch(ctx, text)
	elapsedMs := time.Since(t0).Milliseconds()

	argsRaw := ""
	if idx := strings.IndexAny(text, " \t"); idx >= 0 {
		argsRaw = strings.TrimSpace(text[idx+1:])
	}
	errText := ""
	if runErr != nil {
		errText = runErr.Error()
	}
	if _, lerr := store.LogSlashInvocation(cmdName, argsRaw, caller, result.Text, errText, elapsedMs); lerr != nil {
		log.Printf("agentmgr: log slash invocation failed: %v", lerr)
	}

	if runErr != nil {
		httpx.WriteJSON(w, map[string]any{
			"error":       runErr.Error(),
			"command":     cmdName,
			"duration_ms": elapsedMs,
		})
		return
	}
	httpx.WriteJSON(w, map[string]any{
		"ok":          true,
		"command":     cmdName,
		"result":      result,
		"duration_ms": elapsedMs,
	})
}

func SlashRegistryHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed (use GET)"})
		return
	}
	items := slashcmd.ListSummaries()
	httpx.WriteJSON(w, map[string]any{
		"items":        items,
		"count":        len(items),
		"algo_version": slashcmd.AlgoVersion,
	})
}

func SlashInvocationsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed (use GET)"})
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if !reID.MatchString(id) {
		httpx.WriteJSON(w, map[string]any{"error": "invalid id"})
		return
	}
	dbPath := agentdb.Resolve(id, agentFolder(id))
	if _, err := os.Stat(dbPath); err != nil {
		httpx.WriteJSON(w, map[string]any{"items": []any{}, "note": "db belum ada"})
		return
	}
	store, err := agentdb.Open(dbPath)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "open db: " + err.Error()})
		return
	}
	defer store.Close()

	command := strings.TrimSpace(r.URL.Query().Get("command"))
	caller := strings.TrimSpace(r.URL.Query().Get("caller"))
	limit := 50
	if s := strings.TrimSpace(r.URL.Query().Get("limit")); s != "" {
		if n, perr := strconv.Atoi(s); perr == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	items, err := store.ListSlashInvocations(command, caller, limit)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "list: " + err.Error()})
		return
	}
	httpx.WriteJSON(w, map[string]any{"items": items, "count": len(items)})
}

func ToolRunHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed (use POST)"})
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))

	if secret := strings.TrimSpace(os.Getenv("FLOWORK_LOOPBACK_SECRET")); secret != "" &&
		subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Flowork-Secret")), []byte(secret)) == 1 {
		if caller := strings.TrimSpace(r.Header.Get("X-Flowork-Caller")); caller != "" {
			if id != "" && id != caller {
				httpx.WriteJSON(w, map[string]any{"error": "agent identity mismatch (caller-bound execution)"})
				return
			}
			id = caller
		}
	}
	if !reID.MatchString(id) {
		httpx.WriteJSON(w, map[string]any{"error": "invalid id"})
		return
	}
	dbPath := agentdb.Resolve(id, agentFolder(id))
	if _, err := os.Stat(dbPath); err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "db belum ada — boot agent dulu"})
		return
	}
	store, err := agentdb.Open(dbPath)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "open db: " + err.Error()})
		return
	}
	defer store.Close()

	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
	var body struct {
		ToolName string         `json:"tool_name"`
		Args     map[string]any `json:"args"`
		Caller   string         `json:"caller"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "decode: " + err.Error()})
		return
	}
	toolName := strings.TrimSpace(body.ToolName)
	if toolName == "" {
		httpx.WriteJSON(w, map[string]any{"error": "tool_name required"})
		return
	}
	caller := strings.TrimSpace(body.Caller)
	if caller == "" {
		caller = "http-admin"
	}

	t, ok := tools.Lookup(toolName)
	if !ok {

		httpx.WriteJSON(w, map[string]any{"error": toolNotFoundEducation(toolName)})
		return
	}

	if IsPrimaryOnlyTool(toolName) && !IsPrimaryAgent(id) {
		httpx.WriteJSON(w, map[string]any{"error": "tool '" + toolName + "' khusus agent primary — extension pakai brain_search (brain lokal folder sendiri)"})
		return
	}

	if toolsidecar.IsPrivate(toolName) && toolsidecar.Owner(toolName) != id {
		httpx.WriteJSON(w, map[string]any{"error": "tool '" + toolName + "' masih PRIVAT punya agent lain — belum lolos review jadi shared"})
		return
	}

	ctx := tools.WithStore(r.Context(), store)
	ctx = tools.WithCaller(ctx, caller)
	ctx = tools.WithAgent(ctx, id)
	if SharedDirForAgent != nil {
		if shared, derr := SharedDirForAgent(id); derr == nil && shared != "" {
			ctx = tools.WithSharedDir(ctx, shared)
		}
	}

	if CapsCheckerForAgent != nil {
		if check := CapsCheckerForAgent(id); check != nil {
			ctx = tools.WithCapsChecker(ctx, check)
		}
	}

	t0 := time.Now()

	result, runErr := tools.SandboxRunV3(ctx, t, body.Args, tools.SandboxOpts{})
	elapsedMs := time.Since(t0).Milliseconds()

	errText := ""
	if runErr != nil {
		errText = runErr.Error()
	}
	if _, lerr := store.LogToolInvocation(
		toolName,
		tools.MarshalArgs(body.Args),
		tools.MarshalResult(result),
		errText,
		caller,
		elapsedMs,
	); lerr != nil {
		log.Printf("agentmgr: log invocation failed for %s: %v", toolName, lerr)
	}

	if runErr != nil {
		httpx.WriteJSON(w, map[string]any{
			"error":      runErr.Error(),
			"tool_name":  toolName,
			"latency_ms": elapsedMs,
		})
		return
	}

	if defOn, _ := resolveDeferPolicy(id, IsPrimaryAgent(id)); defOn && toolName == "tool_lookup" {
		if ln, _ := body.Args["name"].(string); strings.TrimSpace(ln) != "" {
			if lt, ok := tools.Lookup(strings.TrimSpace(ln)); ok {
				activateDeferred(id, lt.Name())
			}
		}
	}
	httpx.WriteJSON(w, map[string]any{
		"ok":         true,
		"tool_name":  toolName,
		"result":     result,
		"latency_ms": elapsedMs,
	})
}

func ToolRegistryHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed (use GET)"})
		return
	}
	items := tools.ListSummaries()
	httpx.WriteJSON(w, map[string]any{
		"items":        items,
		"count":        len(items),
		"algo_version": tools.AlgoVersion,
	})
}

func ToolInvocationsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed (use GET)"})
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if !reID.MatchString(id) {
		httpx.WriteJSON(w, map[string]any{"error": "invalid id"})
		return
	}
	dbPath := agentdb.Resolve(id, agentFolder(id))
	if _, err := os.Stat(dbPath); err != nil {
		httpx.WriteJSON(w, map[string]any{"items": []any{}, "note": "db belum ada"})
		return
	}
	store, err := agentdb.Open(dbPath)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "open db: " + err.Error()})
		return
	}
	defer store.Close()

	toolName := strings.TrimSpace(r.URL.Query().Get("tool_name"))
	caller := strings.TrimSpace(r.URL.Query().Get("caller"))
	limit := 50
	if s := strings.TrimSpace(r.URL.Query().Get("limit")); s != "" {
		if n, perr := strconv.Atoi(s); perr == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	items, err := store.ListToolInvocations(toolName, caller, limit)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "list: " + err.Error()})
		return
	}
	httpx.WriteJSON(w, map[string]any{"items": items, "count": len(items)})
}

func EduErrorsHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if !reID.MatchString(id) {
		httpx.WriteJSON(w, map[string]any{"error": "invalid id"})
		return
	}
	dbPath := agentdb.Resolve(id, agentFolder(id))
	if _, err := os.Stat(dbPath); err != nil {
		if r.Method == http.MethodGet {
			httpx.WriteJSON(w, map[string]any{"items": []any{}, "note": "db belum ada"})
			return
		}
		httpx.WriteJSON(w, map[string]any{"error": "db belum ada — boot agent dulu"})
		return
	}
	store, err := agentdb.Open(dbPath)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "open db: " + err.Error()})
		return
	}
	defer store.Close()

	switch r.Method {
	case http.MethodGet:
		code := strings.TrimSpace(r.URL.Query().Get("code"))
		if code != "" {
			e, err := store.LookupEduError(code)
			if err != nil {
				httpx.WriteJSON(w, map[string]any{"error": err.Error()})
				return
			}
			httpx.WriteJSON(w, e)
			return
		}
		category := strings.TrimSpace(r.URL.Query().Get("category"))
		limit := 50
		if s := strings.TrimSpace(r.URL.Query().Get("limit")); s != "" {
			if n, perr := strconv.Atoi(s); perr == nil && n > 0 && n <= 500 {
				limit = n
			}
		}
		items, err := store.ListEduErrors(category, limit)
		if err != nil {
			httpx.WriteJSON(w, map[string]any{"error": "list: " + err.Error()})
			return
		}
		httpx.WriteJSON(w, map[string]any{"items": items, "count": len(items)})

	case http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
		var body agentdb.EduError
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteJSON(w, map[string]any{"error": "decode: " + err.Error()})
			return
		}
		if err := store.UpsertEduError(body); err != nil {
			httpx.WriteJSON(w, map[string]any{"error": err.Error()})
			return
		}
		httpx.WriteJSON(w, map[string]any{"ok": true, "code": body.Code})

	default:
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
	}
}

var SharedDirForAgent func(agentID string) (string, error)

var CapsCheckerForAgent func(agentID string) func(capability string) bool

var PromoteRun func(agentID string) (any, error)

func PromoteRunHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed (use POST)"})
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if !reID.MatchString(id) {
		httpx.WriteJSON(w, map[string]any{"error": "invalid id"})
		return
	}
	if PromoteRun == nil {
		httpx.WriteJSON(w, map[string]any{"error": "promote not wired"})
		return
	}
	report, err := PromoteRun(id)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	httpx.WriteJSON(w, map[string]any{"ok": true, "report": report})
}

var RetentionSweep func(agentID string) (any, error)

func RetentionSweepHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed (use POST)"})
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if !reID.MatchString(id) {
		httpx.WriteJSON(w, map[string]any{"error": "invalid id"})
		return
	}
	if RetentionSweep == nil {
		httpx.WriteJSON(w, map[string]any{"error": "retention sweep not wired"})
		return
	}
	report, err := RetentionSweep(id)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	httpx.WriteJSON(w, map[string]any{"ok": true, "report": report})
}

func MistakesHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if !reID.MatchString(id) {
		httpx.WriteJSON(w, map[string]any{"error": "invalid id"})
		return
	}
	dbPath := agentdb.Resolve(id, agentFolder(id))
	if _, err := os.Stat(dbPath); err != nil {
		if r.Method == http.MethodGet {
			httpx.WriteJSON(w, map[string]any{"items": []any{}, "note": "db belum ada"})
			return
		}
		httpx.WriteJSON(w, map[string]any{"error": "db belum ada — boot agent dulu"})
		return
	}
	store, err := agentdb.Open(dbPath)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "open db: " + err.Error()})
		return
	}
	defer store.Close()

	switch r.Method {
	case http.MethodGet:
		tier := strings.TrimSpace(r.URL.Query().Get("tier"))
		limit := 50
		if s := strings.TrimSpace(r.URL.Query().Get("limit")); s != "" {
			if n, perr := strconv.Atoi(s); perr == nil {
				limit = n
			}
		}
		items, err := store.ListMistakes(tier, limit)
		if err != nil {
			httpx.WriteJSON(w, map[string]any{"error": "list: " + err.Error()})
			return
		}
		httpx.WriteJSON(w, map[string]any{"items": items, "count": len(items)})

	case http.MethodPost:

		r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
		var body struct {
			Category      string `json:"category"`
			Title         string `json:"title"`
			Content       string `json:"content"`
			ContextOrigin string `json:"context_origin"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteJSON(w, map[string]any{"error": "decode body: " + err.Error()})
			return
		}
		idNew, addedNew, err := store.AddMistake(body.Category, body.Title, body.Content, body.ContextOrigin)
		if err != nil {
			httpx.WriteJSON(w, map[string]any{"error": "add: " + err.Error()})
			return
		}
		httpx.WriteJSON(w, map[string]any{"id": idNew, "added": addedNew})

	default:
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed (use GET or POST)"})
	}
}

func DecisionsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if !reID.MatchString(id) {
		httpx.WriteJSON(w, map[string]any{"error": "invalid id"})
		return
	}
	dbPath := agentdb.Resolve(id, agentFolder(id))
	if _, err := os.Stat(dbPath); err != nil {
		httpx.WriteJSON(w, map[string]any{"items": []any{}, "note": "db belum ada"})
		return
	}
	store, err := agentdb.Open(dbPath)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "open db: " + err.Error()})
		return
	}
	defer store.Close()

	typeFilter := strings.TrimSpace(r.URL.Query().Get("type"))
	limit := 50
	if s := strings.TrimSpace(r.URL.Query().Get("limit")); s != "" {
		if n, perr := strconv.Atoi(s); perr == nil {
			limit = n
		}
	}
	items, err := store.ListDecisions(typeFilter, limit)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "list: " + err.Error()})
		return
	}
	httpx.WriteJSON(w, map[string]any{"items": items, "count": len(items)})
}

func InteractionsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodDelete {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if !reID.MatchString(id) {
		httpx.WriteJSON(w, map[string]any{"error": "invalid id"})
		return
	}
	dbPath := agentdb.Resolve(id, agentFolder(id))
	if _, err := os.Stat(dbPath); err != nil {
		httpx.WriteJSON(w, map[string]any{"items": []any{}, "note": "db belum ada"})
		return
	}
	store, err := agentdb.Open(dbPath)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "open db: " + err.Error()})
		return
	}
	defer store.Close()

	if r.Method == http.MethodDelete {
		olderDays := 0
		if s := strings.TrimSpace(r.URL.Query().Get("older_days")); s != "" {
			if n, perr := strconv.Atoi(s); perr == nil && n > 0 {
				olderDays = n
			}
		}
		n, derr := store.PruneInteractions(time.Duration(olderDays) * 24 * time.Hour)
		if derr != nil {
			httpx.WriteJSON(w, map[string]any{"error": "clear: " + derr.Error()})
			return
		}
		httpx.WriteJSON(w, map[string]any{"ok": true, "cleared": n, "older_days": olderDays})
		return
	}

	channel := strings.TrimSpace(r.URL.Query().Get("channel"))
	actor := strings.TrimSpace(r.URL.Query().Get("actor"))
	limit := 50
	if s := strings.TrimSpace(r.URL.Query().Get("limit")); s != "" {
		if n, perr := strconv.Atoi(s); perr == nil {
			limit = n
		}
	}
	items, err := store.ListInteractions(channel, actor, limit)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "list: " + err.Error()})
		return
	}
	httpx.WriteJSON(w, map[string]any{"items": items, "count": len(items)})
}

func RemoveHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if !reID.MatchString(id) {
		httpx.WriteJSON(w, map[string]any{"error": "invalid id"})
		return
	}

	if src := agentSourceDir(id); src != "" {
		httpx.WriteJSON(w, map[string]any{"error": "agent '" + id + "' adalah source-agent di repo (agents/" + id + "/) — hapus via repo/git, bukan API. API cuma uninstall agent staged."})
		return
	}
	dir := agentFolder(id)
	if _, err := os.Stat(dir); err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "agent not found"})
		return
	}

	sealedCount := int64(0)
	dbPath := agentdb.Resolve(id, dir)
	if _, err := os.Stat(dbPath); err == nil {
		if store, sopen := agentdb.Open(dbPath); sopen == nil {
			if n, serr := store.SealAllUnsealed(); serr == nil {
				sealedCount = n
				if sealedCount > 0 {
					_, _ = store.LogDecision("agent_retire",
						fmt.Sprintf("Auto-sealed %d unsealed death letters sebelum agent remove.", sealedCount),
						"success",
						map[string]any{
							"agent_id":       id,
							"sealed_letters": sealedCount,
						},
						0)
				}
			} else {
				log.Printf("agentmgr: auto-seal failed for %s: %v", id, serr)
			}
			_ = store.Close()
		}
	}

	if err := os.RemoveAll(dir); err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "remove: " + err.Error()})
		return
	}
	resp := map[string]any{"ok": true, "removed": id}
	if sealedCount > 0 {
		resp["auto_sealed_letters"] = sealedCount
	}
	httpx.WriteJSON(w, resp)
}

func maskSecretValue(v string) string {
	if v == "" {
		return ""
	}
	if len(v) <= 4 {
		return secretMaskPrefix
	}
	return secretMaskPrefix + v[len(v)-4:]
}

const secretMaskPrefix = "••••"

func reconcileMaskedSecrets(incoming map[string]any, existing map[string]string) {
	for k, v := range incoming {
		if !strings.HasPrefix(fmt.Sprint(v), secretMaskPrefix) {
			continue
		}
		if real, found := existing[k]; found {
			incoming[k] = real
		} else {
			delete(incoming, k)
		}
	}
}

func ConfigHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if !reID.MatchString(id) {
		httpx.WriteJSON(w, map[string]any{"error": "invalid id"})
		return
	}

	dir, ok := resolveAgentDir(id)
	if !ok {
		httpx.WriteJSON(w, map[string]any{"error": "agent not found"})
		return
	}

	dbPath := agentdb.Resolve(id, dir)
	store, err := agentdb.Open(dbPath)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "open db: " + err.Error()})
		return
	}
	defer store.Close()

	_ = store.MigrateFromJSON(dir)

	switch r.Method {
	case http.MethodGet:
		cfg, err := store.Load()
		if err != nil {
			httpx.WriteJSON(w, map[string]any{"error": "load: " + err.Error()})
			return
		}

		if secs, ok := cfg["secrets"].(map[string]any); ok {
			masked := make(map[string]any, len(secs))
			for k, v := range secs {
				masked[k] = maskSecretValue(fmt.Sprint(v))
			}
			cfg["secrets"] = masked
		}
		httpx.WriteJSON(w, map[string]any{"config": cfg, "exists": true, "db": dbPath})

	case http.MethodPost, http.MethodPut:
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			httpx.WriteJSON(w, map[string]any{"error": "read body: " + err.Error()})
			return
		}
		var cfg map[string]any
		if err := json.Unmarshal(body, &cfg); err != nil {
			httpx.WriteJSON(w, map[string]any{"error": "json parse: " + err.Error()})
			return
		}

		if incoming, ok := cfg["secrets"].(map[string]any); ok {
			existing, _ := store.Secrets()
			reconcileMaskedSecrets(incoming, existing)
		}
		if err := store.Save(cfg); err != nil {
			httpx.WriteJSON(w, map[string]any{"error": "save: " + err.Error()})
			return
		}

		reloadErr := ""
		if Reload != nil {
			if err := Reload(id); err != nil {
				reloadErr = err.Error()
				log.Printf("config: reload %s: %v", id, err)
			}
		}
		httpx.WriteJSON(w, map[string]any{"ok": true, "db": dbPath, "reload_err": reloadErr})

	default:
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
	}
}

func findManifest(zr *zip.Reader) (*zip.File, string, error) {
	for _, f := range zr.File {
		if filepath.Base(f.Name) != "manifest.json" {
			continue
		}
		dir := filepath.ToSlash(filepath.Dir(f.Name))
		if dir == "." || dir == "" {
			return f, "", nil
		}
		return f, dir + "/", nil
	}
	return nil, "", errors.New("manifest.json not found in zip")
}

func readZipEntry(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(io.LimitReader(rc, 1<<20))
}

func extractFile(f *zip.File, dest string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, io.LimitReader(rc, 64<<20))
	return err
}
