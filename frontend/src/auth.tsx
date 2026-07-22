import {
  createContext,
  useEffect,
  useContext,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import { AUTH_EXPIRED_EVENT, type Role } from "./api";
interface Session {
  token: string;
  id: string;
  role: Role;
  email: string;
  name: string;
}
interface AuthContextValue {
  session: Session | null;
  signIn: (session: Session) => void;
  signOut: () => void;
}
const AuthContext = createContext<AuthContextValue | null>(null),
  storageKey = "narm-session";

function isExpired(token: string): boolean {
  try {
    const encoded = token.split(".")[1].replace(/-/g, "+").replace(/_/g, "/");
    const payload = JSON.parse(atob(encoded.padEnd(Math.ceil(encoded.length / 4) * 4, "="))) as { exp?: number };
    return typeof payload.exp === "number" && payload.exp * 1000 <= Date.now();
  } catch {
    return false;
  }
}

function readSession(): Session | null {
  try {
    const raw = localStorage.getItem(storageKey);
    if (!raw) return null;
    const parsed = JSON.parse(raw) as Partial<Session>;
    if (!parsed.token || !parsed.id || !parsed.email || !parsed.role || isExpired(parsed.token)) {
      try { localStorage.removeItem(storageKey); } catch { /* storage may be unavailable */ }
      return null;
    }
    return { ...parsed, name: parsed.name ?? "" } as Session;
  } catch {
    try { localStorage.removeItem(storageKey); } catch { /* storage may be unavailable */ }
    return null;
  }
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [session, setSession] = useState<Session | null>(readSession);
  useEffect(() => {
    const expire = () => {
      try { localStorage.removeItem(storageKey); } catch { /* storage may be unavailable */ }
      setSession(null);
    };
    window.addEventListener(AUTH_EXPIRED_EVENT, expire);
    return () => window.removeEventListener(AUTH_EXPIRED_EVENT, expire);
  }, []);
  const value = useMemo(
    () => ({
      session,
      signIn(next: Session) {
        try { localStorage.setItem(storageKey, JSON.stringify(next)); } catch { /* keep an in-memory session */ }
        setSession(next);
      },
      signOut() {
        try { localStorage.removeItem(storageKey); } catch { /* storage may be unavailable */ }
        setSession(null);
      },
    }),
    [session]
  );
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}
export function useAuth() {
  const context = useContext(AuthContext);
  if (!context) throw new Error("AuthProvider is missing");
  return context;
}
