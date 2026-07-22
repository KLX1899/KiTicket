import { ForbiddenException } from '@nestjs/common';
import { WaitingRoomEntry, WaitingRoomState } from './entities';
import { WaitingRoomService } from './waiting-room.controller';

describe('WaitingRoomService admission enforcement', () => {
  const original = process.env.WAITING_ROOM_REQUIRED;
  afterAll(() => { process.env.WAITING_ROOM_REQUIRED = original; });

  it('accepts only an unexpired token for the same event and user', async () => {
    process.env.WAITING_ROOM_REQUIRED = 'true';
    const entry = { tokenExpiresAt: new Date(Date.now() + 60_000) } as WaitingRoomEntry;
    const repository = { findOneBy: jest.fn().mockResolvedValue(entry) };
    const service = new WaitingRoomService(repository as never);
    await expect(service.assertAdmission('event', 'user', 'valid-token-value-with-length')).resolves.toBeUndefined();
    expect(repository.findOneBy).toHaveBeenCalledWith(expect.objectContaining({ state: WaitingRoomState.ADMITTED }));
  });

  it('rejects missing and expired admission tokens', async () => {
    process.env.WAITING_ROOM_REQUIRED = 'true';
    const repository = { findOneBy: jest.fn().mockResolvedValue({ tokenExpiresAt: new Date(Date.now() - 1) }) };
    const service = new WaitingRoomService(repository as never);
    await expect(service.assertAdmission('event', 'user')).rejects.toBeInstanceOf(ForbiddenException);
    await expect(service.assertAdmission('event', 'user', 'expired')).rejects.toBeInstanceOf(ForbiddenException);
  });
});
