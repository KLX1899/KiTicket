import { Body, ConflictException, Controller, ForbiddenException, Logger, Param, Post, Req, UseGuards } from '@nestjs/common';
import { InjectRepository } from '@nestjs/typeorm';
import { createHash, randomBytes } from 'crypto';
import { DataSource, EntityManager, Repository } from 'typeorm';
import { BrokerService } from './broker.service';
import { CompletePaymentDto, PaymentOutcome, StartPaymentDto } from './dto';
import { Payment, PaymentState, Reservation, ReservationSeat, ReservationState, Ticket } from './entities';
import { LockService } from './lock.service';
import { determinePaymentTransition } from './payment-policy';
import { UpdatesGateway } from './realtime.gateway';
import { AuthenticatedUser, AuthGuard } from './security';

interface PaymentResult {
  payment: Payment;
  reservation: Reservation;
  tickets: Ticket[];
  seatIds: string[];
  transitioned: boolean;
}

@Controller('payments')
@UseGuards(AuthGuard)
export class PaymentsController {
  private readonly logger = new Logger(PaymentsController.name);

  constructor(
    private readonly dataSource: DataSource,
    @InjectRepository(Payment) private readonly payments: Repository<Payment>,
    @InjectRepository(Reservation) private readonly reservations: Repository<Reservation>,
    private readonly locks: LockService,
    private readonly updates: UpdatesGateway,
    private readonly broker: BrokerService,
  ) {}

  @Post()
  async start(@Req() request: { user: AuthenticatedUser }, @Body() body: StartPaymentDto) {
    const existing = await this.payments.findOneBy({ idempotencyKey: body.idempotencyKey });
    if (existing) {
      const linked = await this.reservations.findOneByOrFail({ id: existing.reservationId });
      if (linked.userId !== request.user.sub || linked.id !== body.reservationId) throw new ConflictException('Idempotency key belongs to another request');
      return existing;
    }
    const reservation = await this.reservations.findOneByOrFail({ id: body.reservationId });
    if (reservation.userId !== request.user.sub) throw new ForbiddenException('Reservation belongs to another user');
    if (reservation.state !== ReservationState.PENDING || reservation.expiresAt <= new Date()) throw new ConflictException('Reservation is no longer payable');
    try {
      const payment = await this.payments.save(this.payments.create({
        reservationId: reservation.id,
        idempotencyKey: body.idempotencyKey,
        state: PaymentState.PENDING,
        amount: reservation.totalAmount,
        currency: reservation.currency,
      }));
      this.updates.emitUser(request.user.sub, 'payment.started', payment);
      this.broker.publish('payment.started', { ...payment, userId: request.user.sub });
      return payment;
    } catch (error) {
      if (String(error).includes('duplicate key')) return this.payments.findOneByOrFail({ reservationId: reservation.id });
      throw error;
    }
  }

  @Post(':id/complete')
  async complete(@Req() request: { user: AuthenticatedUser }, @Param('id') id: string, @Body() body: CompletePaymentDto) {
    const result = await this.dataSource.transaction((manager) => this.completeTransaction(manager, id, request.user.sub, body));
    if (!result.transitioned) {
      return { payment: result.payment, reservation: result.reservation, tickets: result.tickets };
    }
    try {
      await this.locks.release(result.reservation.eventId, result.seatIds, result.reservation.id);
    } catch (error) {
      this.logger.warn(`Payment ${result.payment.id} committed but its Redis locks could not be released: ${String(error)}`);
    }
    const eventName = result.payment.state === PaymentState.SUCCESS ? 'payment.succeeded' : 'payment.failed';
    this.updates.emitUser(result.reservation.userId, eventName, result.payment);
    this.updates.emitEvent(result.reservation.eventId, 'seat.status.changed', {
      eventId: result.reservation.eventId,
      seatIds: result.seatIds,
      state: result.payment.state === PaymentState.SUCCESS ? 'BOOKED' : 'AVAILABLE',
    });
    this.broker.publish(eventName, { ...result.payment, userId: result.reservation.userId });
    if (result.tickets.length) {
      this.updates.emitUser(result.reservation.userId, 'ticket.issued', { ticketIds: result.tickets.map((ticket) => ticket.id) });
      this.broker.publish('ticket.issued', { tickets: result.tickets, userId: result.reservation.userId });
    }
    return { payment: result.payment, reservation: result.reservation, tickets: result.tickets };
  }

  private async completeTransaction(manager: EntityManager, id: string, userId: string, body: CompletePaymentDto): Promise<PaymentResult> {
    const payment = await manager.getRepository(Payment).findOne({ where: { id }, lock: { mode: 'pessimistic_write' } });
    if (!payment) throw new ConflictException('Payment not found');
    const reservation = await manager.getRepository(Reservation).findOne({ where: { id: payment.reservationId }, lock: { mode: 'pessimistic_write' } });
    if (!reservation) throw new ConflictException('Reservation not found');
    if (reservation.userId !== userId) throw new ForbiddenException('Payment belongs to another user');
    const seats = await manager.getRepository(ReservationSeat).findBy({ reservationId: reservation.id });
    const ticketRepository = manager.getRepository(Ticket);
    if (payment.state !== PaymentState.PENDING) {
      return {
        payment,
        reservation,
        tickets: await ticketRepository.findBy({ reservationId: reservation.id }),
        seatIds: seats.map((seat) => seat.seatId),
        transitioned: false,
      };
    }
    const transition = determinePaymentTransition(body.outcome, reservation.expiresAt <= new Date());
    let tickets: Ticket[] = [];
    if (transition.outcome === PaymentOutcome.SUCCESS) {
      if (!seats.length) throw new ConflictException('Reservation has no locked seats');
      payment.state = transition.paymentState;
      payment.reference = body.providerReference ?? randomBytes(8).toString('hex');
      reservation.state = transition.reservationState;
      for (const seat of seats) seat.state = 'BOOKED' as ReservationSeat['state'];
      await manager.getRepository(ReservationSeat).save(seats);
      tickets = await ticketRepository.save(seats.map((seat) => {
        const token = randomBytes(32).toString('base64url');
        return ticketRepository.create({
          reservationId: reservation.id,
          seatId: seat.seatId,
          token,
          qrHash: createHash('sha256').update(token).digest('hex'),
        });
      }));
    } else {
      payment.state = transition.paymentState;
      payment.failureReason = transition.failureReason;
      reservation.state = transition.reservationState;
      await manager.getRepository(ReservationSeat).remove(seats);
    }
    await manager.getRepository(Reservation).save(reservation);
    await manager.getRepository(Payment).save(payment);
    return { payment, reservation, tickets, seatIds: seats.map((seat) => seat.seatId), transitioned: true };
  }
}
