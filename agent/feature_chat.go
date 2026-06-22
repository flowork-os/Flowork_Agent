// feature_chat.go — FASE-B: chat sessions (ChatGPT-style) + channel HTTP/CLI ke mr-flow.
package main

import "log"

func init() {
	RegisterFeature(Feature{Name: "chat", Phase: PhaseRoute, Apply: func(d *Deps) {
		if cerr := d.FDB.EnsureChatSchema(); cerr != nil {
			log.Printf("chat: EnsureChatSchema: %v", cerr)
		}
		d.Mux.HandleFunc("/api/chat/sessions", chatSessionsHandler(d.FDB))
		d.Mux.HandleFunc("/api/chat/sessions/rename", chatSessionRenameHandler(d.FDB))
		d.Mux.HandleFunc("/api/chat/sessions/delete", chatSessionDeleteHandler(d.FDB))
		d.Mux.HandleFunc("/api/chat/sessions/meta", chatSessionMetaHandler(d.FDB))
		d.Mux.HandleFunc("/api/chat/sessions/messages", chatMessagesHandler(d.FDB))
		d.Mux.HandleFunc("/api/chat/send", chatSendHandler(d.Host, d.FDB, d.GroupsAPI))
		// CHANNEL HTTP/CLI ke mr-flow channel-agnostic core (test-harness doktrin).
		d.Mux.HandleFunc("/api/chat", chatHandler(d.Host))
	}})
}
