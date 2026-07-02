// telegram_media.go — CABANG (extension) NON-FROZEN buat mr-flow Telegram: FORMAT pesan rapi
// (parse_mode) + BACA media (dokumen/foto/voice). main.go (frozen) cuma manggil fungsi di sini.
//
// ⚖️ ATURAN ABADI (owner Mr.Dev, 2026-06-23): filtur Telegram baru (format, tipe media, vision,
// STT) masuk SINI — main.go frozen ga dibuka lagi. Switch via env. Dok → lock/mrflow.md.
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"os"
	"regexp"
	"strings"
)

// ── FORMAT pesan biar RAPI di Telegram ──────────────────────────────────────
// Akar (owner 2026-06-23): LLM output markdown (**bold**, `code`, # header, dst) tapi sendMessage
// dulu TANPA parse_mode → muncul mentah = "ngak rapi". Fix: convert ke Telegram HTML + parse_mode.
//
// Switch FLOWORK_TG_FORMAT: "html" (default, rapi) | "plain"/"off" (strip markdown, polos).

var (
	reCodeBlock = regexp.MustCompile("(?s)```[a-zA-Z0-9]*\n?(.*?)```")
	reInlineCod = regexp.MustCompile("`([^`\n]+)`")
	reBoldStar  = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	reBoldUnd   = regexp.MustCompile(`__([^_]+)__`)
	reItalic    = regexp.MustCompile(`(^|[\s(])\*([^*\n]+)\*`)
	reStrike    = regexp.MustCompile(`~~([^~]+)~~`)
	reLink      = regexp.MustCompile(`\[([^\]]+)\]\((https?://[^)\s]+)\)`)
	reHeader    = regexp.MustCompile(`(?m)^#{1,6}\s*(.+)$`)
	rePlaceCB   = regexp.MustCompile("\x00CB(\\d+)\x00")
	rePlaceIC   = regexp.MustCompile("\x00IC(\\d+)\x00")
)

func htmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// tgFormatMode — baca switch. balik "" kalau plain/off.
func tgFormatMode() string {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("FLOWORK_TG_FORMAT"))) {
	case "plain", "off", "0":
		return ""
	default:
		return "HTML"
	}
}

// formatTelegram — convert markdown LLM → Telegram HTML (atau strip kalau plain). Balik (teks, parseMode).
// Robust: code-block/inline-code di-"parkir" dulu (biar isinya ga ke-format), escape HTML, convert sisanya.
func formatTelegram(text string) (string, string) {
	if tgFormatMode() == "" {
		return stripMarkdown(text), ""
	}
	// 1. Parkir code blocks + inline code (placeholder non-printable) biar isinya literal.
	var blocks, inlines []string
	text = reCodeBlock.ReplaceAllStringFunc(text, func(m string) string {
		sub := reCodeBlock.FindStringSubmatch(m)
		blocks = append(blocks, sub[1])
		return "\x00CB" + itoa(len(blocks)-1) + "\x00"
	})
	text = reInlineCod.ReplaceAllStringFunc(text, func(m string) string {
		sub := reInlineCod.FindStringSubmatch(m)
		inlines = append(inlines, sub[1])
		return "\x00IC" + itoa(len(inlines)-1) + "\x00"
	})
	// 2. Escape HTML di teks biasa.
	text = htmlEscape(text)
	// 3. Convert markdown → HTML tag.
	text = reHeader.ReplaceAllString(text, "<b>$1</b>")
	text = reBoldStar.ReplaceAllString(text, "<b>$1</b>")
	text = reBoldUnd.ReplaceAllString(text, "<b>$1</b>")
	text = reStrike.ReplaceAllString(text, "<s>$1</s>")
	text = reItalic.ReplaceAllString(text, "$1<i>$2</i>")
	text = reLink.ReplaceAllString(text, `<a href="$2">$1</a>`)
	// 4. Kembaliin code (isi di-escape, bungkus tag pre/code).
	text = rePlaceCB.ReplaceAllStringFunc(text, func(m string) string {
		i := atoi(rePlaceCB.FindStringSubmatch(m)[1])
		if i >= 0 && i < len(blocks) {
			return "<pre>" + htmlEscape(blocks[i]) + "</pre>"
		}
		return m
	})
	text = rePlaceIC.ReplaceAllStringFunc(text, func(m string) string {
		i := atoi(rePlaceIC.FindStringSubmatch(m)[1])
		if i >= 0 && i < len(inlines) {
			return "<code>" + htmlEscape(inlines[i]) + "</code>"
		}
		return m
	})
	return text, "HTML"
}

// splitForTelegram — CABANG non-frozen (#chunk 2026-06-26): pesan PANJANG dipotong jadi beberapa
// bagian biar GA kepotong limit Telegram (4096). Pakai 3500 rune/chunk (margin buat HTML convert).
// Potong di batas BAGUS: paragraf (\n\n) > baris (\n) > spasi → ga motong tengah kata/kalimat.
// Switch FLOWORK_TG_CHUNK=0 → matiin (1 pesan, perilaku lama). main.go sendMessage manggil ini.
func splitForTelegram(text string) []string {
	const limit = 3500
	if os.Getenv("FLOWORK_TG_CHUNK") == "0" {
		return []string{text}
	}
	r := []rune(text)
	if len(r) <= limit {
		return []string{text}
	}
	var out []string
	start := 0
	for start < len(r) {
		if start+limit >= len(r) {
			out = append(out, strings.TrimRight(string(r[start:]), " \n"))
			break
		}
		end := start + limit
		brk := -1
		for i := end - 1; i > start+limit/2; i-- { // 1) paragraf
			if r[i] == '\n' && i+1 < len(r) && r[i+1] == '\n' {
				brk = i + 1
				break
			}
		}
		if brk < 0 { // 2) baris
			for i := end - 1; i > start+limit/2; i-- {
				if r[i] == '\n' {
					brk = i
					break
				}
			}
		}
		if brk < 0 { // 3) spasi
			for i := end - 1; i > start+limit/2; i-- {
				if r[i] == ' ' {
					brk = i
					break
				}
			}
		}
		if brk < 0 {
			brk = end // hard-cut (1 kata super panjang)
		}
		out = append(out, strings.TrimRight(string(r[start:brk]), " \n"))
		start = brk
		for start < len(r) && (r[start] == '\n' || r[start] == ' ') {
			start++
		}
	}
	res := out[:0]
	for _, c := range out {
		if strings.TrimSpace(c) != "" {
			res = append(res, c)
		}
	}
	return res
}

// stripMarkdown — buang penanda markdown → teks polos bersih (mode plain).
func stripMarkdown(text string) string {
	text = reCodeBlock.ReplaceAllString(text, "$1")
	text = reInlineCod.ReplaceAllString(text, "$1")
	text = reBoldStar.ReplaceAllString(text, "$1")
	text = reBoldUnd.ReplaceAllString(text, "$1")
	text = reStrike.ReplaceAllString(text, "$1")
	text = reItalic.ReplaceAllString(text, "$1$2")
	text = reLink.ReplaceAllString(text, "$1 ($2)")
	text = reHeader.ReplaceAllString(text, "$1")
	return text
}

// small int helpers (hindari fmt buat hot path).
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b [20]byte
	p := len(b)
	for i > 0 {
		p--
		b[p] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		p--
		b[p] = '-'
	}
	return string(b[p:])
}
func atoi(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return -1
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// ── BACA MEDIA (dokumen/foto/voice) ─────────────────────────────────────────
// Switch FLOWORK_TG_MEDIA (default ON; "off"/"0" = cuma text). Semua GRACEFUL: kalau download/
// STT/vision gagal → tetep balik teks acknowledge (bot ga pernah diem/crash).

func tgMediaOn() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("FLOWORK_TG_MEDIA"))) {
	case "off", "0", "false", "no":
		return false
	}
	return true
}

// tgRouterBase — ".../v1" dari defaultRouter (".../v1/chat/completions").
func tgRouterBase() string {
	if i := strings.LastIndex(defaultRouter, "/chat/completions"); i > 0 {
		return defaultRouter[:i]
	}
	return "http://127.0.0.1:2402/v1"
}

// enrichMedia — pesan tanpa text tapi ada media → jadiin teks yg bisa diproses LLM. "" = ga ada.
func enrichMedia(m *Message, token string) string {
	if !tgMediaOn() || m == nil {
		return ""
	}
	cap := strings.TrimSpace(m.Caption)
	switch {
	case m.Voice != nil:
		return mediaVoice(token, m.Voice.FileID, cap)
	case m.Audio != nil:
		return mediaVoice(token, m.Audio.FileID, cap)
	case m.Document != nil:
		return mediaDocument(token, m.Document, cap)
	case len(m.Photo) > 0:
		return mediaPhoto(token, m.Photo[len(m.Photo)-1].FileID, cap)
	}
	return ""
}

func tgGetFilePath(token, fileID string) (string, error) {
	resp, err := fetch("GET", "https://api.telegram.org/bot"+token+"/getFile?file_id="+fileID, nil, nil, 15000)
	if err != nil {
		return "", err
	}
	var r struct {
		OK     bool `json:"ok"`
		Result struct {
			FilePath string `json:"file_path"`
		} `json:"result"`
	}
	if json.Unmarshal(resp.Body, &r) != nil || !r.OK || r.Result.FilePath == "" {
		return "", fmt.Errorf("getFile gagal (status %d)", resp.Status)
	}
	return r.Result.FilePath, nil
}

func tgDownload(token, fileID string) ([]byte, string, error) {
	fp, err := tgGetFilePath(token, fileID)
	if err != nil {
		return nil, "", err
	}
	resp, err := fetch("GET", "https://api.telegram.org/file/bot"+token+"/"+fp, nil, nil, 30000)
	if err != nil {
		return nil, "", err
	}
	if resp.Status >= 400 {
		return nil, fp, fmt.Errorf("download status %d", resp.Status)
	}
	return resp.Body, fp, nil
}

// mediaVoice — voice/audio → STT (router /v1/audio/transcriptions multipart). Owner thinks suara udah
// bisa = bener kalau STT provider aktif (Media Providers). Kalau ga → graceful note.
func mediaVoice(token, fileID, caption string) string {
	data, fp, err := tgDownload(token, fileID)
	if err != nil {
		return "[🎤 voice diterima tapi gagal di-download: " + err.Error() + "]"
	}
	transcript, terr := sttTranscribe(data, baseName(fp))
	if terr != nil {
		return "[🎤 Owner kirim voice note (" + itoa(len(data)) + "B). Transkrip gagal: " + terr.Error() +
			". Kalau STT belum diset, minta owner ketik atau set STT provider di Settings → Media Providers.]"
	}
	out := "[🎤 transkrip voice owner]: " + transcript
	if caption != "" {
		out = caption + "\n" + out
	}
	return out
}

func sttTranscribe(audio []byte, filename string) (string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, ferr := w.CreateFormFile("file", filename)
	if ferr != nil {
		return "", ferr
	}
	_, _ = fw.Write(audio)
	_ = w.WriteField("model", "whisper-1")
	_ = w.Close()
	resp, err := fetch("POST", tgRouterBase()+"/audio/transcriptions",
		map[string]string{"Content-Type": w.FormDataContentType()}, buf.Bytes(), 60000)
	if err != nil {
		return "", err
	}
	if resp.Status >= 400 {
		return "", fmt.Errorf("STT %d: %s", resp.Status, truncStr(string(resp.Body), 120))
	}
	var r struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(resp.Body, &r) != nil || strings.TrimSpace(r.Text) == "" {
		return "", fmt.Errorf("transkrip kosong")
	}
	return r.Text, nil
}

// mediaDocument — download + baca teks (txt/md/code/json/csv). Binary (pdf/docx) → note.
func mediaDocument(token string, doc *TgDocument, caption string) string {
	data, _, err := tgDownload(token, doc.FileID)
	if err != nil {
		return "[📄 dokumen " + doc.FileName + " gagal di-download: " + err.Error() + "]"
	}
	head := "[📄 Owner kirim dokumen: " + doc.FileName + "]"
	if caption != "" {
		head = caption + "\n" + head
	}
	if isTextDoc(doc.FileName, doc.MimeType) && isMostlyText(data) {
		content := string(data)
		if len(content) > 12000 {
			content = content[:12000] + "\n…(dipotong, dokumen panjang)"
		}
		return head + "\nISI DOKUMEN:\n" + content
	}
	return head + " — format " + doc.MimeType + " belum bisa dibaca isinya penuh (cuma teks/markdown/code/json/csv). " +
		"Kalau perlu dibahas, minta owner kirim sbg teks atau .md."
}

// mediaPhoto — download + VISION lewat chat endpoint (image_url). Best-effort: kalau model/router ga
// support vision → fallback acknowledge + caption. mr-flow pakai Claude (vision-capable) → mestinya jalan.
func mediaPhoto(token, fileID, caption string) string {
	data, fp, err := tgDownload(token, fileID)
	if err != nil {
		return "[📷 foto diterima tapi gagal di-download: " + err.Error() + "]"
	}
	desc, verr := visionDescribe(data, mimeFromPath(fp), caption)
	if verr == nil && strings.TrimSpace(desc) != "" {
		out := "[📷 Owner kirim foto. Yang gw LIHAT di foto: " + desc + "]"
		if caption != "" {
			out = caption + "\n" + out
		}
		return out
	}
	out := "[📷 Owner kirim foto (" + itoa(len(data)) + "B)"
	if caption != "" {
		out += ", caption: \"" + caption + "\""
	}
	if verr != nil {
		out += ". Vision belum kebaca (" + verr.Error() + ")"
	}
	out += "]"
	return out
}

func visionDescribe(img []byte, mime, caption string) (string, error) {
	dataURI := "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(img)
	q := "Lihat foto ini, deskripsikan isinya detail tapi ringkas (bahasa Indonesia santai)."
	if caption != "" {
		q = "Owner kirim foto dgn caption: \"" + caption + "\". Lihat fotonya, tanggapi sesuai caption + deskripsikan yg lo lihat."
	}
	// KONVENSI FLOWORK (fix 2026-07-02): content WAJIB STRING — router OpenAIMessage.Content
	// itu `string`, jadi array native GAGAL unmarshal → HTTP 400 (bug "vision Telegram 400").
	// Content-block dikirim sbg JSON STRING (di-decode router: preprocess_content + visionblocks
	// → seam vision anthropic/gemini). SAMA persis kayak jalur GUI Chat (chat_vision.go).
	blocks := []map[string]any{
		{"type": "text", "text": q},
		{"type": "image_url", "image_url": map[string]any{"url": dataURI}},
	}
	blocksJSON, _ := json.Marshal(blocks)
	payload := map[string]any{
		"model": defaultModel,
		"messages": []any{
			map[string]any{"role": "user", "content": string(blocksJSON)},
		},
	}
	body, _ := json.Marshal(payload)
	resp, err := fetch("POST", defaultRouter, map[string]string{"Content-Type": "application/json"}, body, 90000)
	if err != nil {
		return "", err
	}
	if resp.Status >= 400 {
		return "", fmt.Errorf("status %d", resp.Status)
	}
	var r struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if json.Unmarshal(resp.Body, &r) != nil || len(r.Choices) == 0 {
		return "", fmt.Errorf("parse vision")
	}
	return r.Choices[0].Message.Content, nil
}

// ── helpers media ──
func baseName(p string) string {
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[i+1:]
	}
	return p
}

func isTextDoc(name, mime string) bool {
	n := strings.ToLower(name)
	for _, ext := range []string{".txt", ".md", ".markdown", ".json", ".csv", ".log", ".yaml", ".yml",
		".go", ".py", ".js", ".ts", ".sh", ".html", ".css", ".xml", ".ini", ".toml", ".sql", ".env", ".rs", ".java", ".c", ".cpp"} {
		if strings.HasSuffix(n, ext) {
			return true
		}
	}
	return strings.HasPrefix(mime, "text/") || mime == "application/json"
}

// isMostlyText — heuristik: cek sample awal, mayoritas printable/whitespace → teks (bukan binary).
func isMostlyText(b []byte) bool {
	n := len(b)
	if n == 0 {
		return false
	}
	if n > 2048 {
		n = 2048
	}
	bad := 0
	for i := 0; i < n; i++ {
		c := b[i]
		if c == 0 {
			return false
		}
		if c < 9 || (c > 13 && c < 32) {
			bad++
		}
	}
	return bad*20 < n // <5% kontrol non-whitespace
}

func mimeFromPath(p string) string {
	n := strings.ToLower(p)
	switch {
	case strings.HasSuffix(n, ".png"):
		return "image/png"
	case strings.HasSuffix(n, ".webp"):
		return "image/webp"
	case strings.HasSuffix(n, ".gif"):
		return "image/gif"
	default:
		return "image/jpeg"
	}
}
