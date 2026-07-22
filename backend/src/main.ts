import { Logger, ValidationPipe } from '@nestjs/common';
import { NestFactory } from '@nestjs/core';
import { DocumentBuilder, SwaggerModule } from '@nestjs/swagger';
import helmet from 'helmet';
import { AppModule } from './app.module';
import { ApiExceptionFilter } from './http-exception.filter';

function validateEnvironment(): void {
  if (process.env.NODE_ENV === 'production') {
    if (!process.env.JWT_SECRET || process.env.JWT_SECRET.length < 32) throw new Error('JWT_SECRET must contain at least 32 characters in production');
    if (process.env.DB_SYNCHRONIZE === 'true') throw new Error('DB_SYNCHRONIZE must be disabled in production');
  }
}

async function bootstrap() {
  validateEnvironment();
  const app = await NestFactory.create(AppModule, { bufferLogs: true });
  app.useLogger(new Logger('TicketingAPI'));
  app.use(helmet({ crossOriginResourcePolicy: false }));
  app.enableCors({
    origin: process.env.CORS_ORIGIN?.split(',') ?? ['http://localhost:5173'],
    credentials: true,
    methods: ['GET', 'POST', 'PATCH', 'DELETE', 'OPTIONS'],
  });
  app.enableShutdownHooks();
  app.setGlobalPrefix('api');
  app.useGlobalFilters(new ApiExceptionFilter());
  app.useGlobalPipes(new ValidationPipe({
    whitelist: true,
    forbidNonWhitelisted: true,
    transform: true,
    transformOptions: { enableImplicitConversion: false },
  }));
  const config = new DocumentBuilder()
    .setTitle('Event Ticketing API')
    .setDescription('Concurrency-safe booking, waiting-room, checkout and ticketing API')
    .setVersion('2.0')
    .addBearerAuth()
    .build();
  SwaggerModule.setup('api/docs', app, SwaggerModule.createDocument(app, config));
  const port = Number(process.env.PORT ?? 3000);
  await app.listen(port, '0.0.0.0');
  Logger.log(`Ticketing API listening on ${port}`, 'Bootstrap');
}

void bootstrap();
