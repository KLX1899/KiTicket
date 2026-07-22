import { useEffect, useState } from "react";
import {
  Search,
  ShieldCheck,
  Clock3,
  TicketCheck,
  Sparkles,
} from "lucide-react";
import { api, ApiError, type EventSummary, type Paginated } from "../api";
import EventCard from "./EventCard";
import Notice from "./Notice";
import PersianDatePicker from "./PersianDatePicker";

function Home() {
  const [events, setEvents] = useState<EventSummary[]>([]),
    [query, setQuery] = useState(""),
    [genre, setGenre] = useState(""),
    [city, setCity] = useState(""),
    [from, setFrom] = useState<Date | null>(null),
    [available, setAvailable] = useState(false),
    [page, setPage] = useState(1),
    [pages, setPages] = useState(1),
    [total, setTotal] = useState(0),
    [loading, setLoading] = useState(true),
    [error, setError] = useState("");
  useEffect(() => {
    let active = true;
    const controller = new AbortController();
    setLoading(true);
    const timer = setTimeout(() => {
      const p = new URLSearchParams();
      if (query) p.set("q", query);
      if (genre) p.set("genre", genre);
      if (city) p.set("city", city);
      if (from) p.set("from", from.toISOString());
      if (available) p.set("available", "true");
      p.set("page", String(page));
      p.set("limit", "12");
      setError("");
      api<Paginated<EventSummary>>(`/events?${p}`, { signal: controller.signal })
        .then((result) => {
          if (!active) return;
          setEvents(result.items);
          setPages(Math.max(1, result.pages));
          setTotal(result.total);
        })
        .catch((e: ApiError) => { if (active) setError(e.message); })
        .finally(() => { if (active) setLoading(false); });
    }, 250);
    return () => {
      active = false;
      clearTimeout(timer);
      controller.abort();
    };
  }, [query, genre, city, from, available, page]);

  function resetPage() {
    setPage(1);
  }
  return (
    <>
      <section className="hero">
        <div className="hero-orbit" />
        <div className="hero-stamp" aria-hidden="true">
          <span>انتخابِ امروز</span>
          <b>۰۱</b>
          <small>رویدادهای منتخب</small>
        </div>
        <div className="eyebrow">
          <Sparkles size={16} />
          همین لحظه، یک تجربه تازه
        </div>
        <h1>
          جایی برای <em>خاطره‌های</em>
          <br />
          فراموش‌نشدنی
        </h1>
        <p>
          از کنسرت و تئاتر تا رویدادهای ورزشی؛ صندلی‌ات را امن و فوری انتخاب کن.
        </p>
        <div className="search-box">
          <Search />
          <input
            value={query}
            onChange={(e) => { setQuery(e.target.value); resetPage(); }}
            placeholder="جست‌وجوی نام رویداد..."
          />
          <select
            id="genre-select"
            value={genre}
            onChange={(e) => { setGenre(e.target.value); resetPage(); }}
          >
            <option value="">همه دسته‌ها</option>
            <option value="Music">موسیقی</option>
            <option value="Theatre">تئاتر</option>
            <option value="Sport">ورزشی</option>
            <option value="Conference">همایش</option>
          </select>
          <input
            id="city-filter"
            className="city-filter"
            value={city}
            onChange={(e) => { setCity(e.target.value); resetPage(); }}
            placeholder="شهر"
          />
          <PersianDatePicker
            className="date-filter"
            value={from}
            onChange={(value) => {
              if (!value) {
                setFrom(null);
                resetPage();
                return;
              }

              const startOfDay = new Date(value);
              startOfDay.setHours(0, 0, 0, 0);
              setFrom(startOfDay);
              resetPage();
            }}
            placeholder="از تاریخ"
            ariaLabel="انتخاب تاریخ شمسی شروع"
            minDate={new Date()}
            clearable
          />
          <label className="availability-filter">
            <input
              type="checkbox"
              checked={available}
              onChange={(e) => { setAvailable(e.target.checked); resetPage(); }}
            />
            فقط دارای ظرفیت
          </label>
        </div>
        <div className="trust-row">
          <span>
            <ShieldCheck />
            رزرو امن
          </span>
          <span>
            <Clock3 />
            قفل ۱۰ دقیقه‌ای
          </span>
          <span>
            <TicketCheck />
            بلیت قابل اعتبارسنجی
          </span>
        </div>
      </section>
      <section className="section">
        <div className="section-title">
          <div>
            <span>انتخاب‌های این هفته</span>
            <h2>رویدادهای پیش رو</h2>
          </div>
          <b>{total.toLocaleString("fa-IR")} رویداد</b>
        </div>
        {error && <Notice text={error} />}{" "}
        {loading ? (
          <div className="loader">در حال دریافت رویدادها…</div>
        ) : (
          <div className="event-grid">
            {events.map((event, index) => (
              <EventCard key={event.id} event={event} index={index} />
            ))}
          </div>
        )}
        {!loading && !events.length && (
          <div className="empty">رویدادی با این مشخصات پیدا نشد.</div>
        )}
        {!loading && pages > 1 && (
          <nav className="pagination" aria-label="صفحه‌بندی رویدادها">
            <button disabled={page <= 1} onClick={() => setPage((value) => value - 1)}>قبلی</button>
            <span>صفحه {page.toLocaleString("fa-IR")} از {pages.toLocaleString("fa-IR")}</span>
            <button disabled={page >= pages} onClick={() => setPage((value) => value + 1)}>بعدی</button>
          </nav>
        )}
      </section>
    </>
  );
}

export default Home;
