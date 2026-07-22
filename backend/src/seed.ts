import 'reflect-metadata';
import { Logger } from '@nestjs/common';
import * as bcrypt from 'bcrypt';
import { DataSource } from 'typeorm';
import { Event, Notification, Payment, PricingCategory, Reservation, ReservationSeat, Role, Seat, Sector, Ticket, User, Venue, WaitingRoomEntry } from './entities';

const entities=[User,Venue,Sector,Seat,Event,PricingCategory,Reservation,ReservationSeat,Payment,Ticket,Notification,WaitingRoomEntry];
const db=new DataSource({type:'postgres',url:process.env.DATABASE_URL??'postgres://ticketing:ticketing@localhost:5432/ticketing',entities,synchronize:true});

async function seed(){
  await db.initialize();
  const users=db.getRepository(User),venues=db.getRepository(Venue),sectors=db.getRepository(Sector),seats=db.getRepository(Seat),events=db.getRepository(Event),prices=db.getRepository(PricingCategory);
  const passwordHash=await bcrypt.hash('Password123!',12);
  const identities=[
    {email:'admin@KiTicket.local',role:Role.ADMIN},
    {email:'organizer@KiTicket.local',role:Role.ORGANIZER},
    {email:'customer@KiTicket.local',role:Role.CUSTOMER},
    {email:'buyer@KiTicket.local',role:Role.CUSTOMER},
  ];
  for(const identity of identities)if(!await users.findOneBy({email:identity.email}))await users.save(users.create({...identity,passwordHash}));
  const organizer=await users.findOneByOrFail({email:'organizer@KiTicket.local'});
  const samples=[
    ['کاربیت آن دِ بیت','Music','اجرای زنده ترک جدید کاربیت',0,3],
    ['نمایش این بود زندگی','Theatre','یادآوری خاطرات خوابگاه دانشجویی در راه فرودگاه',0,7],
    ['فینال بسکتبال NBA2K26','Sport','مسابقه پایانی لیگ بسکتبال-الکترونیکی زنده',1,10],
    ['مراسم Galexy Unpacked 26','Conference','رونمایی از جدیدترین محصولات شرکت سامسونگ',1,14],
    ['کنسرت آثار هانس زیمر','Music','شب فراموش‌نشدنی شنیدن آثار هانس زیمر',0,21],
  ] as const;
  if(await events.count()===0){
    const venueData=[{name:'تالار وحدت',city:'تهران',address:'خیابان حافظ'},{name:'برج میلاد',city:'تهران',address:'بزرگراه همت'}];
    const madeVenues:Venue[]=[];
    for(const data of venueData){const venue=await venues.save(venues.create(data));madeVenues.push(venue);const sector=await sectors.save(sectors.create({venueId:venue.id,name:'سالن اصلی'}));const generated:Seat[]=[];for(let r=0;r<8;r++)for(let n=1;n<=12;n++)generated.push(seats.create({sectorId:sector.id,row:String.fromCharCode(65+r),number:n}));await seats.save(generated);}
    for(const [title,genre,description,venueIndex,days] of samples){const venue=madeVenues[venueIndex];const event=await events.save(events.create({organizerId:organizer.id,venueId:venue.id,title,genre,description,city:venue.city,startsAt:new Date(Date.now()+days*86400000),published:true}));await prices.save(prices.create({eventId:event.id,name:'استاندارد',price:2500000}));}
  }
  for(const [title,,,,days] of samples){
    const event=await events.findOneBy({title});
    if(event&&event.startsAt<=new Date()){
      event.startsAt=new Date(Date.now()+days*86400000);
      await events.save(event);
    }
  }
  Logger.log('Seed complete: customer@KiTicket.local / Password123!', 'Seed');
  await db.destroy();
}
seed().catch(error=>{console.error(error);process.exit(1)});
