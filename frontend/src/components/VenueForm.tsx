import { useState, type FormEvent } from "react";
import { api, type Venue } from "../api";
import Notice from "./Notice";

function VenueForm({ token, done }: { token: string; done: (venue: Venue) => void }) {
  const [name, setName] = useState(""), [city, setCity] = useState(""), [address, setAddress] = useState(""),
    [error, setError] = useState(""), [loading, setLoading] = useState(false);
  async function submit(event: FormEvent) {
    event.preventDefault();
    setLoading(true);
    setError("");
    try {
      const venue = await api<Venue>("/venues", { method: "POST", body: JSON.stringify({ name, city, address }) }, token);
      done(venue);
      setName(""); setCity(""); setAddress("");
    } catch (caught) {
      setError((caught as Error).message);
    } finally {
      setLoading(false);
    }
  }
  return (
    <form className="manage-form" onSubmit={submit}>
      {error && <Notice text={error} />}
      <label>نام مجموعه<input value={name} onChange={(event) => setName(event.target.value)} required minLength={2} /></label>
      <label>شهر<input value={city} onChange={(event) => setCity(event.target.value)} required minLength={2} /></label>
      <label>نشانی<input value={address} onChange={(event) => setAddress(event.target.value)} required /></label>
      <button className="button" disabled={loading}>{loading ? "در حال ثبت…" : "ثبت سالن"}</button>
    </form>
  );
}

export default VenueForm;
