import { plainToInstance } from 'class-transformer';
import { validate } from 'class-validator';
import { CreateReservationDto, RegisterDto } from './dto';

describe('API input contracts', () => {
  it('rejects weak registration passwords', async () => {
    const input = plainToInstance(RegisterDto, { email: 'buyer@example.com', name: 'Buyer', password: 'password' });
    expect(await validate(input)).not.toHaveLength(0);
  });

  it('accepts a strong registration payload', async () => {
    const input = plainToInstance(RegisterDto, { email: 'buyer@example.com', name: 'Buyer', password: 'Password123!' });
    expect(await validate(input)).toHaveLength(0);
  });

  it('rejects duplicate seats and oversized carts', async () => {
    const duplicate = '21a7a66a-0e02-4e2d-8144-2a272c6cfb63';
    const input = plainToInstance(CreateReservationDto, {
      eventId: 'fe5b46bc-a5b2-4b7a-af28-aab142c9de53',
      seatIds: Array.from({ length: 11 }, () => duplicate),
    });
    const errors = await validate(input);
    expect(errors.some((error) => error.property === 'seatIds')).toBe(true);
  });
});
