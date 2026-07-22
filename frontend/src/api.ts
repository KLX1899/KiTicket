export const API_URL = import.meta.env.VITE_API_URL ?? "/api";

export type Role = "CUSTOMER" | "ORGANIZER" | "ADMIN";
export type SeatState = "AVAILABLE" | "LOCKED" | "BOOKED";
export type WaitingRoomState = "QUEUED" | "ADMITTED" | "EXPIRED";
export type PaymentOutcome = "success" | "failure" | "timeout";

export interface UserSummary {
  id: string;
  email: string;
  name: string;
  role: Role;
}

export interface AuthResponse {
  accessToken: string;
  expiresIn: number;
  user: UserSummary;
}

export interface Paginated<T> {
  items: T[];
  page: number;
  limit: number;
  total: number;
  pages: number;
}

export interface EventSummary {
  id: string;
  title: string;
  genre: string;
  startsAt: string;
  endsAt?: string;
  city: string;
  description?: string;
  tags?: string[];
}

export interface InventorySeat {
  id: string;
  seatId: string;
  sectorId: string;
  row: string;
  number: number;
  accessible: boolean;
  state: SeatState;
  price: number;
  currency: string;
}

export interface EventDetail extends EventSummary {
  published: boolean;
  bookable: boolean;
  venue: { id: string; name: string; city: string; address: string };
  availability: number;
  inventory: InventorySeat[];
}

export interface Venue {
  id: string;
  name: string;
  city: string;
  address: string;
}

export interface WaitingRoomEntry {
  id: string;
  eventId: string;
  position: number;
  state: WaitingRoomState;
  admissionToken?: string;
  tokenExpiresAt?: string;
}

export interface Reservation {
  id: string;
  userId: string;
  eventId: string;
  state: "PENDING" | "CONFIRMED" | "CANCELLED" | "EXPIRED";
  expiresAt: string;
  totalAmount: number;
  currency: string;
}

export interface Payment {
  id: string;
  reservationId: string;
  state: "PENDING" | "SUCCESS" | "FAILED" | "TIMEOUT" | "CANCELLED";
  amount: number;
  currency: string;
  reference?: string;
}

export interface Ticket {
  id: string;
  reservationId: string;
  seatId: string;
  issuedAt: string;
  checkedInAt?: string;
}

export interface StoredTicket extends Ticket {
  qrDataUrl?: string;
  eventTitle?: string;
  eventStartsAt?: string;
  seatLabel?: string;
}

export interface EventAnalytics {
  reservations: number;
  confirmedReservations: number;
  bookedSeats: number;
  remainingSeats: number;
  capacity: number;
  revenue: number;
  currency: string;
}

export class ApiError extends Error {
  constructor(public status: number, message: string, public retryable = false) {
    super(message);
    this.name = "ApiError";
  }
}

export const AUTH_EXPIRED_EVENT = "narm:auth-expired";
const DEFAULT_TIMEOUT_MS = 15_000;
const transientStatuses = new Set([408, 502, 503, 504]);

function errorMessage(body: unknown): string {
  if (typeof body === "string" && body.trim()) return body;
  if (!body || typeof body !== "object") return "درخواست ناموفق بود";
  const value = body as { message?: unknown; detail?: unknown };
  if (Array.isArray(value.message)) return value.message.map(String).join("، ");
  if (typeof value.message === "string") return value.message;
  if (typeof value.detail === "string") return value.detail;
  return "درخواست ناموفق بود";
}

async function responseBody(response: Response): Promise<unknown> {
  if (response.status === 204) return undefined;
  const text = await response.text();
  if (!text) return undefined;
  try {
    return JSON.parse(text) as unknown;
  } catch {
    return text;
  }
}

function wait(milliseconds: number): Promise<void> {
  return new Promise((resolve) => window.setTimeout(resolve, milliseconds));
}

export async function api<T>(
  path: string,
  options: RequestInit = {},
  token?: string | null,
): Promise<T> {
  const method = (options.method ?? "GET").toUpperCase();
  const canRetry = method === "GET" || method === "HEAD";
  const attempts = canRetry ? 2 : 1;
  for (let attempt = 0; attempt < attempts; attempt += 1) {
    const controller = new AbortController();
    let timedOut = false;
    const abortFromCaller = () => controller.abort(options.signal?.reason);
    if (options.signal?.aborted) abortFromCaller();
    else options.signal?.addEventListener("abort", abortFromCaller, { once: true });
    const timeout = window.setTimeout(() => {
      timedOut = true;
      controller.abort();
    }, DEFAULT_TIMEOUT_MS);
    try {
      const headers = new Headers(options.headers);
      if (options.body && !(options.body instanceof FormData) && !headers.has("Content-Type")) {
        headers.set("Content-Type", "application/json");
      }
      if (token) headers.set("Authorization", `Bearer ${token}`);
      const response = await fetch(`${API_URL}${path}`, {
        ...options,
        headers,
        signal: controller.signal,
      });
      const body = await responseBody(response);
      if (!response.ok) {
        if (response.status === 401 && token) window.dispatchEvent(new Event(AUTH_EXPIRED_EVENT));
        const retryable = transientStatuses.has(response.status);
        if (retryable && attempt + 1 < attempts) {
          await wait(300 * (attempt + 1));
          continue;
        }
        throw new ApiError(response.status, errorMessage(body), retryable);
      }
      return body as T;
    } catch (caught) {
      if (caught instanceof ApiError) throw caught;
      if (options.signal?.aborted) throw caught;
      const error = new ApiError(
        timedOut ? 408 : 0,
        timedOut
          ? "زمان پاسخ‌گویی سرور تمام شد؛ دوباره تلاش کنید."
          : "ارتباط با سرور برقرار نشد؛ اتصال اینترنت را بررسی کنید.",
        true,
      );
      if (attempt + 1 >= attempts) throw error;
      await wait(300 * (attempt + 1));
    } finally {
      window.clearTimeout(timeout);
      options.signal?.removeEventListener("abort", abortFromCaller);
    }
  }
  throw new ApiError(0, "درخواست ناموفق بود");
}

const volatileIdempotencyKeys = new Map<string, string>();

export function idempotencyKey(scope: string): string {
  const storageKey = `narm-idempotency-${scope}`;
  let existing: string | null = null;
  try {
    existing = sessionStorage.getItem(storageKey);
  } catch {
    existing = volatileIdempotencyKeys.get(storageKey) ?? null;
  }
  if (existing) return existing;
  const random = crypto.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(16).slice(2)}`;
  const generated = `${scope}-${random}`;
  volatileIdempotencyKeys.set(storageKey, generated);
  try {
    sessionStorage.setItem(storageKey, generated);
  } catch {
    // The in-memory key still prevents duplicate payment requests in this tab.
  }
  return generated;
}
