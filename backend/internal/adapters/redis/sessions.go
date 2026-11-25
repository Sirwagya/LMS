package redisadapter

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"vedam/lms/internal/domain/errs"
	"vedam/lms/internal/domain/iam"
)

type Session struct {
	UserID string
	Role   iam.Role
}
type Sessions struct {
	client *redis.Client
	ttl    time.Duration
}

func NewSessions(client *redis.Client, ttl time.Duration) *Sessions { return &Sessions{client, ttl} }

func digest(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

var createSession = redis.NewScript(`
redis.call('HSET', KEYS[1], 'user_id', ARGV[1], 'role', ARGV[2], 'expires_ms', ARGV[3])
redis.call('PEXPIRE', KEYS[1], ARGV[4])
local members = redis.call('SMEMBERS', KEYS[2])
for _, id in ipairs(members) do
 if redis.call('EXISTS', 'sess:' .. id) == 0 then redis.call('SREM', KEYS[2], id) end
end
redis.call('SADD', KEYS[2], ARGV[5])
if redis.call('PTTL', KEYS[2]) < tonumber(ARGV[4]) then redis.call('PEXPIRE', KEYS[2], ARGV[4]) end
return 1
`)

func (s *Sessions) Create(ctx context.Context, userID string, role iam.Role) (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	token := hex.EncodeToString(raw[:])
	id := digest(token)
	err := createSession.Run(ctx, s.client, []string{"sess:" + id, "user_sess:" + userID}, userID, string(role), time.Now().Add(s.ttl).UnixMilli(), s.ttl.Milliseconds(), id).Err()
	if err != nil {
		return "", err
	}
	return token, nil
}

func (s *Sessions) Lookup(ctx context.Context, token string) (Session, error) {
	if len(token) != 64 {
		return Session{}, errs.ErrUnauthorized
	}
	result, err := s.client.HGetAll(ctx, "sess:"+digest(token)).Result()
	if err != nil {
		return Session{}, err
	}
	expiry, _ := strconv.ParseInt(result["expires_ms"], 10, 64)
	if expiry <= time.Now().UnixMilli() || result["user_id"] == "" {
		return Session{}, errs.ErrUnauthorized
	}
	return Session{result["user_id"], iam.Role(result["role"])}, nil
}

var refreshSession = redis.NewScript(`
local session = redis.call('HMGET', KEYS[1], 'user_id', 'role', 'expires_ms')
local t = redis.call('TIME')
local now = t[1] * 1000 + math.floor(t[2]/1000)
if not session[1] or not session[2] or tonumber(session[3] or '0') <= now then return {} end
local ttl = tonumber(ARGV[1])
local index = 'user_sess:' .. session[1]
redis.call('HSET', KEYS[1], 'expires_ms', now + ttl)
redis.call('PEXPIRE', KEYS[1], ttl)
redis.call('SADD', index, ARGV[2])
if redis.call('PTTL', index) < ttl then redis.call('PEXPIRE', index, ttl) end
return {session[1], session[2]}
`)

func (s *Sessions) Refresh(ctx context.Context, token string) (Session, error) {
	if len(token) != 64 {
		return Session{}, errs.ErrUnauthorized
	}
	id := digest(token)
	fields, err := refreshSession.Run(ctx, s.client, []string{"sess:" + id}, s.ttl.Milliseconds(), id).StringSlice()
	if err != nil {
		return Session{}, err
	}
	if len(fields) != 2 {
		return Session{}, errs.ErrUnauthorized
	}
	return Session{fields[0], iam.Role(fields[1])}, nil
}

var revokeSessions = redis.NewScript(`
for _, id in ipairs(redis.call('SMEMBERS', KEYS[1])) do redis.call('DEL', 'sess:' .. id) end
return redis.call('DEL', KEYS[1])
`)

func (s *Sessions) RevokeAll(ctx context.Context, userID string) error {
	return revokeSessions.Run(ctx, s.client, []string{"user_sess:" + userID}).Err()
}
