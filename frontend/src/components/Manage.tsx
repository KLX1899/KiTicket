import { useEffect, useState } from "react";
import { Navigate } from "react-router-dom";
import { CalendarDays, TicketCheck, Users } from "lucide-react";
import {
  api,
  type EventAnalytics,
  type EventSummary,
  type Role,
  type UserSummary,
  type Venue,
} from "../api";
import { useAuth } from "../auth";
import EventForm from "./EventForm";
import Notice from "./Notice";
import SectorForm from "./SectorForm";
import VenueForm from "./VenueForm";

function Manage() {
  const { session } = useAuth();
  const [venues, setVenues] = useState<Venue[]>([]), [events, setEvents] = useState<EventSummary[]>([]),
    [selectedEvent, setSelectedEvent] = useState(""), [analytics, setAnalytics] = useState<EventAnalytics | null>(null),
    [users, setUsers] = useState<UserSummary[]>([]), [message, setMessage] = useState(""), [error, setError] = useState("");

  useEffect(() => {
    if (!session || session.role === "CUSTOMER") return;
    let active = true;
    const controller = new AbortController();
    const load = async () => {
      try {
        const [venueResult, eventResult] = await Promise.all([
          api<Venue[]>("/venues", { signal: controller.signal }),
          api<EventSummary[]>("/events/mine", { signal: controller.signal }, session.token),
        ]);
        if (!active) return;
        setVenues(venueResult);
        setEvents(eventResult);
        setSelectedEvent(eventResult[0]?.id ?? "");
        if (session.role === "ADMIN") {
          const result = await api<UserSummary[]>("/users", { signal: controller.signal }, session.token);
          if (active) setUsers(result);
        }
      } catch (caught) {
        if (active) setError((caught as Error).message);
      }
    };
    void load();
    return () => { active = false; controller.abort(); };
  }, [session]);

  useEffect(() => {
    if (!session || !selectedEvent) { setAnalytics(null); return; }
    let active = true;
    const controller = new AbortController();
    api<EventAnalytics>(`/events/${selectedEvent}/analytics`, { signal: controller.signal }, session.token)
      .then((result) => { if (active) setAnalytics(result); })
      .catch((caught) => { if (active) setError((caught as Error).message); });
    return () => { active = false; controller.abort(); };
  }, [selectedEvent, session]);

  if (!session) return <Navigate to="/login" />;
  if (session.role === "CUSTOMER") return <Navigate to="/" />;
  const token = session.token;

  function success(text: string) { setError(""); setMessage(text); }

  async function changeRole(userId: string, role: Role) {
    setError("");
    try {
      const updated = await api<UserSummary>(`/users/${userId}/role`, { method: "PATCH", body: JSON.stringify({ role }) }, token);
      setUsers((current) => current.map((user) => user.id === userId ? updated : user));
      success("نقش کاربر به‌روزرسانی شد.");
    } catch (caught) {
      setError((caught as Error).message);
    }
  }

  return (
    <section className="section">
      <div className="section-title"><div><span>پنل برگزارکننده</span><h2>مدیریت و تحلیل رویداد</h2></div></div>
      {message && <Notice text={message} kind="success" />}
      {error && <Notice text={error} />}
      <label className="analytics-picker">رویداد آماری<select value={selectedEvent} onChange={(event) => setSelectedEvent(event.target.value)}><option value="">انتخاب کنید</option>{events.map((event) => <option key={event.id} value={event.id}>{event.title}</option>)}</select></label>
      <div className="stats">
        <div><Users /><span><b>{(analytics?.capacity ?? 0).toLocaleString("fa-IR")}</b>ظرفیت کل</span></div>
        <div><CalendarDays /><span><b>{(analytics?.remainingSeats ?? 0).toLocaleString("fa-IR")}</b>صندلی باقی‌مانده</span></div>
        <div><TicketCheck /><span><b>{(analytics?.revenue ?? 0).toLocaleString("fa-IR")}</b>درآمد {analytics?.currency ?? "IRR"}</span></div>
      </div>
      <div className="manage-grid">
        <div className="manage-card"><h3>۱. ایجاد سالن</h3><p>اطلاعات فیزیکی مجموعه را ثبت کنید.</p><VenueForm token={session.token} done={(venue) => { setVenues((current) => [...current, venue]); success("سالن ایجاد شد؛ اکنون بخش و صندلی تعریف کنید."); }} /></div>
        <div className="manage-card"><h3>۲. ساخت بخش و صندلی</h3><p>ردیف‌ها اتمیک ساخته می‌شوند و ظرفیت از چیدمان محاسبه می‌شود.</p><SectorForm token={session.token} venues={venues} done={() => success("بخش و صندلی‌ها ساخته شدند.")} /></div>
        <div className="manage-card full-card"><h3>۳. ایجاد و انتشار رویداد</h3><p>رویداد پس از قیمت‌گذاری منتشر می‌شود.</p><EventForm token={session.token} venues={venues} done={(created) => { setEvents((current) => [...current, created]); setSelectedEvent(created.id); success("رویداد قیمت‌گذاری و منتشر شد."); }} /></div>
      </div>
      {session.role === "ADMIN" && (
        <div className="manage-card">
          <h3>مدیریت نقش کاربران</h3>
          <div className="user-table">
            {users.map((user) => (
              <div key={user.id}><span><b>{user.name || user.email}</b><small>{user.email}</small></span><select value={user.role} onChange={(event) => void changeRole(user.id, event.target.value as Role)}><option value="CUSTOMER">خریدار</option><option value="ORGANIZER">برگزارکننده</option><option value="ADMIN">مدیر</option></select></div>
            ))}
          </div>
        </div>
      )}
    </section>
  );
}

export default Manage;
