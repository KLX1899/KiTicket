import { ConflictException } from '@nestjs/common';
import { LockService } from './lock.service';
import { RedisOperations, RedisService } from './redis.service';

class FakeRedis implements RedisOperations {
  status = 'ready';
  readonly values = new Map<string, string>();
  connect = async () => undefined;
  ping = async () => 'PONG';
  quit = async () => 'OK';
  get = async (key: string) => this.values.get(key) ?? null;
  incr = async (key: string) => {
    const value = Number(this.values.get(key) ?? 0) + 1;
    this.values.set(key, String(value));
    return value;
  };

  async eval(script: string, numberOfKeys: number, ...args: (string | number)[]): Promise<number> {
    const keys = args.slice(0, numberOfKeys).map(String);
    const owner = String(args[numberOfKeys]);
    if (script.includes("redis.call('exists'")) {
      if (keys.some((key) => this.values.has(key))) return 0;
      keys.forEach((key) => this.values.set(key, owner));
      return 1;
    }
    let released = 0;
    keys.forEach((key) => {
      if (this.values.get(key) === owner) {
        this.values.delete(key);
        released += 1;
      }
    });
    return released;
  }
}

describe('LockService concurrency invariant', () => {
  let redis: FakeRedis;
  let service: LockService;

  beforeEach(() => {
    redis = new FakeRedis();
    service = new LockService(new RedisService(redis));
  });

  it('allows exactly one winner for one hundred concurrent contenders', async () => {
    const attempts = await Promise.allSettled(Array.from({ length: 100 }, (_, index) => service.acquire('event-1', ['seat-1'], `owner-${index}`)));
    expect(attempts.filter((attempt) => attempt.status === 'fulfilled')).toHaveLength(1);
    expect(attempts.filter((attempt) => attempt.status === 'rejected')).toHaveLength(99);
  });

  it('acquires multiple seats atomically', async () => {
    await service.acquire('event-1', ['seat-2'], 'other');
    await expect(service.acquire('event-1', ['seat-1', 'seat-2'], 'buyer')).rejects.toBeInstanceOf(ConflictException);
    expect(redis.values.has(service.key('event-1', 'seat-1'))).toBe(false);
  });

  it('only releases locks owned by the reservation', async () => {
    await service.acquire('event-1', ['seat-1'], 'buyer');
    expect(await service.release('event-1', ['seat-1'], 'attacker')).toBe(0);
    expect(redis.values.has(service.key('event-1', 'seat-1'))).toBe(true);
    expect(await service.release('event-1', ['seat-1'], 'buyer')).toBe(1);
  });

  it('rejects duplicate seat identifiers before contacting Redis', async () => {
    await expect(service.acquire('event-1', ['seat-1', 'seat-1'], 'buyer')).rejects.toBeInstanceOf(ConflictException);
    expect(redis.values.size).toBe(0);
  });
});
