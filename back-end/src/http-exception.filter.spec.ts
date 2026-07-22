import { ArgumentsHost, BadRequestException } from '@nestjs/common';
import { EntityNotFoundError, QueryFailedError } from 'typeorm';
import { ApiExceptionFilter } from './http-exception.filter';

function httpContext() {
  const response = {
    setHeader: jest.fn(),
    status: jest.fn().mockReturnThis(),
    json: jest.fn(),
  };
  const request = { headers: {}, method: 'GET', url: '/api/test' };
  const host = {
    switchToHttp: () => ({
      getRequest: () => request,
      getResponse: () => response,
    }),
  } as ArgumentsHost;
  return { host, response };
}

describe('ApiExceptionFilter', () => {
  it('turns missing TypeORM entities into a stable 404 response', () => {
    const { host, response } = httpContext();
    new ApiExceptionFilter().catch(new EntityNotFoundError('Reservation', { id: 'missing' }), host);
    expect(response.status).toHaveBeenCalledWith(404);
    expect(response.json).toHaveBeenCalledWith(expect.objectContaining({
      statusCode: 404,
      message: 'رکورد موردنظر پیدا نشد.',
      requestId: expect.any(String),
    }));
  });

  it('maps database uniqueness races to conflict instead of server error', () => {
    const { host, response } = httpContext();
    const driverError = Object.assign(new Error('duplicate'), { code: '23505' });
    new ApiExceptionFilter().catch(new QueryFailedError('INSERT', [], driverError), host);
    expect(response.status).toHaveBeenCalledWith(409);
    expect(response.json).toHaveBeenCalledWith(expect.objectContaining({ statusCode: 409 }));
  });

  it('keeps validation error details and adds request diagnostics', () => {
    const { host, response } = httpContext();
    new ApiExceptionFilter().catch(new BadRequestException(['email must be an email']), host);
    expect(response.status).toHaveBeenCalledWith(400);
    expect(response.json).toHaveBeenCalledWith(expect.objectContaining({
      message: ['email must be an email'],
      path: '/api/test',
      timestamp: expect.any(String),
    }));
  });
});
