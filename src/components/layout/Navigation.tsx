import {
  LayoutGrid,
  ChartNoAxesCombined,
  CalendarDays,
  ReceiptText,
  UserRound,
} from "lucide-react";
import { useLMS } from "../../context/useLMS";

const destinations = [
  { path: "/dashboard", label: "Dashboard", icon: LayoutGrid },
  {
    path: "/overview",
    label: "Attendance & results",
    icon: ChartNoAxesCombined,
  },
  { path: "/schedule", label: "Timetable", icon: CalendarDays },
  { path: "/fees", label: "Fees", icon: ReceiptText },
  { path: "/account", label: "Account", icon: UserRound },
];

export function Navigation() {
  const { route, hasOverdueFees } = useLMS();
  return (
    <nav className="main-nav" aria-label="Main navigation">
      {destinations.map(({ path, label, icon: Icon }) => (
        <a
          key={path}
          href={`#${path}`}
          className="nav-item"
          aria-current={
            route === path ||
            (path === "/dashboard" && route.startsWith("/subjects/"))
              ? "page"
              : undefined
          }
        >
          <Icon aria-hidden="true" />
          <span>{label}</span>
          {path === "/fees" && hasOverdueFees && (
            <span className="nav-dot" aria-label="Overdue balance" />
          )}
        </a>
      ))}
    </nav>
  );
}
