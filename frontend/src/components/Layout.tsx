import { useAuth } from "../auth";
import { io } from "socket.io-client";
import { useEffect, useState } from "react";
import { Link, NavLink } from "react-router-dom";
import { CircleUserRound, LogOut } from "lucide-react";

function Layout({ children }: { children: React.ReactNode }) {
  const { session, signOut } = useAuth();
  const [live, setLive] = useState("");
  useEffect(() => {
    if (!session) return;
    const socket = io(import.meta.env.VITE_WS_URL ?? window.location.origin, {
      auth: { token: session.token },
    });
    let toastTimer: number | undefined;
    [
      "payment.started",
      "payment.succeeded",
      "payment.failed",
      "ticket.issued",
    ].forEach((name) =>
      socket.on(name, () => {
        setLive(name);
        window.clearTimeout(toastTimer);
        toastTimer = window.setTimeout(() => setLive(""), 3500);
      })
    );
    return () => {
      window.clearTimeout(toastTimer);
      socket.close();
    };
  }, [session]);
  return (
    <div className="shell">
      <header>
        <Link className="brand" to="/">
          <span>Ki</span>
          <strong>KiTicket</strong>
        </Link>
        <nav>
          <NavLink to="/" end>رویدادها</NavLink>
          {session && <NavLink to="/tickets">بلیت‌های من</NavLink>}
          {session && session.role !== "CUSTOMER" && (
            <NavLink to="/manage">مدیریت</NavLink>
          )}
        </nav>
        <div className="header-actions">
          {session ? (
            <>
              <span className="user-pill">
                <CircleUserRound size={24} />
                {session.name || session.email}
              </span>
              <button className="icon-button" title="خروج" aria-label="خروج از حساب" onClick={signOut}>
                <LogOut size={19} />
              </button>
            </>
          ) : (
            <Link className="button small" to="/login">
              ورود
            </Link>
          )}
        </div>
      </header>
      {live && (
        <div className="live-toast">
          <span />
          به‌روزرسانی زنده: {live}
        </div>
      )}
      <main>{children}</main>
      <footer>
        <strong>KiTicket</strong>
        <span>بهترین ایونت، ایونتای بهراده که حضوریه 💞</span>
      </footer>
    </div>
  );
}

export default Layout;
