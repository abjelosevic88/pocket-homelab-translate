package server

import (
	"net/http"

	"github.com/abjelosevic88/pocket-homelab-translate/internal/notify"
)

// notifyTestBody is the test message from Settings → Notifications.
const notifyTestBody = "Notifications from Pocket Translate work. You will hear from it when a long document is translated."

func (s *Server) handleNotifyTest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	st, _ := s.store.GetSettings(ctx, actorID(ctx))
	n := notify.New(st.NotifyURL, st.NotifyFormat)
	if err := n.Send(ctx, notify.Message{Title: "Pocket Translate", Body: notifyTestBody, Priority: "info", Tags: []string{"memo"}}); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
