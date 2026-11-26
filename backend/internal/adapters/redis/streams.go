package redisadapter

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

const gradingStream = "stream:grading"
const graders = "graders"

type GradingJob struct {
	ID           string `json:"id"`
	UserID       string `json:"userId"`
	AssignmentID string `json:"assignmentId"`
	Language     string `json:"language"`
	Source       string `json:"source"`
}
type JobState struct {
	Status string          `json:"status"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}
type Queue struct{ client *redis.Client }

func NewQueue(client *redis.Client) *Queue { return &Queue{client} }

var ErrQueueFull = errors.New("Grading queue is full. Try again shortly.")
var ErrJobPending = errors.New("Wait for your current execution to finish.")

func jobKey(user, id string) string { return "grading:job:" + user + ":" + id }
func pendingKey(user string) string { return "grading:pending:" + user }

var enqueueJob = redis.NewScript(`
if redis.call('EXISTS',KEYS[3]) == 1 then return 2 end
if redis.call('XLEN',KEYS[1]) >= 5000 then return 0 end
redis.call('XADD',KEYS[1],'*','job',ARGV[1])
redis.call('SET',KEYS[2],ARGV[2],'EX',86400)
redis.call('SET',KEYS[3],ARGV[3],'EX',86400)
return 1
`)

func (q *Queue) Enqueue(ctx context.Context, job GradingJob) error {
	raw, err := json.Marshal(job)
	if err != nil {
		return err
	}
	code, err := enqueueJob.Run(ctx, q.client, []string{gradingStream, jobKey(job.UserID, job.ID), pendingKey(job.UserID)}, raw, `{"status":"queued"}`, job.ID).Int()
	if err != nil {
		return err
	}
	switch code {
	case 0:
		return ErrQueueFull
	case 2:
		return ErrJobPending
	default:
		return nil
	}
}
func (q *Queue) Status(ctx context.Context, user, id string) (JobState, error) {
	raw, err := q.client.Get(ctx, jobKey(user, id)).Bytes()
	if err != nil {
		return JobState{}, err
	}
	var result JobState
	err = json.Unmarshal(raw, &result)
	return result, err
}

var finishJob = redis.NewScript(`
redis.call('SET',KEYS[2],ARGV[2],'EX',86400)
if redis.call('GET',KEYS[3]) == ARGV[3] then redis.call('DEL',KEYS[3]) end
redis.call('XACK',KEYS[1],ARGV[1],ARGV[4])
return redis.call('XDEL',KEYS[1],ARGV[4])
`)

func (q *Queue) Finish(ctx context.Context, messageID string, job GradingJob, state JobState) error {
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return finishJob.Run(ctx, q.client, []string{gradingStream, jobKey(job.UserID, job.ID), pendingKey(job.UserID)}, graders, raw, job.ID, messageID).Err()
}
func (q *Queue) Initialize(ctx context.Context) error {
	err := q.client.XGroupCreateMkStream(ctx, gradingStream, graders, "0").Err()
	if redis.HasErrorPrefix(err, "BUSYGROUP") {
		return nil
	}
	return err
}
func (q *Queue) Next(ctx context.Context, consumer string) (redis.XMessage, error) {
	claimed, _, err := q.client.XAutoClaim(ctx, &redis.XAutoClaimArgs{Stream: gradingStream, Group: graders, Consumer: consumer, MinIdle: time.Minute, Start: "0-0", Count: 1}).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return redis.XMessage{}, err
	}
	if len(claimed) > 0 {
		return claimed[0], nil
	}
	streams, err := q.client.XReadGroup(ctx, &redis.XReadGroupArgs{Group: graders, Consumer: consumer, Streams: []string{gradingStream, ">"}, Count: 1, Block: time.Second}).Result()
	if err != nil {
		return redis.XMessage{}, err
	}
	if len(streams) == 0 || len(streams[0].Messages) == 0 {
		return redis.XMessage{}, redis.Nil
	}
	return streams[0].Messages[0], nil
}
func (q *Queue) Drop(ctx context.Context, id string) error {
	_, err := q.client.TxPipelined(ctx, func(p redis.Pipeliner) error {
		p.XAck(ctx, gradingStream, graders, id)
		p.XDel(ctx, gradingStream, id)
		return nil
	})
	return err
}

func (q *Queue) Deliveries(ctx context.Context, id string) (int64, error) {
	rows, err := q.client.XPendingExt(ctx, &redis.XPendingExtArgs{Stream: gradingStream, Group: graders, Start: id, End: id, Count: 1}).Result()
	if err != nil || len(rows) == 0 {
		return 0, err
	}
	return rows[0].RetryCount, nil
}
