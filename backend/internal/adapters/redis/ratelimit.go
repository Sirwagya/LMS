package redisadapter

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

var limitWindow = redis.NewScript(`
local t = redis.call('TIME')
local now = t[1] * 1000 + math.floor(t[2]/1000)
local window = tonumber(ARGV[1])
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', now-window)
if redis.call('ZCARD', KEYS[1]) >= tonumber(ARGV[2]) then
 local oldest = redis.call('ZRANGE', KEYS[1], 0, 0, 'WITHSCORES')
 return math.max(1, math.ceil((oldest[2]+window-now)/1000))
end
redis.call('ZADD', KEYS[1], now, ARGV[3])
redis.call('PEXPIRE', KEYS[1], window)
return 0
`)

func RateLimit(ctx context.Context, client *redis.Client, bucket, actor string, limit int) (int, error) {
	return limitWindow.Run(ctx, client, []string{"rl:" + bucket + ":" + actor}, 60000, limit, uuid.NewString()).Int()
}

var releaseJudge = redis.NewScript(`
if redis.call('GET', KEYS[1]) == ARGV[1] then return redis.call('DEL', KEYS[1]) end
return 0
`)

func LockJudge(ctx context.Context, client *redis.Client, user string) (func(), bool, error) {
	key, token := "lock:judge:"+user, uuid.NewString()
	got, err := client.SetNX(ctx, key, token, 30*time.Second).Result()
	unlock := func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer cancel()
		_ = releaseJudge.Run(cleanup, client, []string{key}, token).Err()
	}
	return unlock, got, err
}
