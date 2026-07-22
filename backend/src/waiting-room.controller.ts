import { Body, ConflictException, Controller, ForbiddenException, Get, Injectable, NotFoundException, Param, Post, Req, UseGuards } from '@nestjs/common';
import { InjectRepository } from '@nestjs/typeorm';
import { randomBytes } from 'crypto';
import { Repository } from 'typeorm';
import { AdmitUsersDto } from './dto';
import { Event, Role, WaitingRoomEntry, WaitingRoomState } from './entities';
import { RedisService } from './redis.service';
import { AuthenticatedUser, AuthGuard, Roles } from './security';

@Injectable()
export class WaitingRoomService {
  constructor(@InjectRepository(WaitingRoomEntry) private readonly entries: Repository<WaitingRoomEntry>) {}

  async assertAdmission(eventId: string, userId: string, token?: string): Promise<void> {
    if ((process.env.WAITING_ROOM_REQUIRED ?? 'true') !== 'true') return;
    if (!token) throw new ForbiddenException('Waiting-room admission token is required');
    const entry = await this.entries.findOneBy({ eventId, userId, admissionToken: token, state: WaitingRoomState.ADMITTED });
    if (!entry || !entry.tokenExpiresAt || entry.tokenExpiresAt <= new Date()) throw new ForbiddenException('Admission token is invalid or expired');
  }
}

@Controller('waiting-room')
@UseGuards(AuthGuard)
export class WaitingRoomController {
  constructor(
    @InjectRepository(WaitingRoomEntry) private readonly entries: Repository<WaitingRoomEntry>,
    @InjectRepository(Event) private readonly events: Repository<Event>,
    private readonly redis: RedisService,
  ) {}

  @Post(':eventId/join')
  async join(@Req() request: { user: AuthenticatedUser }, @Param('eventId') eventId: string) {
    const event = await this.events.findOneBy({ id: eventId, published: true });
    if (!event || event.startsAt <= new Date()) throw new ConflictException('Event is unavailable');
    const active = await this.entries.findOne({
      where: [
        { eventId, userId: request.user.sub, state: WaitingRoomState.QUEUED },
        { eventId, userId: request.user.sub, state: WaitingRoomState.ADMITTED },
      ],
      order: { createdAt: 'DESC' },
    });
    if (active && (!active.tokenExpiresAt || active.tokenExpiresAt > new Date())) return this.response(active);

    const client = await this.redis.ready();
    const position = await client.incr(`waiting:event:${eventId}:sequence`);
    const immediateCapacity = Number(process.env.WAITING_ROOM_INITIAL_ADMISSIONS ?? 100);
    const admitted = position <= immediateCapacity;
    const entry = await this.entries.save(this.entries.create({
      eventId,
      userId: request.user.sub,
      position,
      state: admitted ? WaitingRoomState.ADMITTED : WaitingRoomState.QUEUED,
      ...(admitted ? this.tokenFields() : {}),
    }));
    return this.response(entry);
  }

  @Get(':id')
  async status(@Req() request: { user: AuthenticatedUser }, @Param('id') id: string) {
    const entry = await this.entries.findOneBy({ id });
    if (!entry) throw new NotFoundException('Waiting-room entry not found');
    if (entry.userId !== request.user.sub && request.user.role !== Role.ADMIN) throw new ForbiddenException();
    return this.response(entry);
  }

  @Post(':eventId/admit')
  @Roles(Role.ADMIN)
  async admit(@Param('eventId') eventId: string, @Body() body: AdmitUsersDto) {
    const queued = await this.entries.find({
      where: { eventId, state: WaitingRoomState.QUEUED },
      order: { position: 'ASC' },
      take: body.count,
    });
    for (const entry of queued) Object.assign(entry, { state: WaitingRoomState.ADMITTED, ...this.tokenFields() });
    await this.entries.save(queued);
    return { admitted: queued.length, entries: queued.map((entry) => this.response(entry)) };
  }

  private tokenFields(): Pick<WaitingRoomEntry, 'admissionToken' | 'tokenExpiresAt'> {
    return {
      admissionToken: randomBytes(24).toString('base64url'),
      tokenExpiresAt: new Date(Date.now() + Number(process.env.ADMISSION_TTL_SECONDS ?? 900) * 1000),
    };
  }

  private response(entry: WaitingRoomEntry) {
    return {
      id: entry.id,
      eventId: entry.eventId,
      position: entry.position,
      state: entry.state,
      admissionToken: entry.admissionToken,
      tokenExpiresAt: entry.tokenExpiresAt,
    };
  }
}
