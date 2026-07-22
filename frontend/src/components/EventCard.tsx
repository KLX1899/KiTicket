import { Link } from "react-router-dom";
import { CalendarDays, ChevronLeft, MapPin } from "lucide-react";
import type { EventSummary } from "../api";

function EventCard({ event, index }: { event: EventSummary; index: number }) {
  const palette = ["violet", "amber", "blue"][index % 3];
  return (
    <Link to={`/events/${event.id}`} className="event-card">
      <div className={`poster ${palette}`}>
        <span>{event.genre}</span>
        <div className="poster-art">{event.title.slice(0, 1)}</div>
      </div>
      <div className="card-body">
        <div className="date">
          <CalendarDays size={17} />
          {new Date(event.startsAt).toLocaleDateString("fa-IR", {
            dateStyle: "long",
          })}
        </div>
        <h3>{event.title}</h3>
        <div className="location">
          <MapPin size={16} />
          {event.city}
        </div>
        <div className="card-link">
          مشاهده و انتخاب صندلی <ChevronLeft size={18} />
        </div>
      </div>
    </Link>
  );
}

export default EventCard;
