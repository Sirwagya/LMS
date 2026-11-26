package httpadapter

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	redisadapter "vedam/lms/internal/adapters/redis"
)

type streamSlots struct {
	mu    sync.Mutex
	total int
	users map[string]int
}

func (s *streamSlots) acquire(user string) (func(), bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.total >= 128 || s.users[user] >= 4 {
		return nil, false
	}
	if s.users == nil {
		s.users = make(map[string]int)
	}
	s.users[user]++
	s.total++
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.total--
		s.users[user]--
		if s.users[user] == 0 {
			delete(s.users, user)
		}
	}, true
}

func (a *api) events(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	session := actor(r)
	release, ok := a.streams.acquire(session.UserID)
	if !ok {
		w.Header().Set("Retry-After", "5")
		return problem(429, "Too many open event streams.")
	}
	defer release()
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
