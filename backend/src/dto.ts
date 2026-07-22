import { Type } from 'class-transformer';
import {
  ArrayMaxSize,
  ArrayMinSize,
  ArrayUnique,
  IsArray,
  IsBoolean,
  IsDateString,
  IsEmail,
  IsEnum,
  IsInt,
  IsNumber,
  IsOptional,
  IsString,
  IsUUID,
  Length,
  Matches,
  Max,
  MaxLength,
  Min,
  MinLength,
} from 'class-validator';
import { PaymentState, Role } from './entities';

export class RegisterDto {
  @IsEmail() email!: string;
  @IsString() @MinLength(2) @MaxLength(80) name!: string;
  @IsString() @MinLength(10) @MaxLength(72)
  @Matches(/^(?=.*[a-z])(?=.*[A-Z])(?=.*\d).+$/, { message: 'password must include upper, lower and numeric characters' })
  password!: string;
}

export class LoginDto {
  @IsEmail() email!: string;
  @IsString() @MinLength(1) password!: string;
}

export class ChangeRoleDto {
  @IsEnum(Role) role!: Role;
}

export class CreateVenueDto {
  @IsString() @Length(2, 120) name!: string;
  @IsString() @Length(2, 80) city!: string;
  @IsString() @MaxLength(300) address!: string;
}

export class CreateSectorDto {
  @IsString() @Length(1, 80) name!: string;
  @Type(() => Number) @IsInt() @Min(1) @Max(52) rows!: number;
  @Type(() => Number) @IsInt() @Min(1) @Max(200) seatsPerRow!: number;
  @IsOptional() @IsBoolean() accessibleFirstRow?: boolean;
}

export class CreateEventDto {
  @IsUUID() venueId!: string;
  @IsString() @Length(2, 180) title!: string;
  @IsString() @MaxLength(5000) description!: string;
  @IsString() @Length(2, 80) genre!: string;
  @IsString() @Length(2, 80) city!: string;
  @IsOptional() @IsArray() @ArrayMaxSize(20) @IsString({ each: true }) tags?: string[];
  @IsDateString() startsAt!: string;
  @IsOptional() @IsDateString() endsAt?: string;
}

export class UpdateEventDto {
  @IsOptional() @IsString() @Length(2, 180) title?: string;
  @IsOptional() @IsString() @MaxLength(5000) description?: string;
  @IsOptional() @IsString() genre?: string;
  @IsOptional() @IsString() city?: string;
  @IsOptional() @IsArray() @ArrayMaxSize(20) @IsString({ each: true }) tags?: string[];
  @IsOptional() @IsDateString() startsAt?: string;
  @IsOptional() @IsDateString() endsAt?: string;
}

export class CreatePricingDto {
  @IsOptional() @IsUUID() sectorId?: string;
  @IsString() @Length(1, 80) name!: string;
  @Type(() => Number) @IsNumber() @Min(0) price!: number;
  @IsOptional() @IsString() @Length(3, 3) currency?: string;
}

export class EventQueryDto {
  @IsOptional() @IsString() @MaxLength(120) q?: string;
  @IsOptional() @IsString() genre?: string;
  @IsOptional() @IsString() city?: string;
  @IsOptional() @IsDateString() from?: string;
  @IsOptional() @IsDateString() to?: string;
  @IsOptional() @Type(() => Boolean) @IsBoolean() available?: boolean;
  @IsOptional() @Type(() => Number) @IsInt() @Min(1) page = 1;
  @IsOptional() @Type(() => Number) @IsInt() @Min(1) @Max(100) limit = 20;
}

export class CreateReservationDto {
  @IsUUID() eventId!: string;
  @IsArray() @ArrayMinSize(1) @ArrayMaxSize(10) @ArrayUnique() @IsUUID('4', { each: true }) seatIds!: string[];
  @IsOptional() @IsString() @MinLength(20) admissionToken?: string;
}

export class StartPaymentDto {
  @IsUUID() reservationId!: string;
  @IsString() @Length(8, 120) idempotencyKey!: string;
}

export enum PaymentOutcome {
  SUCCESS = 'success',
  FAILURE = 'failure',
  TIMEOUT = 'timeout',
}

export class CompletePaymentDto {
  @IsEnum(PaymentOutcome) outcome!: PaymentOutcome;
  @IsOptional() @IsString() @MaxLength(120) providerReference?: string;
}

export class AdmitUsersDto {
  @Type(() => Number) @IsInt() @Min(1) @Max(500) count!: number;
}

export class PaymentQueryDto {
  @IsOptional() @IsEnum(PaymentState) state?: PaymentState;
}
