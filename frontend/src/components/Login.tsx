import { useState, type FormEvent } from "react";
import { Navigate, useLocation, useNavigate } from "react-router-dom";
import { api, type AuthResponse } from "../api";
import { useAuth } from "../auth";
import Notice from "./Notice";

function Login() {
  const [mode, setMode] = useState<"login" | "register">("login"),
    [name, setName] = useState(""),
    [email, setEmail] = useState("customer@narm.local"),
    [password, setPassword] = useState("Password123!"),
    [error, setError] = useState(""),
    [loading, setLoading] = useState(false);
  const { session, signIn } = useAuth(), navigate = useNavigate(), location = useLocation();
  const navigation = location.state as { returnTo?: string; resumeReservation?: boolean } | null;
  const returnTo = navigation?.returnTo?.startsWith("/") && !navigation.returnTo.startsWith("//") ? navigation.returnTo : "/";
  const resumeState = navigation?.resumeReservation ? { resumeReservation: true } : null;

  if (session) return <Navigate to={returnTo} replace state={resumeState} />;

  async function submit(event: FormEvent) {
    event.preventDefault();
    setLoading(true);
    setError("");
    try {
      if (mode === "register") {
        await api("/auth/register", {
          method: "POST",
          body: JSON.stringify({ name, email, password }),
        });
      }
      const result = await api<AuthResponse>("/auth/login", {
        method: "POST",
        body: JSON.stringify({ email, password }),
      });
      signIn({
        token: result.accessToken,
        id: result.user.id,
        role: result.user.role,
        email: result.user.email,
        name: result.user.name,
      });
      navigate(returnTo, { replace: true, state: resumeState });
    } catch (caught) {
      setError((caught as Error).message);
    } finally {
      setLoading(false);
    }
  }

  return (
    <section className="auth-page">
      <form className="auth-card" onSubmit={submit}>
        <div className="auth-mark">ن</div>
        <span className="eyebrow">{mode === "login" ? "خوش آمدی" : "شروع یک تجربه تازه"}</span>
        <h1>{mode === "login" ? "ورود به حساب" : "ساخت حساب خریدار"}</h1>
        <p>برای رزرو صندلی و دریافت بلیت وارد شو.</p>
        {error && <Notice text={error} />}
        {mode === "register" && (
          <label>
            نام و نام خانوادگی
            <input value={name} onChange={(event) => setName(event.target.value)} minLength={2} maxLength={80} required />
          </label>
        )}
        <label>
          ایمیل
          <input type="email" autoComplete="email" value={email} onChange={(event) => setEmail(event.target.value)} required />
        </label>
        <label>
          رمز عبور
          <input
            type="password"
            autoComplete={mode === "login" ? "current-password" : "new-password"}
            value={password}
            onChange={(event) => setPassword(event.target.value)}
            minLength={mode === "register" ? 10 : 1}
            required
          />
        </label>
        <button className="button wide" disabled={loading}>
          {loading ? "در حال بررسی…" : mode === "login" ? "ورود امن" : "ثبت‌نام و ورود"}
        </button>
        <button
          type="button"
          className="text-button"
          onClick={() => { setMode((value) => value === "login" ? "register" : "login"); setError(""); }}
        >
          {mode === "login" ? "حساب ندارم؛ ثبت‌نام" : "حساب دارم؛ ورود"}
        </button>
        <small>حساب‌های نمایشی با اجرای seed ساخته می‌شوند.</small>
      </form>
    </section>
  );
}

export default Login;
