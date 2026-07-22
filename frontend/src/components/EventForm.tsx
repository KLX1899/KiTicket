import { useState, type FormEvent } from "react";
import { api, type EventSummary, type Venue } from "../api";
import Notice from "./Notice";
import PersianDatePicker from "./PersianDatePicker";

export default function EventForm({ token, venues, done }: { token: string; venues: Venue[]; done: (event: EventSummary) => void }) {
  const [venueId, setVenueId] = useState(""), [title, setTitle] = useState(""), [description, setDescription] = useState(""),
    [genre, setGenre] = useState("Music"), [city, setCity] = useState(""), [startsAt, setStartsAt] = useState<Date | null>(null),
    [price, setPrice] = useState(2_500_000), [tags, setTags] = useState(""), [error, setError] = useState(""),
    [loading, setLoading] = useState(false), [draft, setDraft] = useState<EventSummary | null>(null),
    [pricingReady, setPricingReady] = useState(false);
  async function submit(formEvent: FormEvent) {
    formEvent.preventDefault();
    if (!startsAt) { setError("تاریخ و ساعت شروع را از تقویم شمسی انتخاب کنید."); return; }
    setLoading(true); setError("");
    try {
      let event = draft;
      if (!event) {
        event = await api<EventSummary>("/events", {
          method: "POST",
          body: JSON.stringify({
            venueId, title, description, genre, city,
            startsAt: startsAt.toISOString(),
            tags: tags.split(",").map((tag) => tag.trim()).filter(Boolean),
          }),
        }, token);
        setDraft(event);
      }
      if (!pricingReady) {
        await api(`/events/${event.id}/pricing`, { method: "POST", body: JSON.stringify({ name: "استاندارد", price, currency: "IRR" }) }, token);
        setPricingReady(true);
      }
      await api(`/events/${event.id}/publish`, { method: "POST" }, token);
      done(event);
      setDraft(null); setPricingReady(false);
      setTitle(""); setDescription(""); setStartsAt(null); setTags("");
    } catch (caught) {
      setError((caught as Error).message);
    } finally {
      setLoading(false);
    }
  }
  return (
    <form className="manage-form wide-form" onSubmit={submit}>
      {error && <Notice text={error} />}
      <label>سالن<select value={venueId} onChange={(event) => { const next = event.target.value; setVenueId(next); setCity(venues.find((venue) => venue.id === next)?.city ?? ""); }} required><option value="">انتخاب کنید</option>{venues.map((venue) => <option key={venue.id} value={venue.id}>{venue.name}</option>)}</select></label>
      <label>عنوان<input value={title} onChange={(event) => setTitle(event.target.value)} minLength={2} required /></label>
      <label>دسته<select value={genre} onChange={(event) => setGenre(event.target.value)}><option value="Music">موسیقی</option><option value="Theatre">تئاتر</option><option value="Sport">ورزشی</option><option value="Conference">همایش</option></select></label>
      <label>شهر<input value={city} onChange={(event) => setCity(event.target.value)} required /></label>
      <label>شروع<PersianDatePicker value={startsAt} onChange={setStartsAt} placeholder="انتخاب تاریخ و ساعت" ariaLabel="انتخاب تاریخ و ساعت شمسی شروع رویداد" minDate={new Date()} withTime /></label>
      <label>قیمت پایه (ریال)<input type="number" min={0} value={price} onChange={(event) => setPrice(Number(event.target.value))} required /></label>
      <label>برچسب‌ها با ویرگول<input value={tags} onChange={(event) => setTags(event.target.value)} /></label>
      <label className="full-field">توضیحات<textarea value={description} onChange={(event) => setDescription(event.target.value)} maxLength={5000} required /></label>
      <button className="button" disabled={loading || !venues.length}>{loading ? "در حال انتشار…" : "ایجاد، قیمت‌گذاری و انتشار"}</button>
    </form>
  );
}
