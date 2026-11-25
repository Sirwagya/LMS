package httpadapter

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	redisadapter "vedam/lms/internal/adapters/redis"
)

func (a *api) events(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	session := actor(r)
	sub := a.Redis.Subscribe(ctx, redisadapter.EventChannel(session.UserID), redisadapter.EventChannel(""))
	defer sub.Close()
	if _, err := sub.Receive(ctx); err != nil {
		return problem(503, "Event service unavailable.")
	}
	messages := sub.Channel()
	conn := http.NewResponseController(w)
	write := func(format string, args ...any) error {
		_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if _, err := fmt.Fprintf(w, format, args...); err != nil {
			return err
		}
		return conn.Flush()
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	if err := write(": connected\n\n"); err != nil {
		return nil
	}
	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()
	token := r.Context().Value(stateKey).(*requestState).token
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if _, err := a.Sessions.Lookup(ctx, token); err != nil {
				return nil
			}
			if err := write(": heartbeat\n\n"); err != nil {
				return nil
			}
		case msg, open := <-messages:
			if !open {
				return nil
			}
			if _, err := a.Sessions.Lookup(ctx, token); err != nil {
				return nil
			}
			var event redisadapter.Event
			if json.Unmarshal([]byte(msg.Payload), &event) != nil || strings.ContainsAny(event.ID+event.Type, "\r\n") {
				continue
			}
			data, err := json.Marshal(event.Data)
			if err != nil {
				continue
			}
			if err := write("id: %s\nevent: %s\ndata: %s\n\n", event.ID, event.Type, data); err != nil {
				return nil
			}
		}
	}
}
