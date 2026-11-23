import { useState } from "react";
import { ChevronLeft, ChevronRight, MapPin } from "lucide-react";
import { useLMS } from "../../context/useLMS";
import { demoWeek } from "../../data/lmsData";
import { publishedMonday, formatDate, localISO } from "../../utils/academic";
import { PageHeader, Badge, EmptyState } from "../ui/Workspace";

export function ScheduleScreen() {
  const { currentSemester, weekOffset, changeWeek, resetWeek, subjectByCode } =
    useLMS();
  const monday = publishedMonday();
  monday.setDate(monday.getDate() + weekOffset * 7);
  const saturday = new Date(monday);
  saturday.setDate(saturday.getDate() + 5);
  const published =
    demoWeek.published &&
    Array.from({ length: 7 }, (_, i) => {
      const d = new Date(monday);
      d.setDate(d.getDate() + i);
      return (demoWeek.days[localISO(d)] || []).some(
        (e) => !!subjectByCode(e.code),
      );
    }).some(Boolean);
  const [today] = useState(() => localISO(new Date()));
  return (
    <section className="screen">
      <PageHeader
        eyebrow={`Semester ${currentSemester} · ${formatDate(monday, { day: "numeric", month: "short", year: "numeric" })} – ${formatDate(saturday, { day: "numeric", month: "short", year: "numeric" })}`}
        title="Timetable"
        description="Class times, rooms, and topics for your week."
        actions={
          currentSemester === 3 && (
            <>
              <button
                className="btn btn-secondary"
                onClick={resetWeek}
                disabled={weekOffset === 0}
              >
                Published week
              </button>
              <button
                className="icon-button"
                onClick={() => changeWeek(-1)}
                disabled={weekOffset <= -2}
                aria-label="Previous week"
              >
                <ChevronLeft />
              </button>
              <button
                className="icon-button"
                onClick={() => changeWeek(1)}
                disabled={weekOffset >= 8}
                aria-label="Next week"
              >
                <ChevronRight />
              </button>
            </>
          )
        }
      />
      {published ? (
        <div className="schedule-grid">
          {Array.from({ length: 6 }, (_, i) => {
            const date = new Date(monday);
            date.setDate(date.getDate() + i);
            const entries = (demoWeek.days[localISO(date)] || []).filter(
              (e) => !!subjectByCode(e.code),
            );
            return (
              <article key={i} className="panel schedule-day">
                <header>
                  <h2>{formatDate(date, { weekday: "long" })}</h2>
                  <span>
                    {today === localISO(date) ? (
                      <Badge tone="success">Today</Badge>
                    ) : (
                      formatDate(date)
                    )}
                  </span>
                </header>
                {entries.map((item) => (
                  <a
                    key={`${item.code}-${item.from}`}
                    href={`#/subjects/${item.code}`}
                    className="agenda-item"
                  >
                    <div className="flex items-center justify-between gap-2">
                      <p className="time">
                        {item.from} – {item.to}
                      </p>
                      <Badge>{item.type}</Badge>
                    </div>
                    <h3>{subjectByCode(item.code)?.name || item.code}</h3>
                    <p className="muted text-xs mb-3">{item.topic}</p>
                    <p className="agenda-meta">
                      <MapPin aria-hidden="true" />
                      {item.room}
                    </p>
                  </a>
                ))}
                {!entries.length && (
                  <EmptyState
                    title="No classes"
                    description="No classes are scheduled for this day."
                  />
                )}
              </article>
            );
          })}
        </div>
      ) : (
        <div className="panel">
          <EmptyState
            title={
              currentSemester < 3
                ? "This semester is archived"
                : "Timetable not published"
            }
            description={
              currentSemester < 3
                ? "Switch to Semester 3 to view the latest published timetable."
                : "There are no published classes for these dates. Browse weeks or return to the latest published week."
            }
          >
            {currentSemester === 3 && (
              <button className="btn btn-primary" onClick={resetWeek}>
                View published week
              </button>
            )}
          </EmptyState>
        </div>
      )}
    </section>
  );
}
