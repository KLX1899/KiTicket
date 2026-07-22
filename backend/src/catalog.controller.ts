import { BadRequestException, Body, Controller, ForbiddenException, Get, NotFoundException, Param, Patch, Post, Query, Req, UseGuards } from '@nestjs/common';
import { InjectRepository } from '@nestjs/typeorm';
import { Brackets, DataSource, In, Repository } from 'typeorm';
import { CreateEventDto, CreatePricingDto, CreateSectorDto, CreateVenueDto, EventQueryDto, UpdateEventDto } from './dto';
import { Event, Payment, PaymentState, PricingCategory, Reservation, ReservationSeat, ReservationState, Role, Seat, SeatState, Sector, Venue } from './entities';
import { AuthenticatedUser, AuthGuard, Roles } from './security';

@Controller('venues')
export class VenuesController {
  constructor(
    @InjectRepository(Venue) private readonly venues: Repository<Venue>,
    @InjectRepository(Sector) private readonly sectors: Repository<Sector>,
    @InjectRepository(Seat) private readonly seats: Repository<Seat>,
  ) {}

  @Get()
  list() { return this.venues.find({ order: { name: 'ASC' } }); }

  @Post()
  @UseGuards(AuthGuard)
  @Roles(Role.ORGANIZER, Role.ADMIN)
  create(@Body() body: CreateVenueDto) { return this.venues.save(this.venues.create(body)); }

  @Post(':id/sectors')
  @UseGuards(AuthGuard)
  @Roles(Role.ORGANIZER, Role.ADMIN)
  async createSector(@Param('id') venueId: string, @Body() body: CreateSectorDto) {
    await this.venues.findOneByOrFail({ id: venueId });
    const sector = await this.sectors.save(this.sectors.create({ venueId, name: body.name }));
    const seats: Seat[] = [];
    for (let rowIndex = 0; rowIndex < body.rows; rowIndex += 1) {
      for (let number = 1; number <= body.seatsPerRow; number += 1) {
        seats.push(this.seats.create({
          sectorId: sector.id,
          row: rowLabel(rowIndex),
          number,
          accessible: Boolean(body.accessibleFirstRow && rowIndex === 0),
        }));
      }
    }
    await this.seats.save(seats, { chunk: 500 });
    return { ...sector, capacity: seats.length };
  }

  @Get(':id/layout')
  async layout(@Param('id') id: string) {
    const venue = await this.venues.findOneBy({ id });
    if (!venue) throw new NotFoundException('Venue not found');
    const sectors = await this.sectors.findBy({ venueId: id });
    const seats = sectors.length ? await this.seats.findBy({ sectorId: In(sectors.map((sector) => sector.id)) }) : [];
    return { ...venue, capacity: seats.length, sectors: sectors.map((sector) => ({
      ...sector,
      seats: seats.filter((seat) => seat.sectorId === sector.id).sort((a, b) => a.row.localeCompare(b.row) || a.number - b.number),
    })) };
  }
}

@Controller('events')
export class EventsController {
  constructor(
    private readonly dataSource: DataSource,
    @InjectRepository(Event) private readonly events: Repository<Event>,
    @InjectRepository(Venue) private readonly venues: Repository<Venue>,
    @InjectRepository(Sector) private readonly sectors: Repository<Sector>,
    @InjectRepository(Seat) private readonly seats: Repository<Seat>,
    @InjectRepository(PricingCategory) private readonly prices: Repository<PricingCategory>,
    @InjectRepository(ReservationSeat) private readonly reservationSeats: Repository<ReservationSeat>,
    @InjectRepository(Reservation) private readonly reservations: Repository<Reservation>,
    @InjectRepository(Payment) private readonly payments: Repository<Payment>,
  ) {}

  @Get()
  async list(@Query() query: EventQueryDto) {
    const builder = this.events.createQueryBuilder('event')
      .where('event.published = true')
      .andWhere('event.startsAt > :now', { now: new Date() });
    if (query.q) builder.andWhere(new Brackets((q) => q.where('event.title ILIKE :term').orWhere('event.description ILIKE :term').orWhere('event.tags ILIKE :term')), { term: `%${query.q}%` });
    if (query.genre) builder.andWhere('event.genre = :genre', { genre: query.genre });
    if (query.city) builder.andWhere('event.city = :city', { city: query.city });
    if (query.from) builder.andWhere('event.startsAt >= :from', { from: query.from });
    if (query.to) builder.andWhere('event.startsAt <= :to', { to: query.to });
    if (query.available) {
      builder.andWhere(`EXISTS (
        SELECT 1 FROM seat s JOIN sector sec ON sec.id = s."sectorId"
        WHERE sec."venueId" = event."venueId" AND NOT EXISTS (
          SELECT 1 FROM reservation_seat rs WHERE rs."eventId" = event.id AND rs."seatId" = s.id
        )
      )`);
    }
    const [items, total] = await builder.orderBy('event.startsAt', 'ASC')
      .skip((query.page - 1) * query.limit).take(query.limit).getManyAndCount();
    return { items, page: query.page, limit: query.limit, total, pages: Math.ceil(total / query.limit) };
  }

  @Get('mine')
  @UseGuards(AuthGuard)
  @Roles(Role.ORGANIZER, Role.ADMIN)
  listOwned(@Req() request: { user: AuthenticatedUser }) {
    return this.events.find({
      where: request.user.role === Role.ADMIN ? {} : { organizerId: request.user.sub },
      order: { startsAt: 'ASC' },
    });
  }

  @Get(':id')
  async detail(@Param('id') id: string) {
    const event = await this.events.findOneBy({ id, published: true });
    if (!event) throw new NotFoundException('رویداد پیدا نشد');
    const venue = await this.venues.findOneByOrFail({ id: event.venueId });
    const sectors = await this.sectors.findBy({ venueId: event.venueId });
    const seats = sectors.length ? await this.seats.findBy({ sectorId: In(sectors.map((sector) => sector.id)) }) : [];
    const states = await this.reservationSeats.findBy({ eventId: id });
    const pricing = await this.prices.findBy({ eventId: id });
    const stateBySeat = new Map(states.map((state) => [state.seatId, state.state]));
    const defaultPrice = pricing.find((price) => !price.sectorId);
    const inventory = seats.map((seat) => {
      const price = pricing.find((candidate) => candidate.sectorId === seat.sectorId) ?? defaultPrice;
      return {
        id: seat.id,
        seatId: seat.id,
        sectorId: seat.sectorId,
        row: seat.row,
        number: seat.number,
        accessible: seat.accessible,
        state: stateBySeat.get(seat.id) ?? SeatState.AVAILABLE,
        price: Number(price?.price ?? 0),
        currency: price?.currency ?? 'IRR',
      };
    });
    const bookable = event.startsAt > new Date();
    return {
      ...event,
      venue,
      sectors,
      pricing,
      bookable,
      availability: bookable ? inventory.filter((seat) => seat.state === SeatState.AVAILABLE).length : 0,
      inventory,
    };
  }

  @Post()
  @UseGuards(AuthGuard)
  @Roles(Role.ORGANIZER, Role.ADMIN)
  async create(@Req() request: { user: AuthenticatedUser }, @Body() body: CreateEventDto) {
    await this.venues.findOneByOrFail({ id: body.venueId });
    const startsAt = new Date(body.startsAt);
    const endsAt = body.endsAt ? new Date(body.endsAt) : undefined;
    validateEventTimes(startsAt, endsAt, true);
    return this.events.save(this.events.create({ ...body, startsAt, endsAt, tags: body.tags ?? [], organizerId: request.user.sub, published: false }));
  }

  @Patch(':id')
  @UseGuards(AuthGuard)
  @Roles(Role.ORGANIZER, Role.ADMIN)
  async update(@Req() request: { user: AuthenticatedUser }, @Param('id') id: string, @Body() body: UpdateEventDto) {
    const event = await this.ownedEvent(id, request.user);
    const startsAt = body.startsAt ? new Date(body.startsAt) : event.startsAt;
    const endsAt = body.endsAt ? new Date(body.endsAt) : event.endsAt;
    validateEventTimes(startsAt, endsAt, Boolean(body.startsAt));
    Object.assign(event, body, {
      ...(body.startsAt ? { startsAt } : {}),
      ...(body.endsAt ? { endsAt } : {}),
    });
    return this.events.save(event);
  }

  @Post(':id/pricing')
  @UseGuards(AuthGuard)
  @Roles(Role.ORGANIZER, Role.ADMIN)
  async addPricing(@Req() request: { user: AuthenticatedUser }, @Param('id') eventId: string, @Body() body: CreatePricingDto) {
    const event = await this.ownedEvent(eventId, request.user);
    if (body.sectorId) {
      const sector = await this.sectors.findOneBy({ id: body.sectorId, venueId: event.venueId });
      if (!sector) throw new NotFoundException('Sector does not belong to the event venue');
    }
    return this.prices.save(this.prices.create({ ...body, eventId, currency: body.currency ?? 'IRR' }));
  }

  @Post(':id/publish')
  @UseGuards(AuthGuard)
  @Roles(Role.ORGANIZER, Role.ADMIN)
  async publish(@Req() request: { user: AuthenticatedUser }, @Param('id') id: string) {
    const event = await this.ownedEvent(id, request.user);
    if (!await this.prices.exist({ where: { eventId: id } })) throw new ForbiddenException('At least one price is required');
    event.published = true;
    return this.events.save(event);
  }

  @Get(':id/analytics')
  @UseGuards(AuthGuard)
  @Roles(Role.ORGANIZER, Role.ADMIN)
  async analytics(@Req() request: { user: AuthenticatedUser }, @Param('id') id: string) {
    await this.ownedEvent(id, request.user);
    const reservations = await this.reservations.findBy({ eventId: id });
    const confirmed = reservations.filter((reservation) => reservation.state === ReservationState.CONFIRMED);
    const successfulPayments = confirmed.length ? await this.payments.findBy({ reservationId: In(confirmed.map((reservation) => reservation.id)), state: PaymentState.SUCCESS }) : [];
    const bookedSeats = await this.reservationSeats.countBy({ eventId: id, state: SeatState.BOOKED });
    const event = await this.events.findOneByOrFail({ id });
    const sectorIds = (await this.sectors.findBy({ venueId: event.venueId })).map((sector) => sector.id);
    const capacity = sectorIds.length ? await this.seats.countBy({ sectorId: In(sectorIds) }) : 0;
    return {
      reservations: reservations.length,
      confirmedReservations: confirmed.length,
      bookedSeats,
      remainingSeats: Math.max(0, capacity - bookedSeats),
      capacity,
      revenue: successfulPayments.reduce((sum, payment) => sum + Number(payment.amount), 0),
      currency: successfulPayments[0]?.currency ?? 'IRR',
    };
  }

  private async ownedEvent(id: string, user: AuthenticatedUser): Promise<Event> {
    const event = await this.events.findOneBy({ id });
    if (!event) throw new NotFoundException('Event not found');
    if (user.role !== Role.ADMIN && event.organizerId !== user.sub) throw new ForbiddenException('You do not own this event');
    return event;
  }
}

function rowLabel(index: number): string {
  let label = '';
  for (let value = index + 1; value > 0; value = Math.floor((value - 1) / 26)) label = String.fromCharCode(65 + ((value - 1) % 26)) + label;
  return label;
}

function validateEventTimes(startsAt: Date, endsAt?: Date, requireFuture = false): void {
  if (Number.isNaN(startsAt.getTime()) || (endsAt && Number.isNaN(endsAt.getTime()))) {
    throw new BadRequestException('Event times are invalid');
  }
  if ((requireFuture && startsAt <= new Date()) || (endsAt && endsAt <= startsAt)) {
    throw new BadRequestException('Event times are invalid');
  }
}
