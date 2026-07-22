import { CanActivate, ExecutionContext, Injectable, SetMetadata, UnauthorizedException } from '@nestjs/common';
import { Reflector } from '@nestjs/core';
import { JwtService } from '@nestjs/jwt';
import { Role } from './entities';

export interface AuthenticatedUser {
  sub: string;
  email: string;
  role: Role;
}

export const Roles = (...roles: Role[]) => SetMetadata('roles', roles);

@Injectable()
export class AuthGuard implements CanActivate {
  constructor(private readonly jwt: JwtService, private readonly reflector: Reflector) {}

  canActivate(context: ExecutionContext): boolean {
    const request = context.switchToHttp().getRequest<{ headers: { authorization?: string }; user?: AuthenticatedUser }>();
    const header = request.headers.authorization;
    if (!header?.startsWith('Bearer ')) throw new UnauthorizedException('Bearer token is required');
    try {
      const user = this.jwt.verify<AuthenticatedUser>(header.slice(7));
      if (!user.sub || !user.email || !Object.values(Role).includes(user.role)) throw new Error('Invalid claims');
      request.user = user;
    } catch {
      throw new UnauthorizedException('Token is invalid or expired');
    }
    const roles = this.reflector.getAllAndOverride<Role[]>('roles', [context.getHandler(), context.getClass()]);
    return !roles || roles.includes(request.user.role);
  }
}
