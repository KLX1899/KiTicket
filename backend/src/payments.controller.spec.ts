import { PaymentOutcome } from './dto';
import { PaymentState, ReservationState, Role } from './entities';
import { PaymentsController } from './payments.controller';

describe('PaymentsController idempotent completion', () => {
  it('returns an already completed payment without repeating side effects', async () => {
    const payment = { id: 'payment-1', state: PaymentState.SUCCESS };
    const reservation = { id: 'reservation-1', eventId: 'event-1', userId: 'user-1', state: ReservationState.CONFIRMED };
    const tickets = [{ id: 'ticket-1' }];
    const dataSource = {
      transaction: jest.fn().mockResolvedValue({ payment, reservation, tickets, seatIds: ['seat-1'], transitioned: false }),
    };
    const locks = { release: jest.fn() };
    const updates = { emitUser: jest.fn(), emitEvent: jest.fn() };
    const broker = { publish: jest.fn() };
    const controller = new PaymentsController(
      dataSource as never,
      {} as never,
      {} as never,
      locks as never,
      updates as never,
      broker as never,
    );

    await expect(controller.complete(
      { user: { sub: 'user-1', email: 'buyer@example.com', role: Role.CUSTOMER } },
      'payment-1',
      { outcome: PaymentOutcome.SUCCESS },
    )).resolves.toEqual({ payment, reservation, tickets });
    expect(locks.release).not.toHaveBeenCalled();
    expect(updates.emitUser).not.toHaveBeenCalled();
    expect(updates.emitEvent).not.toHaveBeenCalled();
    expect(broker.publish).not.toHaveBeenCalled();
  });
});
