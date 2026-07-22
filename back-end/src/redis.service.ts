import { Inject, Injectable, OnApplicationShutdown } from '@nestjs/common';
import Redis from 'ioredis';

export const REDIS_CLIENT = Symbol('REDIS_CLIENT');

export interface RedisOperations {
  status: string;
  connect(): Promise<void>;
  eval(script: string, numberOfKeys: number, ...args: (string | number)[]): Promise<unknown>;
  get(key: string): Promise<string | null>;
  incr(key: string): Promise<number>;
  ping(): Promise<string>;
  quit(): Promise<string>;
}

export const redisProvider = {
  provide: REDIS_CLIENT,
  useFactory: (): RedisOperations => new Redis(process.env.REDIS_URL ?? 'redis://localhost:6379', {
    lazyConnect: true,
    maxRetriesPerRequest: 2,
    enableOfflineQueue: false,
  }),
};

@Injectable()
export class RedisService implements OnApplicationShutdown {
  constructor(@Inject(REDIS_CLIENT) private readonly client: RedisOperations) {}

  async ready(): Promise<RedisOperations> {
    if (this.client.status === 'wait') await this.client.connect();
    return this.client;
  }

  async ping(): Promise<boolean> {
    try {
      return (await (await this.ready()).ping()) === 'PONG';
    } catch {
      return false;
    }
  }

  async onApplicationShutdown(): Promise<void> {
    if (!['end', 'wait'].includes(this.client.status)) await this.client.quit();
  }
}
