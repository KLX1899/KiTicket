// Package redislock implements atomic, cluster-safe transient reservation locks.
package redislock

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/KLX1899/KiTicket/internal/reservation/application"
	"github.com/KLX1899/KiTicket/internal/reservation/domain"
	"github.com/redis/go-redis/v9"
)

const acquireScript = `
local existing = redis.call('GET', KEYS[1])
if existing then
  local saved = cjson.decode(existing)
  if saved.request_hash ~= ARGV[1] then
    return redis.error_reply('IDEMPOTENCY_MISMATCH')
  end
  return {2, saved.reservation_id, tostring(saved.fence), tostring(saved.expires_at)}
end

for i = 3, #KEYS do
  if redis.call('EXISTS', KEYS[i]) == 1 then
    return {0, ARGV[i + 3]}
  end
end

local fence = redis.call('INCR', KEYS[2])
local server_time = redis.call('TIME')
local now_ms = (server_time[1] * 1000) + math.floor(server_time[2] / 1000)
local expires_at = now_ms + tonumber(ARGV[4])
local lock_value = cjson.encode({
  reservation_id = ARGV[2],
  owner_id = ARGV[3],
  fence = fence
})

for i = 3, #KEYS do
  redis.call('SET', KEYS[i], lock_value, 'PX', ARGV[4])
end

local saved = cjson.encode({
  request_hash = ARGV[1],
  reservation_id = ARGV[2],
  fence = fence,
  expires_at = expires_at
})
redis.call('SET', KEYS[1], saved, 'PX', ARGV[5])
return {1, ARGV[2], tostring(fence), tostring(expires_at)}
`

const validateScript = `
local minimum_ttl = nil
for i = 1, #KEYS do
  local raw = redis.call('GET', KEYS[i])
  if not raw then
    return 0
  end
  local lock = cjson.decode(raw)
  if lock.reservation_id ~= ARGV[1] or lock.owner_id ~= ARGV[2] or tostring(lock.fence) ~= ARGV[3] then
    return 0
  end
  local ttl = redis.call('PTTL', KEYS[i])
  if ttl <= 0 then
    return 0
  end
  if not minimum_ttl or ttl < minimum_ttl then
    minimum_ttl = ttl
  end
end
return minimum_ttl
`

const releaseScript = `
for i = 1, #KEYS do
  local raw = redis.call('GET', KEYS[i])
  if not raw then
    return 0
  end
  local lock = cjson.decode(raw)
  if lock.reservation_id ~= ARGV[1] or lock.owner_id ~= ARGV[2] or tostring(lock.fence) ~= ARGV[3] then
    return 0
  end
end
redis.call('DEL', unpack(KEYS))
return 1
`

type Store struct {
	client               redis.UniversalClient
	prefix               string
	idempotencyRetention time.Duration
	acquire              *redis.Script
	validate             *redis.Script
	release              *redis.Script
}

func New(client redis.UniversalClient, prefix string, idempotencyRetention time.Duration) (*Store, error) {
	if client == nil || idempotencyRetention < domain.MaximumTTL {
		return nil, errors.New("Redis lock store requires a client and idempotency retention at least as long as maximum lock TTL")
	}
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = "kiticket:reservation"
	}
	return &Store{
		client:               client,
		prefix:               prefix,
		idempotencyRetention: idempotencyRetention,
		acquire:              redis.NewScript(acquireScript),
		validate:             redis.NewScript(validateScript),
		release:              redis.NewScript(releaseScript),
	}, nil
}

func NewClient(rawURL string) (*redis.Client, error) {
	options, err := redis.ParseURL(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse Redis URL: %w", err)
	}
	options.MaxRetries = 2
	options.MinRetryBackoff = 10 * time.Millisecond
	options.MaxRetryBackoff = 100 * time.Millisecond
	options.DialTimeout = 2 * time.Second
	options.ReadTimeout = 2 * time.Second
	options.WriteTimeout = 2 * time.Second
	return redis.NewClient(options), nil
}

func (s *Store) Acquire(ctx context.Context, command application.LockCommand) (domain.Lock, error) {
	keys := []string{s.idempotencyKey(command.ScheduleID, command.OwnerID, command.IdempotencyKey), s.fenceKey(command.ScheduleID)}
	arguments := []any{command.RequestHash, command.ReservationID, command.OwnerID, command.TTLMillis, s.idempotencyRetention.Milliseconds()}
	for _, seatID := range command.SeatIDs {
		keys = append(keys, s.lockKey(command.ScheduleID, seatID))
		arguments = append(arguments, seatID)
	}
	result, err := s.acquire.Run(ctx, s.client, keys, arguments...).Slice()
	if err != nil {
		if strings.Contains(err.Error(), "IDEMPOTENCY_MISMATCH") {
			return domain.Lock{}, domain.ErrIdempotencyMismatch
		}
		return domain.Lock{}, fmt.Errorf("acquire Redis reservation lock: %w", err)
	}
	if len(result) < 2 {
		return domain.Lock{}, errors.New("Redis reservation script returned an invalid result")
	}
	code, err := asInt64(result[0])
	if err != nil {
		return domain.Lock{}, fmt.Errorf("decode Redis reservation result: %w", err)
	}
	if code == 0 {
		seatID, _ := result[1].(string)
		return domain.Lock{}, &domain.SeatConflictError{SeatID: seatID}
	}
	if len(result) != 4 || (code != 1 && code != 2) {
		return domain.Lock{}, errors.New("Redis reservation script returned an invalid success result")
	}
	reservationID, ok := result[1].(string)
	if !ok {
		return domain.Lock{}, errors.New("Redis reservation script returned an invalid reservation ID")
	}
	fence, err := asInt64(result[2])
	if err != nil {
		return domain.Lock{}, fmt.Errorf("decode reservation fence: %w", err)
	}
	expiresAt, err := asInt64(result[3])
	if err != nil {
		return domain.Lock{}, fmt.Errorf("decode reservation expiry: %w", err)
	}
	return domain.Lock{
		ReservationID: reservationID,
		ScheduleID:    command.ScheduleID,
		OwnerID:       command.OwnerID,
		SeatIDs:       append([]string(nil), command.SeatIDs...),
		Fence:         fence,
		ExpiresAt:     time.UnixMilli(expiresAt).UTC(),
		Replayed:      code == 2,
	}, nil
}

func (s *Store) Validate(ctx context.Context, request domain.ReleaseRequest) error {
	keys := s.lockKeys(request.ScheduleID, request.SeatIDs)
	result, err := s.validate.Run(ctx, s.client, keys, request.ReservationID, request.OwnerID, request.Fence).Int64()
	if err != nil {
		return fmt.Errorf("validate Redis reservation lock: %w", err)
	}
	if result <= 0 {
		return domain.ErrLockLost
	}
	return nil
}

func (s *Store) Release(ctx context.Context, request domain.ReleaseRequest) error {
	keys := s.lockKeys(request.ScheduleID, request.SeatIDs)
	result, err := s.release.Run(ctx, s.client, keys, request.ReservationID, request.OwnerID, request.Fence).Int64()
	if err != nil {
		return fmt.Errorf("release Redis reservation lock: %w", err)
	}
	if result != 1 {
		return domain.ErrLockLost
	}
	return nil
}

func (s *Store) lockKeys(scheduleID string, seats []string) []string {
	keys := make([]string, 0, len(seats))
	for _, seatID := range seats {
		keys = append(keys, s.lockKey(scheduleID, seatID))
	}
	return keys
}

func (s *Store) lockKey(scheduleID, seatID string) string {
	return s.scope(scheduleID) + ":lock:" + digest(seatID)
}

func (s *Store) fenceKey(scheduleID string) string { return s.scope(scheduleID) + ":fence" }

func (s *Store) idempotencyKey(scheduleID, ownerID, key string) string {
	return s.scope(scheduleID) + ":idem:" + digest(ownerID+"\x00"+key)
}

func (s *Store) scope(scheduleID string) string {
	// The hash tag keeps all keys for one schedule in the same Redis Cluster slot.
	return s.prefix + ":{" + digest(scheduleID) + "}"
}

func digest(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:16])
}

func asInt64(value any) (int64, error) {
	switch typed := value.(type) {
	case int64:
		return typed, nil
	case string:
		return strconv.ParseInt(typed, 10, 64)
	case []byte:
		return strconv.ParseInt(string(typed), 10, 64)
	default:
		return 0, fmt.Errorf("unexpected type %T", value)
	}
}
