import { useLayoutEffect, useRef, useState } from "react";
import type { KeyboardEvent } from "react";
import {
  ArrowRight,
  CalendarDays,
  ChevronLeft,
  ChevronRight,
  MapPin,
} from "lucide-react";
import { useLMS } from "../../context/useLMS";
import { demoWeek } from "../../data/lmsData";
import { formatDate, publishedMonday, localISO } from "../../utils/academic";
import { EmptyState, Badge } from "../ui/Workspace";

const weekdays = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"];
const sameMonth = (a: Date, b: Date) =>
  a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth();

export function ScheduleSidebar() {
  const { currentSemester, subjectByCode, resetWeek } = useLMS();
  const [today] = useState(() => new Date());
  const [expanded, setExpanded] = useState(false);
  const [selectedDate, setSelectedDate] = useState(publishedMonday);
  const [viewMonth, setViewMonth] = useState(
    () =>
      new Date(
        publishedMonday().getFullYear(),
        publishedMonday().getMonth(),
        1,
      ),
  );
  const calendarRef = useRef<HTMLTableElement>(null);
  const keyboardFocus = useRef<string | null>(null);
  useLayoutEffect(() => {
    if (!keyboardFocus.current) return;
    calendarRef.current
      ?.querySelector<HTMLButtonElement>(
        `[data-date="${keyboardFocus.current}"]`,
      )
      ?.focus();
    keyboardFocus.current = null;
  }, [selectedDate, viewMonth]);
  const firstDay = new Date(viewMonth.getFullYear(), viewMonth.getMonth(), 1);
  const offset = (firstDay.getDay() + 6) % 7;
  const daysInMonth = new Date(
    viewMonth.getFullYear(),
    viewMonth.getMonth() + 1,
    0,
  ).getDate();
  const cells = Array.from(
    { length: Math.ceil((offset + daysInMonth) / 7) * 7 },
    (_, i) =>
      new Date(viewMonth.getFullYear(), viewMonth.getMonth(), i - offset + 1),
  );
  const selectedISO = localISO(selectedDate);
  const entries =
    currentSemester === 3 && demoWeek.published
      ? demoWeek.days[selectedISO]
      : undefined;
  const monthLabel = formatDate(viewMonth, { month: "long", year: "numeric" });
  const tabStop = sameMonth(selectedDate, viewMonth)
    ? selectedISO
    : localISO(firstDay);
  const selectDate = (date: Date) => {
    setSelectedDate(date);
    if (!sameMonth(date, viewMonth))
      setViewMonth(new Date(date.getFullYear(), date.getMonth(), 1));
  };
  const handleDateKeyDown = (
    event: KeyboardEvent<HTMLButtonElement>,
    date: Date,
  ) => {
    const next = new Date(date);
    const dayMoves: Record<string, number> = {
      ArrowLeft: -1,
      ArrowRight: 1,
      ArrowUp: -7,
      ArrowDown: 7,
      Home: -((date.getDay() + 6) % 7),
      End: 6 - ((date.getDay() + 6) % 7),
    };
    if (event.key in dayMoves)
      next.setDate(next.getDate() + dayMoves[event.key]!);
    else if (event.key === "PageUp" || event.key === "PageDown") {
      const month = date.getMonth() + (event.key === "PageUp" ? -1 : 1);
      const lastDay = new Date(date.getFullYear(), month + 1, 0).getDate();
      next.setFullYear(
        date.getFullYear(),
        month,
        Math.min(date.getDate(), lastDay),
      );
    } else return;
    event.preventDefault();
    keyboardFocus.current = localISO(next);
    selectDate(next);
  };
  const returnToPublished = () => {
    const published = publishedMonday();
    setSelectedDate(published);
    setViewMonth(new Date(published.getFullYear(), published.getMonth(), 1));
  };
  return (
    <aside className="schedule-rail" aria-label="Calendar and day schedule">
      <button
        className="calendar-toggle"
        aria-expanded={expanded}
        aria-controls="calendar-and-agenda"
        onClick={() => setExpanded((value) => !value)}
      >
        <CalendarDays size={18} aria-hidden="true" />
        <span>Calendar & day schedule</span>
        <ChevronRight
          className={expanded ? "calendar-toggle-open" : ""}
          size={16}
          aria-hidden="true"
        />
      </button>
      <div
        id="calendar-and-agenda"
        className="calendar-agenda-content"
        data-expanded={expanded}
      >
        <section
          className="panel calendar-panel"
          aria-labelledby="calendar-title"
        >
          <div className="calendar-title">
            <h2 id="calendar-title">Calendar</h2>
            <button
              className="calendar-today"
              onClick={() => selectDate(today)}
            >
              Today
            </button>
          </div>
          <div className="calendar-month">
            <h3 aria-live="polite">{monthLabel}</h3>
            <div className="inline-actions">
              <button
                className="icon-button"
                onClick={() =>
                  setViewMonth(
                    new Date(
                      viewMonth.getFullYear(),
                      viewMonth.getMonth() - 1,
                      1,
                    ),
                  )
                }
                aria-label="Previous month"
              >
                <ChevronLeft />
              </button>
              <button
                className="icon-button"
                onClick={() =>
                  setViewMonth(
                    new Date(
                      viewMonth.getFullYear(),
                      viewMonth.getMonth() + 1,
                      1,
                    ),
                  )
                }
                aria-label="Next month"
              >
                <ChevronRight />
              </button>
            </div>
          </div>
          <table className="calendar-grid" ref={calendarRef}>
            <caption className="sr-only">
              {monthLabel}. Use arrow keys to choose a date.
            </caption>
            <thead>
              <tr>
                {weekdays.map((day) => (
                  <th key={day} scope="col">
                    {day}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {Array.from({ length: cells.length / 7 }, (_, row) => (
                <tr key={row}>
                  {cells.slice(row * 7, row * 7 + 7).map((date) => {
                    const iso = localISO(date);
                    const isSelected = iso === selectedISO;
                    const isPublished = !!demoWeek.days[iso]?.length;
                    return (
                      <td key={iso}>
                        <button
                          className={`date-button ${sameMonth(date, viewMonth) ? "" : "outside-month"} ${isPublished ? "has-classes" : ""}`}
                          data-date={iso}
                          tabIndex={iso === tabStop ? 0 : -1}
                          onClick={() => selectDate(date)}
                          onKeyDown={(event) => handleDateKeyDown(event, date)}
                          aria-label={`${formatDate(date, { weekday: "long", day: "numeric", month: "long", year: "numeric" })}${isPublished ? `, ${demoWeek.days[iso]!.length} published ${demoWeek.days[iso]!.length === 1 ? "class" : "classes"}` : ""}`}
                          aria-pressed={isSelected}
                          aria-current={
                            iso === localISO(today) ? "date" : undefined
                          }
                        >
                          {date.getDate()}
                        </button>
                      </td>
                    );
                  })}
                </tr>
              ))}
            </tbody>
          </table>
          <div className="calendar-legend">
            <span className="calendar-dot" aria-hidden="true" />
            Published classes
          </div>
        </section>
        <section
          className="panel day-agenda"
          aria-labelledby="day-agenda-title"
        >
          <div className="day-agenda-heading">
            <p className="eyebrow">Day schedule</p>
            <h2 id="day-agenda-title">
              {formatDate(selectedDate, {
                weekday: "long",
                day: "numeric",
                month: "short",
              })}
            </h2>
            {entries && (
              <span className="muted text-xs">
                {entries.length} {entries.length === 1 ? "class" : "classes"}{" "}
                scheduled
              </span>
            )}
          </div>
          <div aria-live="polite" aria-atomic="true">
            {entries ? (
              entries.length ? (
                entries.map((item) => (
                  <a
                    className="agenda-item"
                    key={`${item.code}-${item.from}`}
                    href={`#/subjects/${item.code}`}
                  >
                    <div className="agenda-time">
                      <time>
                        {item.from} – {item.to}
                      </time>
                      <Badge>{item.type}</Badge>
                    </div>
                    <h3>{subjectByCode(item.code)?.name || item.topic}</h3>
                    <p className="agenda-meta">
                      <MapPin aria-hidden="true" />
                      {item.room}
                    </p>
                  </a>
                ))
              ) : (
                <EmptyState
                  title="No classes scheduled"
                  description="There are no classes listed for this date."
                />
              )
            ) : (
              <EmptyState
                title="Timetable not published"
                description="No timetable is available for this date. The latest published week is 19–24 October."
              >
                <button className="text-link" onClick={returnToPublished}>
                  View published dates <ArrowRight />
                </button>
              </EmptyState>
            )}
          </div>
          <a
            className="text-link agenda-footer"
            href="#/schedule"
            onClick={resetWeek}
          >
            Full timetable <ArrowRight />
          </a>
        </section>
      </div>
    </aside>
  );
}
