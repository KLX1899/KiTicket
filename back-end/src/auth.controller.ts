import { Body, ConflictException, Controller, Get, Param, Patch, Post, Req, UnauthorizedException, UseGuards } from '@nestjs/common';
import { JwtService } from '@nestjs/jwt';
import { InjectRepository } from '@nestjs/typeorm';
import * as bcrypt from 'bcrypt';
import { QueryFailedError, Repository } from 'typeorm';
import { ChangeRoleDto, LoginDto, RegisterDto } from './dto';
import { Role, User } from './entities';
import { AuthenticatedUser, AuthGuard, Roles } from './security';

@Controller('auth')
export class AuthController {
  constructor(@InjectRepository(User) private readonly users: Repository<User>, private readonly jwt: JwtService) {}

  @Post('register')
  async register(@Body() body: RegisterDto) {
    const email = body.email.trim().toLowerCase();
    if (await this.users.exist({ where: { email } })) throw new ConflictException('Email is already registered');
    let user: User;
    try {
      user = await this.users.save(this.users.create({
        email,
        name: body.name.trim(),
        passwordHash: await bcrypt.hash(body.password, 12),
        role: Role.CUSTOMER,
      }));
    } catch (error) {
      if (error instanceof QueryFailedError && (error.driverError as { code?: string }).code === '23505') {
        throw new ConflictException('Email is already registered');
      }
      throw error;
    }
    return { id: user.id, email: user.email, name: user.name, role: user.role };
  }

  @Post('login')
  async login(@Body() body: LoginDto) {
    const user = await this.users.createQueryBuilder('user').addSelect('user.passwordHash')
      .where('LOWER(user.email) = LOWER(:email)', { email: body.email.trim() }).getOne();
    if (!user || !await bcrypt.compare(body.password, user.passwordHash)) {
      throw new UnauthorizedException('ایمیل یا رمز عبور نادرست است');
    }
    const claims: AuthenticatedUser = { sub: user.id, email: user.email, role: user.role };
    return {
      accessToken: this.jwt.sign(claims),
      expiresIn: 3600,
      user: { id: user.id, email: user.email, name: user.name, role: user.role },
    };
  }

  @Get('me')
  @UseGuards(AuthGuard)
  me(@Req() request: { user: AuthenticatedUser }) { return request.user; }
}

@Controller('users')
@UseGuards(AuthGuard)
@Roles(Role.ADMIN)
export class UsersController {
  constructor(@InjectRepository(User) private readonly users: Repository<User>) {}

  @Get()
  list() { return this.users.find({ select: ['id', 'email', 'name', 'role', 'createdAt'] }); }

  @Patch(':id/role')
  async changeRole(@Param('id') id: string, @Body() body: ChangeRoleDto) {
    await this.users.update(id, { role: body.role });
    return this.users.findOneByOrFail({ id });
  }
}
