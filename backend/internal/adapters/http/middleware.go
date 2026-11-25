package httpadapter

import (
	"context"
	"errors"
	"mime"
	"net"
	"net/http"
	"net/url"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	redisadapter "vedam/lms/internal/adapters/redis"
	"vedam/lms/internal/domain/errs"
	"vedam/lms/internal/domain/iam"
)

type requestKey int

const (
	requestIDKey requestKey = iota
	stateKey
	mutationKey
)

type requestState struct {
	actor redisadapter.Session
	token string
}

func actor(r *http.Request) redisadapter.Session {
	return r.Context().Value(stateKey).(*requestState).actor
}
func (a *api) requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" || len(id) > 128 {
			id = uuid.NewString()
		}
		w.Header().Set("X-Request-ID", id)
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(ctx, stateKey, &requestState{})))
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *statusWriter) WriteHeader(code int) {
	if w.status != 0 {
		return
	}
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}
func (w *statusWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(200)
	}
	return w.ResponseWriter.Write(data)
}
func (a *api) logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorded := &statusWriter{ResponseWriter: w}
		defer func() {
			status := recorded.status
			if status == 0 {
				status = 200
			}
			a.Log.InfoContext(r.Context(), "http request", "method", r.Method, "path", r.URL.Path, "status", status,
				"duration_ms", time.Since(start).Milliseconds(), "request_id", r.Context().Value(requestIDKey), "user_id", actor(r).UserID)
		}()
		next.ServeHTTP(recorded, r)
	})
}
func (a *api) recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				a.Log.ErrorContext(r.Context(), "handler panic", "panic", v, "request_id", r.Context().Value(requestIDKey), "stack", string(debug.Stack()))
				if w.(*statusWriter).status == 0 {
					a.failure(w, r, problem(500, "Internal server error."))
				}
			}
		}()
		next.ServeHTTP(w, r)
	})
}
func (a *api) transport(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if origin := r.Header.Get("Origin"); origin != "" && !allowedOrigin(r, origin) {
				a.failure(w, r, problem(403, "Invalid request origin."))
				return
			}
			media, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if media != "application/json" {
				a.failure(w, r, problem(415, "JSON content type required."))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
func allowedOrigin(r *http.Request, origin string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	if (u.Scheme == "https" || u.Scheme == "http") && strings.EqualFold(u.Host, r.Host) {
		return true
	}
	switch u.Hostname() {
	case "localhost", "127.0.0.1", "[::1]", "::1":
		return u.Scheme == "http"
	default:
		return false
	}
}
func (a *api) authenticate(policy access, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if policy == public {
			next.ServeHTTP(w, r)
			return
		}
		token := ""
		fromCookie := false
		if auth := r.Header.Get("Authorization"); auth != "" {
			scheme, value, ok := strings.Cut(auth, " ")
			if ok && strings.EqualFold(scheme, "Bearer") {
				token = value
			}
		} else if cookie, err := r.Cookie("lms_session"); err == nil {
			token = cookie.Value
			fromCookie = true
		}
		lookup := a.Sessions.Lookup
		if a.SessionSliding {
			lookup = a.Sessions.Refresh
		}
		session, err := lookup(r.Context(), token)
		if err != nil {
			if !errors.Is(err, errs.ErrUnauthorized) {
				err = problem(503, "Session service unavailable.")
			}
			a.failure(w, r, err)
			return
		}
		state := r.Context().Value(stateKey).(*requestState)
		state.actor, state.token = session, token
		if a.SessionSliding && fromCookie && !strings.HasSuffix(r.URL.Path, "/logout") && !strings.HasSuffix(r.URL.Path, "/password") {
			a.cookie(w, token, int(a.SessionTTL.Seconds()))
		}
		next.ServeHTTP(w, r)
	})
}
func (a *api) role(policy access, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		role := actor(r).Role
		var message string
		switch {
		case policy == student && role != iam.RoleStudent:
			message = "Student access required."
		case policy == staffOnly && role != iam.RoleTeacher && role != iam.RoleAdmin:
			message = "Staff access required."
		case policy == admin && role != iam.RoleAdmin:
			message = "Administrator access required."
			if role == iam.RoleStudent {
				message = "Staff access required."
			}
		}
		if message != "" {
			a.failure(w, r, problem(403, message))
			return
		}
		next.ServeHTTP(w, r)
	})
}
func (a *api) rate(path string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var bucket, subject, message string
		var maximum int
		switch path {
		case "/login", "/register":
			bucket, message, maximum = "auth", "Too many attempts. Try again in a minute.", 30
			subject, _, _ = net.SplitHostPort(r.RemoteAddr)
			if subject == "" {
				subject = r.RemoteAddr
			}
		case "/coding/evaluate":
			if actor(r).Role != iam.RoleStudent {
				next.ServeHTTP(w, r)
				return
			}
			bucket, subject, message, maximum = "coding", actor(r).UserID, "Run limit reached. Try again in a minute.", 20
		}
		if maximum != 0 {
			retry, err := redisadapter.RateLimit(r.Context(), a.Redis, bucket, subject, maximum)
			if err != nil {
				a.failure(w, r, problem(503, "Rate limit service unavailable."))
				return
			}
			if retry > 0 {
				w.Header().Set("Retry-After", strconv.Itoa(retry))
				a.failure(w, r, problem(429, message))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
func (a *api) idempotency(path string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if path != "/payments" && path != "/sync/push" {
			next.ServeHTTP(w, r)
			return
		}
		key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		if len(key) > 200 {
			a.failure(w, r, invalid("Idempotency-Key must be at most 200 characters."))
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), mutationKey, key)))
	})
}
