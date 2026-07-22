import { ConflictException, Injectable } from '@nestjs/common';
import { RedisService } from './redis.service';

const ACQUIRE_SCRIPT = `
for i, key in ipairs(KEYS) do
  if redis.call('exists', key) == 1 then return 0 end
end
for i, key in ipairs(KEYS) do
  redis.call('psetex', key, ARGV[2], ARGV[1])
end
return 1`;

const RELEASE_SCRIPT = `
local released = 0
for i, key in ipairs(KEYS) do
  if redis.call('get', key) == ARGV[1] then
    released = released + redis.call('del', key)
  end
end
return released`;

@Injectable()
export class LockService {
  constructor(private readonly redis: RedisService) {}

  key(eventId: string, seatId: string): string {
    return `lock:event:${eventId}:seat:${seatId}`;
  }

  async acquire(eventId: string, seatIds: string[], owner: string): Promise<void> {
    const normalized = [...new Set(seatIds)].sort();
    if (normalized.length !== seatIds.length) throw new ConflictException('Duplicate seats are not allowed');
    const keys = normalized.map((seatId) => this.key(eventId, seatId));
    const ttlMs = Number(process.env.LOCK_TTL_SECONDS ?? 600) * 1000;
    const result = await (await this.redis.ready()).eval(ACQUIRE_SCRIPT, keys.length, ...keys, owner, ttlMs);
    if (Number(result) !== 1) throw new ConflictException('At least one selected seat is currently locked');
  }

  async release(eventId: string, seatIds: string[], owner: string): Promise<number> {
    if (seatIds.length === 0) return 0;
    const keys = [...new Set(seatIds)].sort().map((seatId) => this.key(eventId, seatId));
    return Number(await (await this.redis.ready()).eval(RELEASE_SCRIPT, keys.length, ...keys, owner));
  }
}

export const lockScripts = { ACQUIRE_SCRIPT, RELEASE_SCRIPT };
