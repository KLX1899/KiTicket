import { useState, type FormEvent } from "react";
import { api, type Venue } from "../api";
import Notice from "./Notice";

export default function SectorForm({ token, venues, done }: { token: string; venues: Venue[]; done: () => void }) {
  const [venueId, setVenueId] = useState(""), [name, setName] = useState("سالن اصلی"), [rows, setRows] = useState(8),
    [seatsPerRow, setSeatsPerRow] = useState(12), [accessibleFirstRow, setAccessible] = useState(false),
    [error, setError] = useState(""), [loading, setLoading] = useState(false);
  async function submit(event: FormEvent) {
    event.preventDefault();
    setLoading(true); setError("");
    try {
      await api(`/venues/${venueId}/sectors`, {
        method: "POST",
        body: JSON.stringify({ name, rows, seatsPerRow, accessibleFirstRow }),
      }, token);
      done();
    } catch (caught) {
      setError((caught as Error).message);
    } finally {
      setLoading(false);
    }
  }
  return (
    <form className="manage-form" onSubmit={submit}>
      {error && <Notice text={error} />}
      <label>سالن<select value={venueId} onChange={(event) => setVenueId(event.target.value)} required><option value="">انتخاب کنید</option>{venues.map((venue) => <option key={venue.id} value={venue.id}>{venue.name} — {venue.city}</option>)}</select></label>
      <label>نام بخش<input value={name} onChange={(event) => setName(event.target.value)} required /></label>
      <label>تعداد ردیف<input type="number" min={1} max={52} value={rows} onChange={(event) => setRows(Number(event.target.value))} required /></label>
      <label>صندلی در ردیف<input type="number" min={1} max={200} value={seatsPerRow} onChange={(event) => setSeatsPerRow(Number(event.target.value))} required /></label>
      <label className="check-field"><input type="checkbox" checked={accessibleFirstRow} onChange={(event) => setAccessible(event.target.checked)} />ردیف اول دسترس‌پذیر است</label>
      <button className="button" disabled={loading || !venues.length}>{loading ? "در حال ساخت…" : "ساخت بخش و صندلی‌ها"}</button>
    </form>
  );
}
