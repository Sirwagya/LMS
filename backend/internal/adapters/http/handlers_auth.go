package httpadapter

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"vedam/lms/internal/domain/errs"
	"vedam/lms/internal/domain/iam"
)

var (
	emailPattern = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)
	profileImage = regexp.MustCompile(`^data:image/(png|jpeg|webp|gif);base64,`)
)

func (a *api) health(w http.ResponseWriter, r *http.Request) error {
	status := 200
	db, redis := "up", "up"
	if a.PingDB(r.Context()) != nil {
		db, status = "down", 503
	}
	if a.Redis.Ping(r.Context()).Err() != nil {
		redis, status = "down", 503
	}
	writeJSON(w, status, map[string]any{"ok": status == 200, "db": db, "redis": redis})
	return nil
}
func (a *api) cookie(w http.ResponseWriter, token string, age int) {
	http.SetCookie(w, &http.Cookie{Name: "lms_session", Value: token, Path: "/", MaxAge: age, HttpOnly: true, Secure: a.CookieSecure, SameSite: http.SameSiteStrictMode})
}
func (a *api) createSession(ctx context.Context, u *iam.User) (string, error) {
	token, err := a.Sessions.Create(ctx, u.ID, u.Role)
	if err != nil {
		return "", problem(503, "Session service unavailable.")
	}
	return token, nil
}

func nativeAuth(r *http.Request) bool {
	return strings.HasPrefix(r.URL.Path, "/api/v1/") && r.Header.Get("X-LMS-Auth") == "bearer" && nativeRequest(r)
}

func nativeRequest(r *http.Request) bool {
	return r.Header.Get("Origin") == "" && r.Header.Get("Cookie") == "" &&
		r.Header.Get("Sec-Fetch-Site") == "" && r.Header.Get("Sec-Fetch-Mode") == ""
}

func (a *api) passwordSlot(w http.ResponseWriter) (func(), error) {
	select {
	case a.passwords <- struct{}{}:
		return func() { <-a.passwords }, nil
	default:
		w.Header().Set("Retry-After", "1")
		return nil, problem(429, "Authentication is busy. Try again shortly.")
	}
}

func (a *api) signedIn(w http.ResponseWriter, u *iam.User, token string, includeToken bool) error {
	response := map[string]any{"user": u.Public()}
	if includeToken {
		response["token"] = token
	} else {
		a.cookie(w, token, int(a.SessionTTL.Seconds()))
	}
	writeJSON(w, 200, response)
	return nil
}
func demoEmail(email string) bool {
	switch email {
	case "student@vedam.local", "teacher@vedam.local", "admin@vedam.local":
		return true
	default:
		return false
	}
}

func (a *api) login(w http.ResponseWriter, r *http.Request) error {
	f := readBody(w, r, 4096)
	email, password := strings.ToLower(f.text("email", "email", 254)), f.text("password", "password", 128)
	if f.err != nil {
		return f.err
	}
	if err := a.limit(w, r, "login-account", hashKey(email), "Too many attempts. Try again in a minute.", 10); err != nil {
		return err
	}
	release, err := a.passwordSlot(w)
	if err != nil {
		return err
	}
	defer release()

	ctx := r.Context()
	u, err := a.Users.GetByEmail(ctx, email)
	badLogin := problem(401, "Email or password is incorrect.")
	if errors.Is(err, errs.ErrNotFound) || (a.Production && demoEmail(email)) {
		_, _, _ = a.Hasher.Verify(password, "$argon2id$v=19$m=65536,t=3,p=2$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
		return badLogin
	}
	if err != nil {
		return err
	}
	valid, rehash, err := a.Hasher.Verify(password, u.PasswordHash)
	if err != nil {
		return err
	}
	if !valid {
		return badLogin
	}
	replacement := u.PasswordHash
	if rehash {
		replacement, err = a.Hasher.Hash(password)
		if err != nil {
			return err
		}
	}
	// Check the password again under a row lock before issuing the session. A
	// concurrent password change can then revoke every session using the old hash.
	var token string
	if err := a.withCurrentPassword(ctx, u, replacement, func() error {
		token, err = a.createSession(ctx, u)
		return err
	}, badLogin); err != nil {
		return err
	}
	return a.signedIn(w, u, token, nativeAuth(r))
}
func (a *api) withCurrentPassword(ctx context.Context, u *iam.User, replacement string, fn func() error, mismatch error) error {
	return a.Tx.WithinTx(ctx, func(ctx context.Context) error {
		current, err := a.Users.UpdatePasswordIfCurrent(ctx, u.ID, u.PasswordHash, replacement)
		if err != nil {
			return err
		}
		if !current {
			return mismatch
		}
		return fn()
	})
}
func (a *api) newUser(ctx context.Context, name, email, password string, role iam.Role) (*iam.User, error) {
	if a.Production && demoEmail(email) {
		return nil, invalid("This email address is reserved.")
	}
	hash, err := a.Hasher.Hash(password)
	if err != nil {
		return nil, err
	}
	user := &iam.User{ID: uuid.NewString(), Name: name, Email: email, Role: role, PasswordHash: hash, Version: 1}
	if err := a.Users.Create(ctx, user); err != nil {
		if errors.Is(err, errs.ErrConflict) {
			err = problem(409, "Email already registered.")
		}
		return nil, err
	}
	return user, nil
}
func (a *api) register(w http.ResponseWriter, r *http.Request) error {
	f := readBody(w, r, 4096)
	name, email, password := f.text("name", "name", 100), strings.ToLower(f.text("email", "email", 254)), f.text("password", "password", 128)
	if f.err != nil {
		return f.err
	}
	if !emailPattern.MatchString(email) || textLength(password) < 10 {
		return invalid("Use a valid email and a password with at least 10 characters.")
	}
	release, err := a.passwordSlot(w)
	if err != nil {
		return err
	}
	defer release()

	user, err := a.newUser(r.Context(), name, email, password, iam.RoleStudent)
	if err != nil {
		return err
	}
	token, err := a.createSession(r.Context(), user)
	if err != nil {
		return err
	}
	return a.signedIn(w, user, token, nativeAuth(r))
}
func (a *api) logout(w http.ResponseWriter, r *http.Request) error {
	if err := a.Sessions.RevokeAll(r.Context(), actor(r).UserID); err != nil {
		return problem(503, "Session service unavailable.")
	}
	a.cookie(w, "", -1)
	return ok(w, 200, nil)
}
func (a *api) revokeSession(w http.ResponseWriter, r *http.Request) error {
	scheme, _, _ := strings.Cut(r.Header.Get("Authorization"), " ")
	if !strings.EqualFold(scheme, "Bearer") {
		return problem(401, "Bearer authentication required.")
	}
	state := r.Context().Value(stateKey).(*requestState)
	if err := a.Sessions.Revoke(r.Context(), state.actor.UserID, state.token); err != nil {
		return problem(503, "Session service unavailable.")
	}
	return ok(w, 200, nil)
}
func (a *api) password(w http.ResponseWriter, r *http.Request) error {
	f := readBody(w, r, 4096)
	current, next := f.text("current", "current password", 128), f.text("next", "new password", 128)
	if f.err != nil {
		return f.err
	}
	release, err := a.passwordSlot(w)
	if err != nil {
		return err
	}
	defer release()

	ctx := r.Context()
	u, err := a.Users.GetByID(ctx, actor(r).UserID)
	if err != nil {
		return err
	}
	valid, _, err := a.Hasher.Verify(current, u.PasswordHash)
	if err != nil {
		return err
	}
	mismatch := invalid("Current password is incorrect.")
	if !valid {
		return mismatch
	}
	if textLength(next) < 10 {
		return invalid("Use at least 10 characters.")
	}
	replacement, err := a.Hasher.Hash(next)
	if err != nil {
		return err
	}
	var token string
	if err := a.withCurrentPassword(ctx, u, replacement, func() error {
		if err := a.Sessions.RevokeAll(ctx, u.ID); err != nil {
			return problem(503, "Session service unavailable.")
		}
		token, err = a.createSession(ctx, u)
		return err
	}, mismatch); err != nil {
		return err
	}
	return a.signedIn(w, u, token, nativeAuth(r) && strings.HasPrefix(r.Header.Get("Authorization"), "Bearer "))
}
func validateProfile(raw json.RawMessage) (string, error) {
	f := &fields{}
	if json.Unmarshal(raw, &f.values) != nil || f.values == nil {
		return "", invalid("Profile required.")
	}
	name := f.text("name", "name", 100)
	if f.err != nil {
		return "", f.err
	}
	for _, key := range []string{"avatarUrl", "bannerUrl"} {
		var value string
		image := f.values[key]
		if len(image) == 0 {
			continue
		}
		if json.Unmarshal(image, &value) != nil {
			return "", invalid("Invalid profile image.")
		}
		if value == "" {
			continue
		}
		prefix := profileImage.FindStringIndex(value)
		if prefix == nil || strings.ContainsAny(value, "\r\n") || len(value) > base64.StdEncoding.EncodedLen(5<<20)+64 {
			return "", invalid("Upload a PNG, JPEG, WebP or GIF image up to 5 MB.")
		}
		if data, err := base64.StdEncoding.Strict().DecodeString(value[prefix[1]:]); err != nil || len(data) == 0 || len(data) > 5<<20 {
			return "", invalid("Invalid profile image encoding.")
		}
	}
	var platforms []struct {
		URL string `json:"url"`
	}
	if raw := f.values["platforms"]; len(raw) > 0 && json.Unmarshal(raw, &platforms) != nil {
		return "", invalid("Invalid profile platforms.")
	}
	if len(platforms) > 32 {
		return "", invalid("Too many profile platforms.")
	}
	for _, p := range platforms {
		u, err := url.Parse(p.URL)
		if p.URL != "" && (len(p.URL) > 2048 || (!strings.HasPrefix(p.URL, "https://") && !strings.HasPrefix(p.URL, "http://")) || err != nil || u.Hostname() == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https")) {
			return "", invalid("Profile links must begin with https:// or http://.")
		}
	}
	return name, nil
}
func (a *api) profile(w http.ResponseWriter, r *http.Request) error {
	f := read(w, r)
	if f.err != nil {
		return f.err
	}
	raw := f.values["profile"]
	name, err := validateProfile(raw)
	if err != nil {
		return err
	}
	var expected *int64
	if version := f.values["baseVersion"]; len(version) > 0 && json.Unmarshal(version, &expected) != nil {
		return invalid("Invalid base version.")
	}
	user := actor(r).UserID
	var version int64
	err = a.mutate(r.Context(), user, func(ctx context.Context) error {
		var err error
		version, err = a.updateProfile(ctx, user, name, raw, expected)
		return err
	})
	if err != nil {
		return err
	}
	writeJSON(w, 200, map[string]any{"ok": true, "version": version})
	return nil
}
func (a *api) updateProfile(ctx context.Context, user, name string, raw json.RawMessage, expected *int64) (int64, error) {
	version, err := a.Users.UpdateProfile(ctx, user, name, raw, expected)
	if err != nil {
		return 0, err
	}
	return version, a.changed(ctx, user, "profile", user, "sync.available", map[string]any{"id": user, "profile": raw, "version": version})
}
