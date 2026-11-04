package handler

import (
	"fmt"
	"net/http"
	"time"

	"github.com/mgnsk/splitgo/domain"
	"github.com/mgnsk/splitgo/live"
	"github.com/mgnsk/splitgo/pkg/snowflake"
)

// eventStreamDuration is how long one event stream lasts before the browser
// reconnects. It stays below the server's write and request timeouts.
const eventStreamDuration = 50 * time.Second

// EventsHandler streams change events to open pages.
type EventsHandler struct {
	*Deps
}

// NewEventsHandler creates a new events handler.
func NewEventsHandler(deps *Deps) *EventsHandler {
	return &EventsHandler{Deps: deps}
}

// Register the handler.
func (h *EventsHandler) Register(mux *http.ServeMux) {
	h.handle(mux, "GET /events", h.Stream, true)
}

// Stream sends a refresh event whenever a group of the user changes.
func (h *EventsHandler) Stream(w http.ResponseWriter, r *http.Request) {
	me := mustUser(r)
	rc := http.NewResponseController(w)

	events, unsubscribe := h.Live.Subscribe(me.ID)
	defer unsubscribe()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no") // Don't buffer behind nginx.
	w.WriteHeader(http.StatusOK)

	// Reconnect quickly after the stream ends.
	if _, err := fmt.Fprint(w, "retry: 1000\n\n"); err != nil {
		return
	}
	if err := rc.Flush(); err != nil {
		return
	}

	end := time.NewTimer(eventStreamDuration)
	defer end.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-h.Live.Done():
			return
		case <-end.C:
			return
		case e := <-events:
			if _, err := fmt.Fprintf(w, "event: refresh\ndata: %s\n\n", e.GroupID); err != nil {
				return
			}
			if err := rc.Flush(); err != nil {
				return
			}
		}
	}
}

// publishGroup tells the open pages of users that a group changed.
func (d *Deps) publishGroup(groupID snowflake.ID, users ...[]*domain.User) {
	var ids []snowflake.ID
	for _, list := range users {
		ids = append(ids, memberIDs(list)...)
	}
	d.Live.Publish(ids, live.Event{GroupID: groupID})
}
