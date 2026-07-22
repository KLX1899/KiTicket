import {
  ArgumentsHost,
  Catch,
  ExceptionFilter,
  HttpException,
  HttpStatus,
  Logger,
} from '@nestjs/common';
import { randomUUID } from 'crypto';
import { Request, Response } from 'express';
import { EntityNotFoundError, QueryFailedError } from 'typeorm';

interface DatabaseError {
  code?: string;
}

@Catch()
export class ApiExceptionFilter implements ExceptionFilter {
  private readonly logger = new Logger(ApiExceptionFilter.name);

  catch(exception: unknown, host: ArgumentsHost): void {
    const context = host.switchToHttp();
    const request = context.getRequest<Request>();
    const response = context.getResponse<Response>();
    const requestId = String(request.headers['x-request-id'] ?? randomUUID());
    let status = HttpStatus.INTERNAL_SERVER_ERROR;
    let message: string | string[] = 'خطای داخلی سرور رخ داد.';
    let details: Record<string, unknown> = {};

    if (exception instanceof HttpException) {
      status = exception.getStatus();
      const body = exception.getResponse();
      if (typeof body === 'string') message = body;
      else {
        details = body as Record<string, unknown>;
        const provided = details.message;
        message = typeof provided === 'string' || Array.isArray(provided) ? provided as string | string[] : exception.message;
      }
    } else if (exception instanceof EntityNotFoundError) {
      status = HttpStatus.NOT_FOUND;
      message = 'رکورد موردنظر پیدا نشد.';
    } else if (exception instanceof QueryFailedError) {
      const code = (exception.driverError as DatabaseError | undefined)?.code;
      if (code === '23505') {
        status = HttpStatus.CONFLICT;
        message = 'این اطلاعات قبلاً ثبت شده است.';
      } else if (code === '23503') {
        status = HttpStatus.CONFLICT;
        message = 'اطلاعات وابسته معتبر نیست.';
      } else if (code === '22P02') {
        status = HttpStatus.BAD_REQUEST;
        message = 'شناسه یا مقدار ورودی نامعتبر است.';
      }
    }

    if (status >= 500) {
      const error = exception instanceof Error ? exception : new Error(String(exception));
      this.logger.error(`${request.method} ${request.url} failed [${requestId}]`, error.stack);
    }

    response.setHeader('X-Request-Id', requestId);
    response.status(status).json({
      ...details,
      statusCode: status,
      message,
      path: request.url,
      timestamp: new Date().toISOString(),
      requestId,
    });
  }
}
