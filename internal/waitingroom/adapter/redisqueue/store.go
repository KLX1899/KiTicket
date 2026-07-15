// Package redisqueue maintains fair event queues and replay-safe admissions atomically.
package redisqueue

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const joinScript = `
local now_parts = redis.call('TIME')
local now = (now_parts[1] * 1000) + math.floor(now_parts[2] / 1000)
local existing_id = redis.call('HGET', KEYS[1], ARGV[1])
if existing_id then
  local raw = redis.call('HGET', KEYS[3], existing_id)
  if raw then
    local record = cjson.decode(raw)
    if tonumber(record.expires_at) > now then
      return {2, existing_id, tostring(record.sequence), tostring(record.expires_at)}
    end
  end
  redis.call('ZREM', KEYS[2], existing_id)
  redis.call('ZREM', KEYS[4], existing_id)
  redis.call('HDEL', KEYS[3], existing_id)
end
local sequence = redis.call('INCR', KEYS[5])
local expires_at = now + tonumber(ARGV[4])
local record = cjson.encode({user_id=ARGV[2], user_key=ARGV[1], sequence=sequence, expires_at=expires_at})
redis.call('HSET', KEYS[1], ARGV[1], ARGV[3])
redis.call('HSET', KEYS[3], ARGV[3], record)
redis.call('ZADD', KEYS[2], sequence, ARGV[3])
redis.call('ZADD', KEYS[4], expires_at, ARGV[3])
return {1, ARGV[3], tostring(sequence), tostring(expires_at)}
`

const admitScript = `
local now_parts = redis.call('TIME')
local now = (now_parts[1] * 1000) + math.floor(now_parts[2] / 1000)
local expired = redis.call('ZRANGEBYSCORE', KEYS[3], '-inf', now, 'LIMIT', 0, 1000)
for _, token_id in ipairs(expired) do
  local raw = redis.call('HGET', KEYS[2], token_id)
  if raw then
    local record = cjson.decode(raw)
    redis.call('HDEL', KEYS[1], record.user_key)
  end
  redis.call('ZREM', KEYS[4], token_id)
  redis.call('HDEL', KEYS[2], token_id)
end
if #expired > 0 then redis.call('ZREM', KEYS[3], unpack(expired)) end
local popped = redis.call('ZPOPMIN', KEYS[4], tonumber(ARGV[1]))
local admitted = 0
for i = 1, #popped, 2 do
  local token_id = popped[i]
  local raw = redis.call('HGET', KEYS[2], token_id)
  if raw then
    local record = cjson.decode(raw)
    if tonumber(record.expires_at) > now then
      local admission_expiry = math.min(tonumber(record.expires_at), now + tonumber(ARGV[2]))
      redis.call('HSET', KEYS[5], token_id, admission_expiry)
      admitted = admitted + 1
    end
  end
end
redis.call('PEXPIRE', KEYS[5], tonumber(ARGV[2]) * 2)
return admitted
`

const statusScript = `
local now_parts = redis.call('TIME')
local now = (now_parts[1] * 1000) + math.floor(now_parts[2] / 1000)
local raw = redis.call('HGET', KEYS[1], ARGV[1])
if not raw then return {0} end
local record = cjson.decode(raw)
if record.user_id ~= ARGV[2] or tonumber(record.expires_at) <= now then return {0} end
local admission_expiry = redis.call('HGET', KEYS[3], ARGV[1])
if admission_expiry and tonumber(admission_expiry) > now then
  return {2, '0', tostring(admission_expiry)}
end
local rank = redis.call('ZRANK', KEYS[2], ARGV[1])
if rank then return {1, tostring(rank + 1), tostring(record.expires_at)} end
return {0}
`

const consumeScript = `
local now_parts = redis.call('TIME')
local now = (now_parts[1] * 1000) + math.floor(now_parts[2] / 1000)
local expired = redis.call('ZRANGEBYSCORE', KEYS[3], '-inf', now, 'LIMIT', 0, 1000)
for _, token_id in ipairs(expired) do redis.call('HDEL', KEYS[2], token_id) end
if #expired > 0 then redis.call('ZREM', KEYS[3], unpack(expired)) end
local consumed = redis.call('HGET', KEYS[2], ARGV[1])
if consumed then
  if consumed == ARGV[2] then return 2 end
  return redis.error_reply('ADMISSION_REPLAY')
end
local expiry = redis.call('HGET', KEYS[1], ARGV[1])
if not expiry or tonumber(expiry) <= now then return 0 end
redis.call('HDEL', KEYS[1], ARGV[1])
redis.call('HSET', KEYS[2], ARGV[1], ARGV[2])
redis.call('ZADD', KEYS[3], expiry, ARGV[1])
local retention = math.max(1000, tonumber(expiry) - now)
if redis.call('PTTL', KEYS[2]) < retention then redis.call('PEXPIRE', KEYS[2], retention) end
if redis.call('PTTL', KEYS[3]) < retention then redis.call('PEXPIRE', KEYS[3], retention) end
return 1
`

var (
	ErrNotQueued       = errors.New("queue token is absent or expired")
	ErrAdmissionReplay = errors.New("admission token was replayed for another command")
)

type JoinResult struct {
	TokenID   string
	Sequence  int64
	ExpiresAt time.Time
	Replayed  bool
}

type Status struct {
	State     string
	Position  int64
	ExpiresAt time.Time
}

type Store struct {
	client  redis.UniversalClient
	prefix  string
	join    *redis.Script
	admit   *redis.Script
	status  *redis.Script
	consume *redis.Script
}

func New(client redis.UniversalClient) (*Store, error) {
	if client == nil {
		return nil, errors.New("waiting-room store requires Redis")
	}
	return &Store{client: client, prefix: "kiticket:waiting", join: redis.NewScript(joinScript), admit: redis.NewScript(admitScript), status: redis.NewScript(statusScript), consume: redis.NewScript(consumeScript)}, nil
}

func (s *Store) Join(ctx context.Context, eventID, userID, tokenID string, ttl time.Duration) (JoinResult, error) {
	keys := s.keys(eventID)
	result, err := s.join.Run(ctx, s.client, keys[:5], digest(userID), userID, tokenID, ttl.Milliseconds()).Slice()
	if err != nil {
		return JoinResult{}, fmt.Errorf("join waiting room: %w", err)
	}
	if len(result) != 4 {
		return JoinResult{}, errors.New("waiting-room join returned invalid data")
	}
	code, _ := number(result[0])
	sequence, err := number(result[2])
	if err != nil {
		return JoinResult{}, err
	}
	expires, err := number(result[3])
	if err != nil {
		return JoinResult{}, err
	}
	return JoinResult{TokenID: fmt.Sprint(result[1]), Sequence: sequence, ExpiresAt: time.UnixMilli(expires).UTC(), Replayed: code == 2}, nil
}

func (s *Store) Admit(ctx context.Context, eventID string, limit int, admissionTTL time.Duration) (int, error) {
	keys := s.keys(eventID)
	count, err := s.admit.Run(ctx, s.client, []string{keys[0], keys[2], keys[3], keys[1], keys[5]}, limit, admissionTTL.Milliseconds()).Int()
	if err != nil {
		return 0, fmt.Errorf("admit waiting-room members: %w", err)
	}
	return count, nil
}

func (s *Store) Status(ctx context.Context, eventID, userID, tokenID string) (Status, error) {
	keys := s.keys(eventID)
	result, err := s.status.Run(ctx, s.client, []string{keys[2], keys[1], keys[5]}, tokenID, userID).Slice()
	if err != nil {
		return Status{}, fmt.Errorf("read waiting-room status: %w", err)
	}
	if len(result) == 0 {
		return Status{}, ErrNotQueued
	}
	code, err := number(result[0])
	if err != nil || code == 0 || len(result) != 3 {
		return Status{}, ErrNotQueued
	}
	position, _ := number(result[1])
	expires, _ := number(result[2])
	state := "queued"
	if code == 2 {
		state = "admitted"
	}
	return Status{State: state, Position: position, ExpiresAt: time.UnixMilli(expires).UTC()}, nil
}

func (s *Store) Consume(ctx context.Context, eventID, tokenID, commandHash string) (bool, error) {
	keys := s.keys(eventID)
	result, err := s.consume.Run(ctx, s.client, []string{keys[5], keys[6], keys[7]}, tokenID, commandHash).Int()
	if err != nil {
		if stringsContains(err.Error(), "ADMISSION_REPLAY") {
			return false, ErrAdmissionReplay
		}
		return false, fmt.Errorf("consume admission: %w", err)
	}
	if result == 0 {
		return false, ErrNotQueued
	}
	return result == 2, nil
}

func (s *Store) keys(eventID string) [8]string {
	scope := s.prefix + ":{" + digest(eventID) + "}"
	return [8]string{scope + ":users", scope + ":queue", scope + ":records", scope + ":expiry", scope + ":sequence", scope + ":admitted", scope + ":consumed", scope + ":consumed-expiry"}
}

func digest(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:16])
}

func number(value any) (int64, error) {
	switch typed := value.(type) {
	case int64:
		return typed, nil
	case string:
		return strconv.ParseInt(typed, 10, 64)
	default:
		return 0, fmt.Errorf("unexpected Redis number %T", value)
	}
}

func stringsContains(value, fragment string) bool {
	for index := 0; index+len(fragment) <= len(value); index++ {
		if value[index:index+len(fragment)] == fragment {
			return true
		}
	}
	return false
}
