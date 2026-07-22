import { Body, ConflictException, Controller, ForbiddenException, Get, Injectable, Logger, NotFoundException, OnApplicationShutdown, OnModuleInit, Param, Post, Req, UseGuards } from '@nestjs/common';
import { InjectRepository } from '@nestjs/typeorm';
import { randomUUID } from 'crypto';
import { DataSource, In, Repository } from 'typeorm';
import { BrokerService } from './broker.service';
import { CreateReservationDto } from './dto';
import { Event, PricingCategory, Reservation, ReservationSeat, ReservationState, Role, Seat, SeatState, Sector } from './entities';
import { LockService } from './lock.service';
import { UpdatesGateway } from './realtime.gateway';
import { AuthenticatedUser, AuthGuard } from './security';
import { WaitingRoomService } from './waiting-room.controller';

@Controller('reservations')
@UseGuards(AuthGuard)
export class ReservationsController {
  private readonly logger = new Logger(ReservationsController.name);

  constructor(
    private readonly dataSource: DataSource,
    @InjectRepository(Reservation) private readonly reservations: Repository<Reservation>,
    @InjectRepository(ReservationSeat) private readonly reservationSeats: Repository<ReservationSeat>,
    @InjectRepository(Event) private readonly events: Repository<Event>,
    @InjectRepository(Sector) private readonly sectors: Repository<Sector>,
    @InjectRepository(Seat) private readonly seats: Repository<Seat>,
    @InjectRepository(PricingCategory) private readonly prices: Repository<PricingCategory>,
    private readonly locks: LockService,
    private readonly waitingRoom: WaitingRoomService,
    private readonly updates: UpdatesGateway,
    private readonly broker: BrokerService,
  ) {}

  @Post()
  async create(@Req() request: { user: AuthenticatedUser }, @Body() body: CreateReservationDto) {
    await this.waitingRoom.assertAdmission(body.eventId, request.user.sub, body.admissionToken);
    const event = await this.events.findOneBy({ id: body.eventId, published: true });
    if (!event || event.startsAt <= new Date()) throw new ConflictException('Event is unavailable');
    const sectorIds = (await this.sectors.findBy({ venueId: event.venueId })).map((sector) => sector.id);
    const selectedSeats = sectorIds.length ? await this.seats.findBy({ id: In(body.seatIds), sectorId: In(sectorIds) }) : [];
    if (selectedSeats.length !== body.seatIds.length) throw new ConflictException('One or more seats do not belong to the event venue');
    const pricing = await this.prices.findBy({ eventId: event.id });
    const defaultPrice = pricing.find((price) => !price.sectorId);
    const seatPrices = new Map(selectedSeats.map((seat) => {
      const category = pricing.find((price) => price.sectorId === seat.sectorId) ?? defaultPrice;
      if (!category) throw new ConflictException('Selected seat has no pricing category');
      return [seat.id, Number(category.price)];
    }));
    const id = randomUUID();
    await this.locks.acquire(event.id, body.seatIds, id);
    try {
      const reservation = await this.dataSource.transaction(async (manager) => {
        const occupied = await manager.getRepository(ReservationSeat).findBy({ eventId: event.id, seatId: In(body.seatIds) });
        if (occupied.length) throw new ConflictException('At least one selected seat is unavailable');
        const totalAmount = body.seatIds.reduce((sum, seatId) => sum + (seatPrices.get(seatId) ?? 0), 0);
        const created = await manager.getRepository(Reservation).save(manager.getRepository(Reservation).create({
          id,
          userId: request.user.sub,
          eventId: event.id,
          state: ReservationState.PENDING,
          expiresAt: new Date(Date.now() + Number(process.env.LOCK_TTL_SECONDS ?? 600) * 1000),
          totalAmount,
          currency: pricing[0]?.currency ?? 'IRR',
        }));
        await manager.getRepository(ReservationSeat).save(body.seatIds.map((seatId) => manager.getRepository(ReservationSeat).create({
          reservationId: created.id,
          eventId: event.id,
          seatId,
          state: SeatState.LOCKED,
          price: seatPrices.get(seatId) ?? 0,
        })));
        return created;
      });
      this.updates.emitEvent(event.id, 'seat.status.changed', { eventId: event.id, seatIds: body.seatIds, state: SeatState.LOCKED });
      this.broker.publish('reservation.created', { ...reservation, userId: request.user.sub });
      return reservation;
    } catch (error) {
      try {
        await this.locks.release(event.id, body.seatIds, id);
      } catch (releaseError) {
        this.logger.warn(`Reservation ${id} failed and its Redis locks could not be released: ${String(releaseError)}`);
      }
      if (String(error).includes('duplicate key')) throw new ConflictException('At least one selected seat is unavailable');
      throw error;
    }
  }

  @Get(':id')
  async get(@Req() request: { user: AuthenticatedUser }, @Param('id') id: string) {
    const reservation = await this.reservations.findOneBy({ id });
    if (!reservation) throw new NotFoundException('Reservation not found');
    this.assertOwner(reservation, request.user);
    return { ...reservation, seats: await this.reservationSeats.findBy({ reservationId: id }) };
  }

  @Post(':id/cancel')
  async cancel(@Req() request: { user: AuthenticatedUser }, @Param('id') id: string) {
    const { reservation, seats } = await this.dataSource.transaction(async (manager) => {
      const reservation = await manager.getRepository(Reservation).findOne({ where: { id }, lock: { mode: 'pessimistic_write' } });
      if (!reservation) throw new NotFoundException('Reservation not found');
      this.assertOwner(reservation, request.user);
      if (reservation.state !== ReservationState.PENDING) throw new ConflictException('Only pending reservations can be cancelled');
      const seats = await manager.getRepository(ReservationSeat).findBy({ reservationId: id });
      reservation.state = ReservationState.CANCELLED;
      await manager.getRepository(Reservation).save(reservation);
      await manager.getRepository(ReservationSeat).remove(seats);
      return { reservation, seats };
    });
    try {
      await this.locks.release(reservation.eventId, seats.map((seat) => seat.seatId), reservation.id);
    } catch (error) {
      this.logger.warn(`Reservation ${reservation.id} cancelled but its Redis locks could not be released: ${String(error)}`);
    }
    this.updates.emitEvent(reservation.eventId, 'seat.status.changed', { eventId: reservation.eventId, seatIds: seats.map((seat) => seat.seatId), state: SeatState.AVAILABLE });
    this.broker.publish('reservation.cancelled', { ...reservation, userId: reservation.userId });
    return reservation;
  }

  private assertOwner(reservation: Reservation, user: AuthenticatedUser): void {
    if (reservation.userId !== user.sub && user.role !== Role.ADMIN) throw new ForbiddenException('Reservation belongs to another user');
  }
}

@Injectable()
export class ExpiryService implements OnModuleInit, OnApplicationShutdown {
  private readonly logger = new Logger(ExpiryService.name);
  private timer?: NodeJS.Timeout;
  private running = false;

  constructor(
    private readonly dataSource: DataSource,
    @InjectRepository(Reservation) private readonly reservations: Repository<Reservation>,
    @InjectRepository(ReservationSeat) private readonly seats: Repository<ReservationSeat>,
    private readonly locks: LockService,
    private readonly updates: UpdatesGateway,
    private readonly broker: BrokerService,
  ) {}

  onModuleInit(): void {
    this.timer = setInterval(() => void this.runSweep(), Number(process.env.EXPIRY_SWEEP_MS ?? 30000));
    this.timer.unref();
  }

  onApplicationShutdown(): void { if (this.timer) clearInterval(this.timer); }

  async expire(): Promise<number> {
    const stale = await this.reservations.createQueryBuilder('reservation')
      .where('reservation.state = :state', { state: ReservationState.PENDING })
      .andWhere('reservation.expiresAt <= :now', { now: new Date() })
      .take(100)
      .getMany();
    let expired = 0;
    for (const reservation of stale) {
      const seats = await this.seats.findBy({ reservationId: reservation.id });
      const changed = await this.dataSource.transaction(async (manager) => {
        const result = await manager.getRepository(Reservation).update({ id: reservation.id, state: ReservationState.PENDING }, { state: ReservationState.EXPIRED });
        if (result.affected) await manager.getRepository(ReservationSeat).remove(seats);
        return Boolean(result.affected);
      });
      if (!changed) continue;
      expired += 1;
      try {
        await this.locks.release(reservation.eventId, seats.map((seat) => seat.seatId), reservation.id);
      } catch (error) {
        this.logger.warn(`Failed to release Redis locks for expired reservation ${reservation.id}: ${String(error)}`);
      }
      this.updates.emitEvent(reservation.eventId, 'seat.status.changed', { eventId: reservation.eventId, seatIds: seats.map((seat) => seat.seatId), state: SeatState.AVAILABLE });
      this.broker.publish('reservation.expired', { ...reservation, userId: reservation.userId });
    }
    return expired;
  }

  private async runSweep(): Promise<void> {
    if (this.running) return;
    this.running = true;
    try {
      await this.expire();
    } catch (error) {
      this.logger.error(`Reservation expiry sweep failed: ${String(error)}`, error instanceof Error ? error.stack : undefined);
    } finally {
      this.running = false;
    }
  }
}
