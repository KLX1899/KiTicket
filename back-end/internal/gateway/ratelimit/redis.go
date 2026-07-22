// Package ratelimit implements an atomic Redis token bucket for multi-instance gateways.
package ratelimit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const script = `
local now_parts = redis.call('TIME')
local now = (now_parts[1] * 1000) + math.floor(now_parts[2] / 1000)
local values = redis.call('HMGET', KEYS[1], 'tokens', 'last')
local tokens = tonumber(values[1]) or tonumber(ARGV[1])
local last = tonumber(values[2]) or now
local elapsed = math.max(0, now - last)
tokens = math.min(tonumber(ARGV[1]), tokens + (elapsed * tonumber(ARGV[2]) / 1000))
local allowed = 0
local retry_ms = 0
if tokens >= 1 then
  allowed = 1
  tokens = tokens - 1
else
  retry_ms = math.ceil((1 - tokens) * 1000 / tonumber(ARGV[2]))
end
redis.call('HSET', KEYS[1], 'tokens', tokens, 'last', now)
redis.call('PEXPIRE', KEYS[1], ARGV[3])
return {allowed, retry_ms}
`

type Limiter struct {
	client   redis.UniversalClient
	prefix   string
	capacity int
	rate     float64
	ttl      time.Duration
	script   *redis.Script
}

func New(client redis.UniversalClient, capacity int, ratePerSecond float64) (*Limiter, error) {
	if client == nil || capacity < 1 || ratePerSecond <= 0 {
		return nil, errors.New("rate limiter requires Redis, positive capacity, and positive refill rate")
	}
	ttl := time.Duration(float64(capacity)/ratePerSecond*2*float64(time.Second)) + time.Second
	return &Limiter{client: client, prefix: "kiticket:gateway:rate", capacity: capacity, rate: ratePerSecond, ttl: ttl, script: redis.NewScript(script)}, nil
}

func (l *Limiter) Allow(ctx context.Context, identity string) (bool, time.Duration, error) {
	hash := sha256.Sum256([]byte(identity))
	key := l.prefix + ":" + hex.EncodeToString(hash[:16])
	result, err := l.script.Run(ctx, l.client, []string{key}, l.capacity, l.rate, l.ttl.Milliseconds()).Slice()
	if err != nil {
		return false, 0, fmt.Errorf("evaluate gateway rate limit: %w", err)
	}
	if len(result) != 2 {
		return false, 0, errors.New("rate-limit script returned invalid data")
	}
	allowed, ok := result[0].(int64)
	if !ok {
		return false, 0, errors.New("rate-limit script returned invalid decision")
	}
	retryMillis, ok := result[1].(int64)
	if !ok {
		return false, 0, errors.New("rate-limit script returned invalid retry duration")
	}
	return allowed == 1, time.Duration(retryMillis) * time.Millisecond, nil
}
