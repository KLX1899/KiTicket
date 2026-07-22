import { Module } from '@nestjs/common';
import { APP_GUARD, APP_INTERCEPTOR } from '@nestjs/core';
import { JwtModule } from '@nestjs/jwt';
import { ThrottlerGuard, ThrottlerModule } from '@nestjs/throttler';
import { TypeOrmModule } from '@nestjs/typeorm';
import { AuthController, UsersController } from './auth.controller';
import { BrokerService } from './broker.service';
import { EventsController, VenuesController } from './catalog.controller';
import {
  Event,
  Notification,
  Payment,
  PricingCategory,
  Reservation,
  ReservationSeat,
  Seat,
  Sector,
  Ticket,
  User,
  Venue,
  WaitingRoomEntry,
} from './entities';
import { HealthController, MetricsInterceptor, MetricsService } from './health.controller';
import { LockService } from './lock.service';
import { PaymentsController } from './payments.controller';
import { RedisService, redisProvider } from './redis.service';
import { ExpiryService, ReservationsController } from './reservations.controller';
import { UpdatesGateway } from './realtime.gateway';
import { AuthGuard } from './security';
import { NotificationsController, TicketsController } from './tickets.controller';
import { WaitingRoomController, WaitingRoomService } from './waiting-room.controller';

const entities = [
  User,
  Venue,
  Sector,
  Seat,
  Event,
  PricingCategory,
  Reservation,
  ReservationSeat,
  Payment,
  Ticket,
  Notification,
  WaitingRoomEntry,
];

@Module({
  imports: [
    JwtModule.register({
      global: true,
      secret: process.env.JWT_SECRET ?? 'development-secret-change-me',
      signOptions: { expiresIn: '1h', issuer: 'ticketing-api', audience: 'ticketing-web' },
      verifyOptions: { issuer: 'ticketing-api', audience: 'ticketing-web' },
    }),
    ThrottlerModule.forRoot([{
      ttl: Number(process.env.RATE_LIMIT_WINDOW_MS ?? 60000),
      limit: Number(process.env.RATE_LIMIT_REQUESTS ?? 120),
    }]),
    TypeOrmModule.forRoot({
      type: 'postgres',
      url: process.env.DATABASE_URL ?? 'postgres://ticketing:ticketing@localhost:5432/ticketing',
      entities,
      synchronize: process.env.DB_SYNCHRONIZE === 'true' || process.env.NODE_ENV !== 'production',
      ssl: process.env.DB_SSL === 'true' ? { rejectUnauthorized: true } : false,
      logging: process.env.DB_LOGGING === 'true',
    }),
    TypeOrmModule.forFeature(entities),
  ],
  controllers: [
    AuthController,
    UsersController,
    VenuesController,
    EventsController,
    ReservationsController,
    PaymentsController,
    TicketsController,
    NotificationsController,
    WaitingRoomController,
    HealthController,
  ],
  providers: [
    AuthGuard,
    redisProvider,
    RedisService,
    LockService,
    BrokerService,
    UpdatesGateway,
    ExpiryService,
    WaitingRoomService,
    MetricsService,
    { provide: APP_GUARD, useClass: ThrottlerGuard },
    { provide: APP_INTERCEPTOR, useClass: MetricsInterceptor },
  ],
})
export class AppModule {}
