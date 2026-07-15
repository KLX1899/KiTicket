# KiTicket

KiTicket یک نمونه microservice-based برای کشف رویداد، صف انتظار، رزرو صندلی، checkout و verify بلیت است.
فرانت سبک پروژه هم از خود `api-gateway` سرو می‌شود و روی همان `http://127.0.0.1:18080` در دسترس است.

## Quick start

پیش‌نیازها:

- `Docker` و `docker compose`
- `Go 1.26+` برای build/test محلی

راه‌اندازی:

```bash
cp .env.example .env
make up
```

آدرس‌ها:

- Frontend + API Gateway: `http://127.0.0.1:18080`
- PostgreSQL: `127.0.0.1:55432`
- Redis: `127.0.0.1:56379`
- RabbitMQ Management: `http://127.0.0.1:55673`

دستورهای مهم:

```bash
make up           # ساخت و بالا آوردن کل stack
make down         # خاموش کردن سرویس‌ها
make migrate      # اجرای migrationها
make seed         # seed دیتای دمو
make build        # build همه سرویس‌های Go
make test         # اجرای تست‌های unit
make integration  # اجرای تست‌های integration
```

سناریوی دمو:

1. به `http://127.0.0.1:18080` برو.
2. ثبت‌نام یا login کن.
3. رویداد `Tehran Night Jazz` را انتخاب کن.
4. در صورت نیاز وارد صف شو، بعد صندلی رزرو و checkout را کامل کن.
5. `QR payload` بلیت صادرشده را در بخش verify تست کن.
