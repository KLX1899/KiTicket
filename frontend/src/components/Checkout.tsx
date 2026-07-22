import { useEffect, useState } from "react";
import { Navigate, useNavigate, useParams } from "react-router-dom";
import {
  api,
  idempotencyKey,
  type InventorySeat,
  type Payment,
  type PaymentOutcome,
  type Reservation,
  type StoredTicket,
  type Ticket,
} from "../api";
import { useAuth } from "../auth";
import Notice from "./Notice";

interface StoredReservation extends Reservation {
  eventTitle?: string;
  seats?: InventorySeat[];
  total?: number;
}

function readReservation(): StoredReservation | null {
  try {
    const raw = sessionStorage.getItem("narm-reservation");
    return raw ? JSON.parse(raw) as StoredReservation : null;
  } catch {
    return null;
  }
}

function Checkout() {
  const { reservationId } = useParams(), { session } = useAuth(), navigate = useNavigate();
  const [outcome, setOutcome] = useState<PaymentOutcome>("success"),
    [loading, setLoading] = useState(false),
    [terminal, setTerminal] = useState(false),
    [error, setError] = useState(""),
    [now, setNow] = useState(Date.now()),
    [reservation, setReservation] = useState<StoredReservation | null>(readReservation),
    [checkingReservation, setCheckingReservation] = useState(true);

  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, []);

  useEffect(() => {
    if (!session || !reservationId) { setCheckingReservation(false); return; }
    if (reservation?.id === reservationId) { setCheckingReservation(false); return; }
    let active = true;
    const controller = new AbortController();
    api<StoredReservation>(`/reservations/${reservationId}`, { signal: controller.signal }, session.token)
      .then((result) => { if (active) setReservation(result); })
      .catch((caught) => { if (active) setError((caught as Error).message); })
      .finally(() => { if (active) setCheckingReservation(false); });
    return () => { active = false; controller.abort(); };
  }, [reservation?.id, reservationId, session]);

  if (!session) return <Navigate to="/login" />;
  if (checkingReservation) return <div className="loader">در حال بازیابی رزرو…</div>;
  if (!reservation || reservation.id !== reservationId) return (
    <section className="checkout-page"><div className="payment-card">
      <Notice text={error || "رزرو موردنظر پیدا نشد یا دیگر قابل پرداخت نیست."} />
      <button className="button wide" onClick={() => navigate("/")}>بازگشت به رویدادها</button>
    </div></section>
  );
  const activeSession = session;
  const activeReservation = reservation;

  const remainingSeconds = Math.max(0, Math.ceil((new Date(activeReservation.expiresAt).getTime() - now) / 1000));
  const minutes = Math.floor(remainingSeconds / 60).toLocaleString("fa-IR", { minimumIntegerDigits: 2 });
  const seconds = (remainingSeconds % 60).toLocaleString("fa-IR", { minimumIntegerDigits: 2 });

  async function pay() {
    if (!reservationId || terminal) return;
    setLoading(true);
    setError("");
    try {
      const payment = await api<Payment>(
        "/payments",
        {
          method: "POST",
          body: JSON.stringify({ reservationId, idempotencyKey: idempotencyKey(reservationId) }),
        },
        activeSession.token,
      );
      const result = await api<{ payment: Payment; reservation: Reservation; tickets: Ticket[] }>(
        `/payments/${payment.id}/complete`,
        { method: "POST", body: JSON.stringify({ outcome }) },
        activeSession.token,
      );
      if (result.payment.state === "SUCCESS") {
        setTerminal(true);
        const stored = await Promise.all(result.tickets.map(async (ticket): Promise<StoredTicket> => {
          const seat = activeReservation.seats?.find((candidate) => candidate.id === ticket.seatId);
          let qrDataUrl: string | undefined;
          try {
            const qr = await api<{ ticketId: string; qrDataUrl: string }>(`/tickets/${ticket.id}/qr`, {}, activeSession.token);
            qrDataUrl = qr.qrDataUrl;
          } catch {
            // The ticket is already issued; the wallet page can retry loading its QR code.
          }
          return {
            ...ticket,
            qrDataUrl,
            eventTitle: activeReservation.eventTitle,
            seatLabel: seat ? `ردیف ${seat.row}، صندلی ${seat.number}` : undefined,
          };
        }));
        const walletKey = `narm-tickets-${activeSession.id}`;
        let existing: StoredTicket[] = [];
        try {
          existing = JSON.parse(localStorage.getItem(walletKey) ?? "[]") as StoredTicket[];
        } catch {
          existing = [];
        }
        const merged = [...existing.filter((ticket) => !stored.some((next) => next.id === ticket.id)), ...stored];
        try { localStorage.setItem(walletKey, JSON.stringify(merged)); } catch { /* server remains the source of truth */ }
        try { sessionStorage.removeItem("narm-reservation"); } catch { /* storage may be unavailable */ }
        navigate("/tickets");
        return;
      }
      setTerminal(true);
      setError(result.payment.state === "TIMEOUT"
        ? "زمان رزرو تمام شد؛ پرداخت ثبت نشد و صندلی‌ها آزاد شدند."
        : "پرداخت ناموفق بود؛ مبلغی نهایی نشده و صندلی‌ها آزاد شدند.");
    } catch (caught) {
      setError((caught as Error).message);
    } finally {
      setLoading(false);
    }
  }

  async function cancel() {
    if (!reservationId || terminal) return;
    setLoading(true);
    setError("");
    try {
      await api(`/reservations/${reservationId}/cancel`, { method: "POST" }, activeSession.token);
      try { sessionStorage.removeItem("narm-reservation"); } catch { /* storage may be unavailable */ }
      navigate(`/events/${activeReservation.eventId}`);
    } catch (caught) {
      setError((caught as Error).message);
    } finally {
      setLoading(false);
    }
  }

  return (
    <section className="checkout-page">
      <div className="payment-card">
        <span className="eyebrow">درگاه آزمایشی و idempotent</span>
        <h1>تکمیل پرداخت</h1>
        <div className={`countdown ${remainingSeconds < 60 ? "urgent" : ""}`} aria-live="polite">
          زمان باقی‌مانده رزرو: <b>{minutes}:{seconds}</b>
        </div>
        <div className="payment-summary">
          <span>{(activeReservation.seats?.length ?? 0).toLocaleString("fa-IR")} × {activeReservation.eventTitle ?? "رزرو بلیت"}</span>
          <strong>{Number(activeReservation.total ?? activeReservation.totalAmount ?? 0).toLocaleString("fa-IR")} {activeReservation.currency ?? "IRR"}</strong>
        </div>
        {error && <Notice text={error} />}
        <label>
          سناریوی درگاه sandbox
          <select value={outcome} onChange={(event) => setOutcome(event.target.value as PaymentOutcome)} disabled={terminal}>
            <option value="success">پرداخت موفق</option>
            <option value="failure">رد قطعی پرداخت</option>
            <option value="timeout">عدم پاسخ تا پایان مهلت</option>
          </select>
        </label>
        <div className="fake-card" aria-label="کارت آزمایشی؛ اطلاعات واقعی وارد نکنید">
          <small>شماره کارت آزمایشی</small>
          <b>۱۲۳۴ **** **** ۴۳۲۱</b>
          <div><span>CVV2<br /><b>۱۲۳</b></span><span>تاریخ<br /><b>۰۹ / ۰۸</b></span></div>
        </div>
        <button className="button wide" onClick={pay} disabled={loading || terminal || remainingSeconds === 0}>
          {loading ? "در حال پردازش…" : terminal ? "فرایند پایان یافته" : "پرداخت و صدور بلیت"}
        </button>
        <button type="button" className="text-button" onClick={cancel} disabled={loading || terminal}>انصراف و آزادسازی صندلی‌ها</button>
      </div>
    </section>
  );
}

export default Checkout;
