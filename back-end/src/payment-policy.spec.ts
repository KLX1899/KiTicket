import { PaymentOutcome } from './dto';
import { PaymentState, ReservationState } from './entities';
import { determinePaymentTransition } from './payment-policy';

describe('payment saga transition policy', () => {
  it('confirms the reservation after a successful gateway result', () => {
    expect(determinePaymentTransition(PaymentOutcome.SUCCESS, false)).toEqual({
      outcome: PaymentOutcome.SUCCESS,
      paymentState: PaymentState.SUCCESS,
      reservationState: ReservationState.CONFIRMED,
    });
  });

  it('compensates a declined payment', () => {
    expect(determinePaymentTransition(PaymentOutcome.FAILURE, false)).toMatchObject({
      paymentState: PaymentState.FAILED,
      reservationState: ReservationState.CANCELLED,
      failureReason: 'gateway-declined',
    });
  });

  it('turns even a late success into a timeout compensation', () => {
    expect(determinePaymentTransition(PaymentOutcome.SUCCESS, true)).toMatchObject({
      outcome: PaymentOutcome.TIMEOUT,
      paymentState: PaymentState.TIMEOUT,
      reservationState: ReservationState.EXPIRED,
    });
  });
});
