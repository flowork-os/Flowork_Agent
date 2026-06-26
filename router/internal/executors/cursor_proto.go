// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package executors

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	wireVarint = 0
	wireLen    = 2
)

const (
	cursorRoleUser      = 1
	cursorRoleAssistant = 2
)

const (
	cursorModeChat  = 1
	cursorModeAgent = 2
)

const (
	cfRequest = 1

	cfMessages      = 1
	cfModel         = 5
	cfCursorSetting = 15
	cfMetadata      = 26
	cfIsAgentic     = 27
	cfUnifiedMode   = 46
	cfUnifiedName   = 54

	cfMsgContent     = 1
	cfMsgRole        = 2
	cfMsgID          = 13
	cfMsgIsAgentic   = 29
	cfMsgUnifiedMode = 47

	cfResponseText = 1
)

func encodeVarint(value uint64) []byte {
	buf := make([]byte, 0, binary.MaxVarintLen64)
	for value >= 0x80 {
		buf = append(buf, byte(value&0x7F)|0x80)
		value >>= 7
	}
	buf = append(buf, byte(value&0x7F))
	return buf
}

func decodeVarint(buf []byte, offset int) (uint64, int, error) {
	var v uint64
	var shift uint
	pos := offset
	for {
		if pos >= len(buf) {
			return 0, 0, errors.New("varint: truncated")
		}
		b := buf[pos]
		pos++
		v |= uint64(b&0x7F) << shift
		if b < 0x80 {
			return v, pos - offset, nil
		}
		shift += 7
		if shift >= 64 {
			return 0, 0, errors.New("varint: overflow")
		}
	}
}

func encodeFieldVarint(fieldNum int, value uint64) []byte {
	tag := uint64(fieldNum<<3) | wireVarint
	out := encodeVarint(tag)
	return append(out, encodeVarint(value)...)
}

func encodeFieldLen(fieldNum int, data []byte) []byte {
	tag := uint64(fieldNum<<3) | wireLen
	out := encodeVarint(tag)
	out = append(out, encodeVarint(uint64(len(data)))...)
	out = append(out, data...)
	return out
}

func encodeFieldLenString(fieldNum int, s string) []byte {
	return encodeFieldLen(fieldNum, []byte(s))
}

func concatBytes(parts ...[]byte) []byte {
	n := 0
	for _, p := range parts {
		n += len(p)
	}
	out := make([]byte, 0, n)
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func encodeCursorMessage(content, role, messageID string, hasTools bool) []byte {
	roleNum := uint64(cursorRoleUser)
	if role == "assistant" {
		roleNum = cursorRoleAssistant
	}
	mode := uint64(cursorModeChat)
	agentic := uint64(0)
	if hasTools {
		mode = cursorModeAgent
		agentic = 1
	}
	return concatBytes(
		encodeFieldLenString(cfMsgContent, content),
		encodeFieldVarint(cfMsgRole, roleNum),
		encodeFieldLenString(cfMsgID, messageID),
		encodeFieldVarint(cfMsgIsAgentic, agentic),
		encodeFieldVarint(cfMsgUnifiedMode, mode),
	)
}

func encodeCursorChatRequest(messages []CursorMessage, modelName string) []byte {
	parts := make([][]byte, 0, len(messages)+4)
	for i, m := range messages {
		mid := fmt.Sprintf("msg-%d", i)
		parts = append(parts, encodeFieldLen(cfMessages, encodeCursorMessage(m.Content, m.Role, mid, false)))
	}

	model := encodeFieldLenString(1, modelName)
	parts = append(parts, encodeFieldLen(cfModel, model))

	parts = append(parts, encodeFieldVarint(cfUnifiedMode, cursorModeChat))
	parts = append(parts, encodeFieldLenString(cfUnifiedName, "chat"))
	parts = append(parts, encodeFieldVarint(cfIsAgentic, 0))

	return concatBytes(parts...)
}

type CursorMessage struct {
	Content string
	Role    string
}

func wrapConnectRPCFrame(payload []byte) []byte {
	frame := make([]byte, 5+len(payload))
	frame[0] = 0x00
	binary.BigEndian.PutUint32(frame[1:5], uint32(len(payload)))
	copy(frame[5:], payload)
	return frame
}

type connectFrame struct {
	Flags    byte
	Length   int
	Payload  []byte
	Consumed int
}

func parseConnectRPCFrame(buf []byte) *connectFrame {
	if len(buf) < 5 {
		return nil
	}

	length64 := uint64(binary.BigEndian.Uint32(buf[1:5]))
	if uint64(len(buf)) < 5+length64 {
		return nil
	}
	length := int(length64)
	payload := make([]byte, length)
	copy(payload, buf[5:5+length])
	return &connectFrame{
		Flags:    buf[0],
		Length:   length,
		Payload:  payload,
		Consumed: 5 + length,
	}
}

func extractTextFromCursorResponse(payload []byte) string {
	if len(payload) == 0 {
		return ""
	}
	out := ""
	offset := 0
	for offset < len(payload) {
		tag, n, err := decodeVarint(payload, offset)
		if err != nil {
			return out
		}
		offset += n
		fieldNum := int(tag >> 3)
		wireType := int(tag & 0x7)

		switch wireType {
		case wireVarint:
			_, n, err := decodeVarint(payload, offset)
			if err != nil {
				return out
			}
			offset += n
		case wireLen:
			length, n, err := decodeVarint(payload, offset)
			if err != nil {
				return out
			}
			offset += n
			if offset+int(length) > len(payload) {
				return out
			}
			data := payload[offset : offset+int(length)]
			offset += int(length)
			if fieldNum == cfResponseText && looksLikeUTF8(data) {
				out += string(data)
			} else {

				if nested := extractTextFromCursorResponse(data); nested != "" {
					out += nested
				}
			}
		default:

			return out
		}
	}
	return out
}

func looksLikeUTF8(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	printable := 0
	for _, c := range b {
		if c == 0x09 || c == 0x0A || c == 0x0D || (c >= 0x20 && c < 0x7F) || c >= 0xC0 {
			printable++
		}
	}
	return printable*100/len(b) >= 80
}
