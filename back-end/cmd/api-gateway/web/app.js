const state = {
  token: localStorage.getItem("kiticket.token") || "",
  user: JSON.parse(localStorage.getItem("kiticket.user") || "null"),
  selectedEvent: null,
  detail: null,
  selectedSeats: new Set(),
  queueToken: "",
  admissionToken: "",
  reservation: null,
  tickets: [],
  queueTimer: null,
};

const el = {
  events: document.querySelector("#events"),
  seatMap: document.querySelector("#seatMap"),
  sessionState: document.querySelector("#sessionState"),
  selectionMeta: document.querySelector("#selectionMeta"),
  queueState: document.querySelector("#queueState"),
  reservationState: document.querySelector("#reservationState"),
  ticketOutput: document.querySelector("#ticketOutput"),
  verifyState: document.querySelector("#verifyState"),
  statusLog: document.querySelector("#statusLog"),
  reserveBtn: document.querySelector("#reserveBtn"),
  checkoutBtn: document.querySelector("#checkoutBtn"),
};

document.querySelector("#registerForm").addEventListener("submit", (event) => submitAuth(event, "register"));
document.querySelector("#loginForm").addEventListener("submit", (event) => submitAuth(event, "login"));
document.querySelector("#logoutBtn").addEventListener("click", logout);
document.querySelector("#searchForm").addEventListener("submit", searchEvents);
document.querySelector("#joinQueueBtn").addEventListener("click", joinQueue);
document.querySelector("#reserveBtn").addEventListener("click", reserveSeats);
document.querySelector("#checkoutBtn").addEventListener("click", checkout);
document.querySelector("#verifyForm").addEventListener("submit", verifyTicket);

refreshSession();
searchEvents(new Event("submit"));

async function api(path, options = {}) {
  const headers = new Headers(options.headers || {});
  headers.set("Content-Type", "application/json");
  headers.set("X-Request-ID", crypto.randomUUID().replace(/-/g, "").slice(0, 24));
  if (state.token && !headers.has("Authorization")) {
    headers.set("Authorization", `Bearer ${state.token}`);
  }
  const response = await fetch(path, { ...options, headers });
  const text = await response.text();
  const data = text ? JSON.parse(text) : null;
  if (!response.ok) {
    const message = data?.error?.message || `HTTP ${response.status}`;
    const error = new Error(message);
    error.code = data?.error?.code || "request_failed";
    throw error;
  }
  return data;
}

function log(message, payload) {
  const lines = [new Date().toLocaleTimeString("fa-IR"), message];
  if (payload) {
    lines.push(JSON.stringify(payload, null, 2));
  }
  el.statusLog.textContent = `${lines.join("\n")}\n\n${el.statusLog.textContent}`.trim();
}

function persistSession() {
  localStorage.setItem("kiticket.token", state.token);
  localStorage.setItem("kiticket.user", JSON.stringify(state.user));
}

function refreshSession() {
  el.sessionState.textContent = state.user
    ? `${state.user.display_name || state.user.email || state.user.id} وارد شده است.`
    : "وارد نشده‌اید.";
}

async function submitAuth(event, mode) {
  event.preventDefault();
  const form = new FormData(event.currentTarget);
  const body = Object.fromEntries(form.entries());
  try {
    const session = await api(`/v1/auth/${mode}`, { method: "POST", body: JSON.stringify(body) });
    state.token = session.access_token;
    state.user = session.user;
    persistSession();
    refreshSession();
    log(mode === "register" ? "ثبت‌نام انجام شد." : "ورود انجام شد.", session.user);
  } catch (error) {
    log("خطای احراز هویت", { message: error.message, code: error.code });
  }
}

function logout() {
  state.token = "";
  state.user = null;
  state.admissionToken = "";
  state.queueToken = "";
  state.reservation = null;
  state.tickets = [];
  persistSession();
  refreshSession();
  renderReservation();
  renderTickets();
  renderQueue();
}

async function searchEvents(event) {
  event.preventDefault();
  const query = new FormData(document.querySelector("#searchForm")).get("q") || "";
  try {
    const data = await api(`/v1/events?available_only=true&q=${encodeURIComponent(query)}`, { method: "GET" });
    renderEvents(data.items || []);
    log("رویدادها بارگذاری شدند.", { count: data.items?.length || 0 });
  } catch (error) {
    el.events.innerHTML = `<div class="notice">${escapeHTML(error.message)}</div>`;
    log("خطا در دریافت رویدادها", { message: error.message, code: error.code });
  }
}

function renderEvents(items) {
  if (!items.length) {
    el.events.innerHTML = `<div class="notice">رویدادی پیدا نشد.</div>`;
    return;
  }
  el.events.innerHTML = items.map((item) => `
    <article class="event">
      <div class="row-between">
        <div>
          <h3>${escapeHTML(item.title)}</h3>
          <div class="event-meta">
            <span>${escapeHTML(item.genre)}</span>
            <span>${escapeHTML(item.city)} / ${escapeHTML(item.venue_name)}</span>
            <span>${formatMoney(item.minimum_price_minor, item.currency)}</span>
            <span>${item.available_seats} صندلی آزاد</span>
          </div>
        </div>
        <button type="button" data-event="${item.event_id}" data-schedule="${item.schedule_id}">انتخاب</button>
      </div>
    </article>
  `).join("");
  el.events.querySelectorAll("button").forEach((button) => {
    button.addEventListener("click", () => loadSchedule(button.dataset.event, button.dataset.schedule));
  });
}

async function loadSchedule(eventID, scheduleID) {
  try {
    const detail = await api(`/v1/events/${eventID}/schedules/${scheduleID}`, { method: "GET" });
    state.selectedEvent = { eventID, scheduleID };
    state.detail = detail;
    state.selectedSeats.clear();
    state.queueToken = "";
    state.admissionToken = "";
    state.reservation = null;
    renderQueue();
    renderReservation();
    renderSeats();
    log("جزئیات schedule بارگذاری شد.", { event_id: eventID, schedule_id: scheduleID });
  } catch (error) {
    log("خطا در دریافت جزئیات schedule", { message: error.message, code: error.code });
  }
}

function renderSeats() {
  if (!state.detail) {
    el.selectionMeta.textContent = "هنوز رویدادی انتخاب نشده.";
    el.seatMap.className = "seat-map empty";
    el.seatMap.textContent = "برای شروع یک رویداد را انتخاب کن.";
    return;
  }
  el.selectionMeta.textContent = `${state.detail.title} - ${new Date(state.detail.starts_at).toLocaleString("fa-IR")} - ${state.detail.hall_name}`;
  const groups = new Map();
  for (const seat of state.detail.seats) {
    const key = `${seat.section_name} / ردیف ${seat.row_label}`;
    if (!groups.has(key)) groups.set(key, []);
    groups.get(key).push(seat);
  }
  el.seatMap.className = "seat-map";
  el.seatMap.innerHTML = [...groups.entries()].map(([label, seats]) => `
    <section class="seat-row">
      <header>${escapeHTML(label)}</header>
      <div class="seats">
        ${seats.map((seat) => {
          const selected = state.selectedSeats.has(seat.seat_id);
          const disabled = !seat.available ? "disabled" : "";
          const className = ["seat", selected ? "selected" : "", !seat.available ? "sold" : ""].join(" ").trim();
          return `
            <button type="button" class="${className}" data-seat="${seat.seat_id}" ${disabled}>
              ${escapeHTML(seat.seat_number)}
              <small>${formatMoney(seat.price_minor, seat.currency)}</small>
            </button>
          `;
        }).join("")}
      </div>
    </section>
  `).join("");
  el.seatMap.querySelectorAll("[data-seat]").forEach((button) => {
    button.addEventListener("click", () => toggleSeat(button.dataset.seat));
  });
}

function toggleSeat(seatID) {
  if (state.selectedSeats.has(seatID)) {
    state.selectedSeats.delete(seatID);
  } else {
    state.selectedSeats.add(seatID);
  }
  renderSeats();
}

async function joinQueue() {
  if (!state.selectedEvent) return log("اول یک رویداد انتخاب کن.");
  if (!state.token) return log("برای ورود به صف باید لاگین کنی.");
  try {
    const joined = await api(`/v1/waiting-room/${state.selectedEvent.eventID}/join`, { method: "POST" });
    state.queueToken = joined.queue_token;
    renderQueue(joined);
    watchQueue();
    log("به صف وارد شدی.", joined);
  } catch (error) {
    log("خطا در ورود به صف", { message: error.message, code: error.code });
  }
}

function watchQueue() {
  if (state.queueTimer) clearInterval(state.queueTimer);
  state.queueTimer = setInterval(async () => {
    if (!state.queueToken) return;
    try {
      const status = await api("/v1/waiting-room/status", {
        method: "POST",
        body: JSON.stringify({ queue_token: state.queueToken }),
      });
      if (status.admission_token) {
        state.admissionToken = status.admission_token;
        clearInterval(state.queueTimer);
      }
      renderQueue(status);
    } catch (error) {
      clearInterval(state.queueTimer);
      log("خطا در بررسی وضعیت صف", { message: error.message, code: error.code });
    }
  }, 1500);
}

function renderQueue(status) {
  if (state.admissionToken) {
    el.queueState.textContent = "Admission token دریافت شد. الان می‌توانی رزرو کنی.";
    return;
  }
  if (status?.position) {
    el.queueState.textContent = `وضعیت صف: ${status.state || "queued"} - موقعیت ${status.position}`;
    return;
  }
  el.queueState.textContent = state.queueToken ? "در حال بررسی وضعیت صف..." : "توکن صف ندارید.";
}

async function reserveSeats() {
  if (!state.detail) return log("اول schedule را انتخاب کن.");
  if (!state.token) return log("برای رزرو باید لاگین کنی.");
  const seatIDs = [...state.selectedSeats];
  if (!seatIDs.length) return log("حداقل یک صندلی انتخاب کن.");
  const headers = { "Idempotency-Key": `reserve-${crypto.randomUUID()}` };
  if (state.admissionToken) headers["X-Admission-Token"] = state.admissionToken;
  try {
    state.reservation = await api("/v1/reservation-locks", {
      method: "POST",
      headers,
      body: JSON.stringify({
        event_id: state.detail.event_id,
        schedule_id: state.detail.schedule_id,
        seat_ids: seatIDs,
        ttl_seconds: 300,
      }),
    });
    renderReservation();
    log("رزرو ثبت شد.", state.reservation);
  } catch (error) {
    log("خطا در رزرو", { message: error.message, code: error.code });
  }
}

function renderReservation() {
  if (!state.reservation) {
    el.reservationState.textContent = "هنوز رزروی ثبت نشده.";
    return;
  }
  el.reservationState.textContent = `رزرو ${state.reservation.reservation_id} برای ${state.reservation.seat_ids.join(", ")} تا ${new Date(state.reservation.expires_at).toLocaleTimeString("fa-IR")} فعال است.`;
}

async function checkout() {
  if (!state.reservation) return log("اول رزرو انجام بده.");
  try {
    const result = await api("/v1/checkouts", {
      method: "POST",
      headers: { "Idempotency-Key": `checkout-${crypto.randomUUID()}` },
      body: JSON.stringify({
        reservation_id: state.reservation.reservation_id,
        reservation_fence: state.reservation.fence,
        schedule_id: state.reservation.schedule_id,
        seat_ids: state.reservation.seat_ids,
      }),
    });
    state.tickets = result.tickets || [];
    renderTickets();
    log("خرید تکمیل شد.", result.order);
  } catch (error) {
    log("خطا در checkout", { message: error.message, code: error.code });
  }
}

function renderTickets() {
  if (!state.tickets.length) {
    el.ticketOutput.textContent = "هنوز بلیتی صادر نشده.";
    return;
  }
  el.ticketOutput.textContent = JSON.stringify(state.tickets, null, 2);
}

async function verifyTicket(event) {
  event.preventDefault();
  const qrPayload = new FormData(event.currentTarget).get("qr_payload");
  try {
    const result = await api("/v1/tickets/verify", {
      method: "POST",
      body: JSON.stringify({ qr_payload: qrPayload }),
    });
    el.verifyState.textContent = result.valid
      ? `بلیت معتبر است. ticket_id=${result.ticket_id}`
      : "بلیت نامعتبر است.";
    log("نتیجه بررسی بلیت", result);
  } catch (error) {
    el.verifyState.textContent = error.message;
    log("خطا در verify", { message: error.message, code: error.code });
  }
}

function formatMoney(amount, currency) {
  return `${Number(amount).toLocaleString("en-US")} ${currency}`;
}

function escapeHTML(value) {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;");
}
