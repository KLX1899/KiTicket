import { CalendarDays, X } from "lucide-react";
import DatePicker from "react-multi-date-picker";
import persian from "react-date-object/calendars/persian";
import persianFa from "react-date-object/locales/persian_fa";
import TimePicker from "react-multi-date-picker/plugins/time_picker";

interface PersianDatePickerProps {
  value: Date | null;
  onChange: (value: Date | null) => void;
  placeholder: string;
  ariaLabel: string;
  withTime?: boolean;
  clearable?: boolean;
  className?: string;
  minDate?: Date;
}

export default function PersianDatePicker({
  value,
  onChange,
  placeholder,
  ariaLabel,
  withTime = false,
  clearable = false,
  className = "",
  minDate,
}: PersianDatePickerProps) {
  return (
    <div className={`persian-date-picker ${className}`.trim()}>
      <DatePicker
        value={value}
        onChange={(selected) => onChange(selected?.toDate() ?? null)}
        calendar={persian}
        locale={persianFa}
        format={withTime ? "YYYY/MM/DD HH:mm" : "YYYY/MM/DD"}
        minDate={minDate}
        calendarPosition="bottom-right"
        className="narm-persian-calendar"
        portal
        zIndex={1000}
        editable={false}
        mobileLabels={{ OK: "تأیید", CANCEL: "انصراف" }}
        plugins={withTime ? [<TimePicker key="time" position="bottom" hideSeconds />] : []}
        render={(formattedValue, openCalendar) => (
          <button
            type="button"
            className="persian-date-trigger"
            onClick={openCalendar}
            aria-label={ariaLabel}
            aria-haspopup="dialog"
          >
            <span className={formattedValue ? "" : "placeholder"}>{formattedValue || placeholder}</span>
            <CalendarDays aria-hidden="true" />
          </button>
        )}
      />
      {clearable && value && (
        <button type="button" className="persian-date-clear" onClick={() => onChange(null)} aria-label="پاک‌کردن تاریخ">
          <X aria-hidden="true" />
        </button>
      )}
    </div>
  );
}
