import {
  Column,
  CreateDateColumn,
  Entity,
  Index,
  PrimaryGeneratedColumn,
  Unique,
  UpdateDateColumn,
} from 'typeorm';

export enum Role {
  CUSTOMER = 'CUSTOMER',
  ORGANIZER = 'ORGANIZER',
  ADMIN = 'ADMIN',
}

export enum SeatState {
  AVAILABLE = 'AVAILABLE',
  LOCKED = 'LOCKED',
  BOOKED = 'BOOKED',
}

export enum ReservationState {
  PENDING = 'PENDING',
  CONFIRMED = 'CONFIRMED',
  CANCELLED = 'CANCELLED',
  EXPIRED = 'EXPIRED',
}

export enum PaymentState {
  PENDING = 'PENDING',
  SUCCESS = 'SUCCESS',
  FAILED = 'FAILED',
  TIMEOUT = 'TIMEOUT',
  CANCELLED = 'CANCELLED',
}

export enum WaitingRoomState {
  QUEUED = 'QUEUED',
  ADMITTED = 'ADMITTED',
  EXPIRED = 'EXPIRED',
}

@Entity()
@Unique(['email'])
export class User {
  @PrimaryGeneratedColumn('uuid') id!: string;
  @Column() email!: string;
  @Column({ default: '' }) name!: string;
  @Column({ select: false }) passwordHash!: string;
  @Column({ type: 'enum', enum: Role, default: Role.CUSTOMER }) role!: Role;
  @CreateDateColumn() createdAt!: Date;
}

@Entity()
export class Venue {
  @PrimaryGeneratedColumn('uuid') id!: string;
  @Column() name!: string;
  @Index() @Column() city!: string;
  @Column({ default: '' }) address!: string;
  @CreateDateColumn() createdAt!: Date;
}

@Entity()
@Unique(['venueId', 'name'])
export class Sector {
  @PrimaryGeneratedColumn('uuid') id!: string;
  @Index() @Column() venueId!: string;
  @Column() name!: string;
}

@Entity()
@Unique(['sectorId', 'row', 'number'])
export class Seat {
  @PrimaryGeneratedColumn('uuid') id!: string;
  @Index() @Column() sectorId!: string;
  @Column() row!: string;
  @Column() number!: number;
  @Column({ default: false }) accessible!: boolean;
}

@Entity()
@Index(['published', 'startsAt'])
export class Event {
  @PrimaryGeneratedColumn('uuid') id!: string;
  @Index() @Column() organizerId!: string;
  @Index() @Column() venueId!: string;
  @Column() title!: string;
  @Column({ type: 'text', default: '' }) description!: string;
  @Index() @Column() genre!: string;
  @Index() @Column() city!: string;
  @Column({ type: 'simple-array', default: '' }) tags!: string[];
  @Column('timestamptz') startsAt!: Date;
  @Column('timestamptz', { nullable: true }) endsAt?: Date;
  @Column({ default: false }) published!: boolean;
  @CreateDateColumn() createdAt!: Date;
  @UpdateDateColumn() updatedAt!: Date;
}

@Entity()
@Unique(['eventId', 'sectorId', 'name'])
export class PricingCategory {
  @PrimaryGeneratedColumn('uuid') id!: string;
  @Index() @Column() eventId!: string;
  @Column({ nullable: true }) sectorId?: string;
  @Column() name!: string;
  @Column('decimal', { precision: 14, scale: 0 }) price!: number;
  @Column({ default: 'IRR' }) currency!: string;
}

@Entity()
@Index(['eventId', 'state'])
export class Reservation {
  @PrimaryGeneratedColumn('uuid') id!: string;
  @Index() @Column() userId!: string;
  @Index() @Column() eventId!: string;
  @Column({ type: 'enum', enum: ReservationState }) state!: ReservationState;
  @Column('timestamptz') expiresAt!: Date;
  @Column('decimal', { precision: 14, scale: 0, default: 0 }) totalAmount!: number;
  @Column({ default: 'IRR' }) currency!: string;
  @CreateDateColumn() createdAt!: Date;
  @UpdateDateColumn() updatedAt!: Date;
}

@Entity()
@Unique(['eventId', 'seatId'])
@Index(['reservationId', 'state'])
export class ReservationSeat {
  @PrimaryGeneratedColumn('uuid') id!: string;
  @Index() @Column() reservationId!: string;
  @Index() @Column() eventId!: string;
  @Index() @Column() seatId!: string;
  @Column({ type: 'enum', enum: SeatState }) state!: SeatState;
  @Column('decimal', { precision: 14, scale: 0, default: 0 }) price!: number;
}

@Entity()
@Unique(['reservationId'])
@Unique(['idempotencyKey'])
export class Payment {
  @PrimaryGeneratedColumn('uuid') id!: string;
  @Index() @Column() reservationId!: string;
  @Column({ nullable: true }) idempotencyKey?: string;
  @Column({ type: 'enum', enum: PaymentState }) state!: PaymentState;
  @Column('decimal', { precision: 14, scale: 0, default: 0 }) amount!: number;
  @Column({ default: 'IRR' }) currency!: string;
  @Column({ nullable: true }) reference?: string;
  @Column({ nullable: true }) failureReason?: string;
  @CreateDateColumn() createdAt!: Date;
  @UpdateDateColumn() updatedAt!: Date;
}

@Entity()
@Unique(['token'])
@Unique(['reservationId', 'seatId'])
export class Ticket {
  @PrimaryGeneratedColumn('uuid') id!: string;
  @Index() @Column() reservationId!: string;
  @Column() seatId!: string;
  @Column() token!: string;
  @Column() qrHash!: string;
  @Column('timestamptz', { nullable: true }) checkedInAt?: Date;
  @CreateDateColumn() issuedAt!: Date;
}

@Entity()
@Index(['userId', 'createdAt'])
export class Notification {
  @PrimaryGeneratedColumn('uuid') id!: string;
  @Column() userId!: string;
  @Column() channel!: string;
  @Column() message!: string;
  @Column({ default: false }) sent!: boolean;
  @Column({ nullable: true }) providerReference?: string;
  @Column({ nullable: true }) error?: string;
  @Column('timestamptz', { nullable: true }) sentAt?: Date;
  @CreateDateColumn() createdAt!: Date;
}

@Entity()
@Index(['eventId', 'state', 'position'])
export class WaitingRoomEntry {
  @PrimaryGeneratedColumn('uuid') id!: string;
  @Index() @Column() eventId!: string;
  @Index() @Column() userId!: string;
  @Column() position!: number;
  @Column({ type: 'enum', enum: WaitingRoomState, default: WaitingRoomState.QUEUED }) state!: WaitingRoomState;
  @Column({ nullable: true, unique: true }) admissionToken?: string;
  @Column('timestamptz', { nullable: true }) tokenExpiresAt?: Date;
  @CreateDateColumn() createdAt!: Date;
}
