import { PaymentOutcome } from './dto';
import { PaymentState, ReservationState } from './entities';

export interface PaymentTransition {
  outcome: PaymentOutcome;
  paymentState: PaymentState;
  reservationState: ReservationState;
  failureReason?: string;
}

export function determinePaymentTransition(requested: PaymentOutcome, expired: boolean): PaymentTransition {
  const outcome = expired ? PaymentOutcome.TIMEOUT : requested;
  if (outcome === PaymentOutcome.SUCCESS) {
    return { outcome, paymentState: PaymentState.SUCCESS, reservationState: ReservationState.CONFIRMED };
  }
  if (outcome === PaymentOutcome.TIMEOUT) {
    return {
      outcome,
      paymentState: PaymentState.TIMEOUT,
      reservationState: ReservationState.EXPIRED,
      failureReason: 'reservation-expired',
    };
  }
  return {
    outcome,
    paymentState: PaymentState.FAILED,
    reservationState: ReservationState.CANCELLED,
    failureReason: 'gateway-declined',
  };
}
