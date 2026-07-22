import { CallHandler, Controller, Get, Header, Injectable, NestInterceptor, Res, ServiceUnavailableException } from '@nestjs/common';
import { DataSource } from 'typeorm';
import { Observable, tap } from 'rxjs';
import { Response } from 'express';
import { BrokerService } from './broker.service';
import { RedisService } from './redis.service';

@Injectable()
export class MetricsService {
  private requests = 0;
  private errors = 0;
  private durationMs = 0;

  observe(durationMs: number, failed: boolean): void {
    this.requests += 1;
    this.durationMs += durationMs;
    if (failed) this.errors += 1;
  }

  render(): string {
    return [
      '# HELP ticketing_http_requests_total Total HTTP requests.',
      '# TYPE ticketing_http_requests_total counter',
      `ticketing_http_requests_total ${this.requests}`,
      '# HELP ticketing_http_errors_total Total failed HTTP requests.',
      '# TYPE ticketing_http_errors_total counter',
      `ticketing_http_errors_total ${this.errors}`,
      '# HELP ticketing_http_duration_milliseconds_total Accumulated request time.',
      '# TYPE ticketing_http_duration_milliseconds_total counter',
      `ticketing_http_duration_milliseconds_total ${this.durationMs.toFixed(3)}`,
      '',
    ].join('\n');
  }
}

@Injectable()
export class MetricsInterceptor implements NestInterceptor {
  constructor(private readonly metrics: MetricsService) {}

  intercept(_context: unknown, next: CallHandler): Observable<unknown> {
    const started = performance.now();
    return next.handle().pipe(tap({
      next: () => this.metrics.observe(performance.now() - started, false),
      error: () => this.metrics.observe(performance.now() - started, true),
    }));
  }
}

@Controller()
export class HealthController {
  constructor(
    private readonly dataSource: DataSource,
    private readonly redis: RedisService,
    private readonly broker: BrokerService,
    private readonly metrics: MetricsService,
  ) {}

  @Get('health/live')
  live() { return { status: 'ok', uptimeSeconds: Math.floor(process.uptime()) }; }

  @Get('health/ready')
  async ready() {
    let database = false;
    try { await this.dataSource.query('SELECT 1'); database = true; } catch { database = false; }
    const redis = await this.redis.ping();
    const rabbitmq = this.broker.isHealthy();
    if (!database || !redis) throw new ServiceUnavailableException({ status: 'unavailable', checks: { database, redis, rabbitmq } });
    return { status: rabbitmq ? 'ok' : 'degraded', checks: { database, redis, rabbitmq } };
  }

  @Get('metrics')
  @Header('Content-Type', 'text/plain; version=0.0.4; charset=utf-8')
  metricsText(@Res() response: Response) { response.send(this.metrics.render()); }
}
