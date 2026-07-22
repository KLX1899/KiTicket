import { useEffect, useMemo, useRef, useState } from "react";
import { useLocation, useNavigate, useParams, Link } from "react-router-dom";
import { CalendarDays, ChevronLeft, MapPin, ShieldCheck } from "lucide-react";
import { api, type EventDetail, type Reservation, type WaitingRoomEntry } from "../api";
import { useAuth } from "../auth";
import Notice from "./Notice";

function EventPage() {
  const { id } = useParams(),
    { session } = useAuth(),
    navigate = useNavigate(),
    location = useLocation(),
    resumed = useRef(false);
  const [event, setEvent] = useState<EventDetail | null>(null),
    [selected, setSelected] = useState<string[]>([]),
    [phase, setPhase] = useState<"view" | "waiting" | "reserving">("view"),
    [queuePosition, setQueuePosition] = useState<number | null>(null),
    [error, setError] = useState("");
  useEffect(() => {
    let active = true;
    let timer: number | undefined;
    let controller: AbortController | undefined;
    const refresh = async () => {
      controller = new AbortController();
      try {
        const result = await api<EventDetail>(`/events/${id}`, { signal: controller.signal });
        if (!active) return;
        setEvent(result);
        setError("");
        setSelected((current) => current.filter((seatId) => result.inventory.some((seat) => seat.id === seatId && seat.state === "AVAILABLE")));
      } catch (caught) {
        if (active) setError((caught as Error).message);
      } finally {
        if (active) timer = window.setTimeout(() => void refresh(), 10_000);
      }
    };
    void refresh();
    return () => {
      active = false;
      controller?.abort();
      window.clearTimeout(timer);
    };
  }, [id]);

  useEffect(() => {
    if (!session || !event || resumed.current || !(location.state as { resumeReservation?: boolean } | null)?.resumeReservation) return;
    resumed.current = true;
    navigate(location.pathname, { replace: true, state: null });
    try {
      const pending = JSON.parse(sessionStorage.getItem("narm-pending-reservation") ?? "null") as { eventId?: string; seatIds?: string[] } | null;
      if (!pending || pending.eventId !== id || !Array.isArray(pending.seatIds)) return;
      const available = pending.seatIds.filter((seatId) => event.inventory.some((seat) => seat.id === seatId && seat.state === "AVAILABLE"));
      if (!available.length) {
        setError("صندلی‌های انتخاب‌شده در این فاصله رزرو شده‌اند؛ لطفاً دوباره انتخاب کنید.");
        sessionStorage.removeItem("narm-pending-reservation");
        return;
      }
      setSelected(available);
      void reserve(available);
    } catch {
      setError("ادامه رزرو قبلی ممکن نبود؛ لطفاً صندلی‌ها را دوباره انتخاب کنید.");
    }
  }, [event, id, location.pathname, location.state, navigate, session]);
  const chosen = useMemo(
      () => event?.inventory.filter((x) => selected.includes(x.id)) ?? [],
      [event, selected]
    ),
    total = chosen.reduce((s, x) => s + Number(x.price), 0);
  function toggle(seatId: string) {
    setSelected((current) =>
      current.includes(seatId)
        ? current.filter((x) => x !== seatId)
        : current.length < 10
        ? [...current, seatId]
        : current
    );
  }
  async function reserve(seatIds = selected) {
    if (!session) {
      try { sessionStorage.setItem("narm-pending-reservation", JSON.stringify({ eventId: id, seatIds })); } catch { /* navigation state still preserves the return path */ }
      return navigate("/login", { state: { returnTo: `/events/${id}`, resumeReservation: true } });
    }
    setError("");
    setPhase("waiting");
    try {
      let admission = await api<WaitingRoomEntry>(
        `/waiting-room/${id}/join`,
        { method: "POST" },
        session.token
      );
      setQueuePosition(admission.position);
      for (let attempt = 0; admission.state === "QUEUED" && attempt < 60; attempt += 1) {
        await new Promise((resolve) => window.setTimeout(resolve, Math.min(5000, 1500 + attempt * 100)));
        admission = await api<WaitingRoomEntry>(`/waiting-room/${admission.id}`, {}, session.token);
        setQueuePosition(admission.position);
      }
      if (admission.state !== "ADMITTED" || !admission.admissionToken) {
        throw new Error("زمان انتظار طولانی شد؛ جایگاه شما حفظ شده و می‌توانید دوباره وضعیت را بررسی کنید.");
      }
      setPhase("reserving");
      const reservation = await api<Reservation>(
        "/reservations",
        {
          method: "POST",
          body: JSON.stringify({ eventId: id, seatIds, admissionToken: admission.admissionToken }),
        },
        session.token
      );
      sessionStorage.setItem(
        "narm-reservation",
        JSON.stringify({
          ...reservation,
          eventTitle: event?.title,
          seats: event?.inventory.filter((seat) => seatIds.includes(seat.id)) ?? [],
          total: Number(reservation.totalAmount || total),
        })
      );
      try { sessionStorage.removeItem("narm-pending-reservation"); } catch { /* storage may be unavailable */ }
      navigate(`/checkout/${reservation.id}`);
    } catch (e) {
      setError((e as Error).message);
      setPhase("view");
      setQueuePosition(null);
    }
  }
  if (!event)
    return <div className="loader">{error || "در حال بارگذاری سالن…"}</div>;
  const rows = event.inventory.reduce<Record<string, typeof event.inventory>>(
    (g, s) => {
      g[s.row] = [...(g[s.row] ?? []), s];
      return g;
    },
    {}
  );
  return (
    <section className="booking-page">
      <div className="booking-head">
        <div>
          <Link to="/">رویدادها</Link>
          <span>/</span>
          <b>{event.title}</b>
        </div>
        <h1>{event.title}</h1>
        <p>
          <CalendarDays />
          {new Date(event.startsAt).toLocaleString("fa-IR")}
          <MapPin />
          {event.venue?.name ?? event.city}
        </p>
      </div>
      <div className="booking-layout">
        <div>
          {error && <Notice text={error} />}
          {!event.bookable && <Notice text="زمان این رویداد گذشته و دیگر قابل رزرو نیست." />}
          {phase === "waiting" && queuePosition !== null && (
            <Notice kind="success" text={`جایگاه شما در صف: ${queuePosition.toLocaleString("fa-IR")}. وضعیت خودکار بررسی می‌شود.`} />
          )}
          <div className="map-card">
            <div className="stage">
              <span>صحنه</span>
            </div>
            <div className="legend">
              <span>
                <i className="available" />
                آزاد
              </span>
              <span>
                <i className="selected" />
                انتخاب شما
              </span>
              <span>
                <i className="locked" />
                رزرو شده
              </span>
            </div>
            <div className="seat-map">
              {Object.entries(rows).map(([row, seats]) => (
                <div className="seat-row" key={row}>
                  <b>{row}</b>
                  <div>
                    {seats.map((seat) => (
                      <button
                        key={seat.id}
                        disabled={seat.state !== "AVAILABLE"}
                        onClick={() => toggle(seat.id)}
                        aria-pressed={selected.includes(seat.id)}
                        aria-label={`ردیف ${seat.row}، صندلی ${seat.number}، ${seat.state === "AVAILABLE" ? "آزاد" : "غیرقابل انتخاب"}${seat.accessible ? "، دسترس‌پذیر" : ""}`}
                        title={`${Number(seat.price).toLocaleString("fa-IR")} ${seat.currency}`}
                        className={
                          selected.includes(seat.id)
                            ? "chosen"
                            : seat.state.toLowerCase()
                        }
                      >
                        {seat.number}
                      </button>
                    ))}
                  </div>
                  <b>{row}</b>
                </div>
              ))}
            </div>
          </div>
        </div>
        <aside className="order-card">
          <h2>خلاصه انتخاب</h2>
          <div className="event-mini">
            <div>{event.title.slice(0, 1)}</div>
            <span>
              <b>{event.title}</b>
              <small>
                {new Date(event.startsAt).toLocaleDateString("fa-IR")}
              </small>
            </span>
          </div>
          <div className="selected-list">
            {chosen.length ? (
              chosen.map((s) => (
                <div key={s.id}>
                  <span>
                    ردیف {s.row}، صندلی {s.number}
                  </span>
                  <b>
                    {Number(s.price).toLocaleString("fa-IR")} {s.currency}
                  </b>
                </div>
              ))
            ) : (
              <p>صندلی‌های موردنظرت را از نقشه انتخاب کن.</p>
            )}
          </div>
          <div className="total">
            <span>مبلغ کل</span>
            <strong>
              {total.toLocaleString("fa-IR")} <small>ریال</small>
            </strong>
          </div>
          <button
            className="button wide"
            disabled={!event.bookable || !selected.length || phase !== "view"}
            onClick={() => void reserve()}
          >
            {phase === "waiting"
              ? "ورود به صف…"
              : phase === "reserving"
              ? "ثبت رزرو…"
              : "ادامه و پرداخت"}
            <ChevronLeft />
          </button>
          <small className="secure" aria-live="polite">
            <ShieldCheck />
            صندلی‌ها پس از رزرو ۱۰ دقیقه قفل می‌شوند.
          </small>
        </aside>
      </div>
    </section>
  );
}

export default EventPage;
