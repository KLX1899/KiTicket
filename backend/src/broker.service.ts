import { Injectable, Logger, OnApplicationShutdown, OnModuleInit } from '@nestjs/common';
import { InjectRepository } from '@nestjs/typeorm';
import { Channel, ChannelModel, ConsumeMessage, connect } from 'amqplib';
import { randomBytes } from 'crypto';
import { Repository } from 'typeorm';
import { Notification } from './entities';

export interface DomainEvent<T = unknown> {
  id: string;
  type: string;
  occurredAt: string;
  payload: T;
}

@Injectable()
export class BrokerService implements OnModuleInit, OnApplicationShutdown {
  private readonly logger = new Logger(BrokerService.name);
  private connection?: ChannelModel;
  private channel?: Channel;
  private healthy = false;

  constructor(@InjectRepository(Notification) private readonly notifications: Repository<Notification>) {}

  async onModuleInit(): Promise<void> {
    try {
      this.connection = await connect(process.env.RABBITMQ_URL ?? 'amqp://localhost');
      this.connection.on('error', () => { this.healthy = false; });
      this.connection.on('close', () => { this.healthy = false; this.channel = undefined; });
      this.channel = await this.connection.createChannel();
      await this.channel.assertExchange('ticketing.events', 'topic', { durable: true });
      const queue = await this.channel.assertQueue('ticketing.notifications', {
        durable: true,
        arguments: { 'x-dead-letter-exchange': 'ticketing.dead-letter' },
      });
      await this.channel.assertExchange('ticketing.dead-letter', 'fanout', { durable: true });
      await this.channel.bindQueue(queue.queue, 'ticketing.events', 'payment.#');
      await this.channel.bindQueue(queue.queue, 'ticketing.events', 'ticket.#');
      await this.channel.consume(queue.queue, (message) => void this.consume(message), { noAck: false });
      this.healthy = true;
    } catch (error) {
      this.logger.warn(`RabbitMQ unavailable; domain events will be logged: ${String(error)}`);
    }
  }

  private async consume(message: ConsumeMessage | null): Promise<void> {
    if (!message || !this.channel) return;
    try {
      const event = JSON.parse(message.content.toString()) as DomainEvent<{ userId?: string }>;
      if (event.payload.userId) {
        const notification = this.notifications.create({
          userId: event.payload.userId,
          channel: 'EMAIL',
          message: this.messageFor(event.type),
          sent: true,
          sentAt: new Date(),
          providerReference: `sandbox-${event.id}`,
        });
        await this.notifications.save(notification);
      }
      this.channel.ack(message);
    } catch (error) {
      this.logger.error(`Notification event failed: ${String(error)}`);
      this.channel.nack(message, false, false);
    }
  }

  private messageFor(type: string): string {
    if (type === 'payment.succeeded') return 'پرداخت شما با موفقیت انجام شد.';
    if (type === 'ticket.issued') return 'بلیت شما صادر شد.';
    return 'وضعیت سفارش شما به‌روزرسانی شد.';
  }

  publish(type: string, payload: unknown): DomainEvent {
    const event: DomainEvent = {
      id: randomBytes(12).toString('hex'),
      type,
      occurredAt: new Date().toISOString(),
      payload,
    };
    const body = Buffer.from(JSON.stringify(event));
    try {
      if (this.channel) this.channel.publish('ticketing.events', type, body, { persistent: true, contentType: 'application/json' });
      else this.logger.log(`Domain event ${type}: ${body.toString()}`);
    } catch (error) {
      this.healthy = false;
      this.logger.warn(`Domain event ${type} could not be published: ${String(error)}`);
    }
    return event;
  }

  isHealthy(): boolean { return this.healthy; }

  async onApplicationShutdown(): Promise<void> {
    this.healthy = false;
    try { await this.channel?.close(); } catch { /* connection may already be closed */ }
    try { await this.connection?.close(); } catch { /* connection may already be closed */ }
  }
}
