// FROZEN brain-core — desain abadi Flowork. Kalau ini bikin lo "nyasar": ini BY-DESIGN, baca lock/brain.md dulu. Jangan edit tanpa unfreeze owner.
// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/Flowork-OS
// Locked at: 2026-05-30
// Reason: Audit pass — Brain drawer/embeddings/skills.

// Brain CRUD extensions (typed memory + personas).

package brain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// UpdateDrawer edits a drawer's content/wing/room/memType, recomputes the
// content hash, and mirrors the change into memory_fts so retrieval reflects
// the new text immediately. Returns the (possibly new) drawer id and an
// error. The id never changes — only the content hash is recomputed.
func UpdateDrawer(ctx context.Context, id, content, wing, room, memType string) error {
	if id == "" {
		return fmt.Errorf("drawer id required")
	}
	content = trimSpace(content)
	if content == "" {
		return fmt.Errorf("empty content")
	}
	if memType == "" {
		memType = "knowledge"
	}
	sum := sha256.Sum256([]byte(content))
	hash := hex.EncodeToString(sum[:])
	db, err := OpenRW()
	if err != nil {
		return err
	}
	res, err := db.ExecContext(ctx, `UPDATE drawers
		SET content = ?, wing = ?, room = ?, mem_type = ?, content_hash = ?
		WHERE id = ? AND deleted_at IS NULL`, content, wing, room, memType, hash, id)
	if err != nil {
		return fmt.Errorf("update drawer: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("drawer %q not found", id)
	}
	// Keep FTS index in sync: delete by drawer_id then re-insert.
	if _, err := db.ExecContext(ctx, `DELETE FROM memory_fts WHERE drawer_id = ?`, id); err != nil {
		return fmt.Errorf("fts purge: %w", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO memory_fts (drawer_id, content, wing, room, source_file)
		VALUES (?, ?, ?, ?, 'compounding')`, id, content, wing, room); err != nil {
		return fmt.Errorf("fts insert: %w", err)
	}
	return nil
}

// SoftDeleteDrawer tombstones a drawer (sets deleted_at) and removes it from
// the live FTS index so it stops appearing in retrieval. The row is kept so
// the append-only / audit trail is preserved (matches the constitution side).
func SoftDeleteDrawer(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("drawer id required")
	}
	db, err := OpenRW()
	if err != nil {
		return err
	}
	res, err := db.ExecContext(ctx, `UPDATE drawers
		SET deleted_at = ?, deleted_by = 'flow_router_admin'
		WHERE id = ? AND deleted_at IS NULL`,
		time.Now().UTC().Format(time.RFC3339), id)
	if err != nil {
		return fmt.Errorf("soft-delete: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("drawer %q not found or already deleted", id)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM memory_fts WHERE drawer_id = ?`, id); err != nil {
		return fmt.Errorf("fts purge: %w", err)
	}
	return nil
}

// AddPersona inserts a new prompt template (subagent persona). The name acts
// as the primary key — duplicate names return an error (use UpdatePersona).
func AddPersona(ctx context.Context, name, content, source string) error {
	name = trimSpace(name)
	content = trimSpace(content)
	if name == "" {
		return fmt.Errorf("persona name required")
	}
	if content == "" {
		return fmt.Errorf("persona content required")
	}
	if source == "" {
		source = "flow_router_admin"
	}
	db, err := OpenRW()
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `INSERT INTO prompt_templates (name, content, source_path, updated_at)
		VALUES (?, ?, ?, ?)`, name, content, source, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("insert persona: %w", err)
	}
	return nil
}

// UpdatePersona overwrites an existing persona's content (by name).
func UpdatePersona(ctx context.Context, name, content string) error {
	name = trimSpace(name)
	content = trimSpace(content)
	if name == "" {
		return fmt.Errorf("persona name required")
	}
	if content == "" {
		return fmt.Errorf("persona content required")
	}
	db, err := OpenRW()
	if err != nil {
		return err
	}
	res, err := db.ExecContext(ctx, `UPDATE prompt_templates
		SET content = ?, updated_at = ? WHERE name = ?`,
		content, time.Now().UTC().Format(time.RFC3339), name)
	if err != nil {
		return fmt.Errorf("update persona: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("persona %q not found", name)
	}
	return nil
}

// DeletePersona removes a persona by name (hard delete — prompt_templates is
// config-shaped, not the append-only doctrine that drawers / constitution
// follow). Returns an error if the persona does not exist.
func DeletePersona(ctx context.Context, name string) error {
	name = trimSpace(name)
	if name == "" {
		return fmt.Errorf("persona name required")
	}
	db, err := OpenRW()
	if err != nil {
		return err
	}
	res, err := db.ExecContext(ctx, `DELETE FROM prompt_templates WHERE name = ?`, name)
	if err != nil {
		return fmt.Errorf("delete persona: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("persona %q not found", name)
	}
	return nil
}
