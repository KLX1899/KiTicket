import { JwtService } from '@nestjs/jwt';
import { WebSocketGateway, WebSocketServer } from '@nestjs/websockets';
import { Server, Socket } from 'socket.io';
import { AuthenticatedUser } from './security';

const uuidPattern = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

@WebSocketGateway({ cors: { origin: process.env.CORS_ORIGIN?.split(',') ?? '*' } })
export class UpdatesGateway {
  @WebSocketServer() server?: Server;

  constructor(private readonly jwt: JwtService) {}

  emitUser(userId: string, name: string, data: unknown): void {
    this.server?.to(`user:${userId}`).emit(name, data);
  }

  emitEvent(eventId: string, name: string, data: unknown): void {
    this.server?.to(`event:${eventId}`).emit(name, data);
  }

  handleConnection(socket: Socket): void {
    const token = socket.handshake.auth?.token;
    if (typeof token !== 'string') {
      socket.disconnect(true);
      return;
    }
    try {
      const user = this.jwt.verify<AuthenticatedUser>(token);
      void socket.join(`user:${user.sub}`);
      socket.on('event.subscribe', (eventId: unknown) => {
        if (typeof eventId === 'string' && uuidPattern.test(eventId)) void socket.join(`event:${eventId}`);
      });
      socket.on('event.unsubscribe', (eventId: unknown) => {
        if (typeof eventId === 'string' && uuidPattern.test(eventId)) void socket.leave(`event:${eventId}`);
      });
    } catch {
      socket.disconnect(true);
    }
  }
}
