import { useCallback } from "react";
import { X, ArrowRight } from "lucide-react";
import { useLMS } from "../../context/useLMS";
import { drawerNotices } from "../../data/lmsData";
import { useDialog } from "../../hooks/useDialog";
import { Badge } from "../ui/Workspace";

export function NoticesDrawer() {
  const { isNoticesDrawerOpen, setIsNoticesDrawerOpen } = useLMS();
  const close = useCallback(
    () => setIsNoticesDrawerOpen(false),
    [setIsNoticesDrawerOpen],
  );
  const ref = useDialog(isNoticesDrawerOpen, close);
  if (!isNoticesDrawerOpen) return null;
  return (
    <div className="overlay drawer-overlay" onClick={close}>
      <div
        ref={ref}
        role="dialog"
        aria-modal="true"
        aria-labelledby="notices-title"
        tabIndex={-1}
        className="drawer"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="dialog-heading">
          <div>
            <h2 id="notices-title">Notice board</h2>
            <p className="muted text-xs mt-1">Academic and campus updates</p>
          </div>
          <button
            className="icon-button"
            onClick={close}
            aria-label="Close notices"
          >
            <X />
          </button>
        </div>
        {drawerNotices.map((n) => (
          <article className="drawer-notice" key={n.id}>
            <div className="flex justify-between gap-4 items-center">
              <Badge tone={n.tag === "URGENT" ? "warning" : "neutral"}>
                {n.tag === "URGENT"
                  ? "Time sensitive"
                  : n.tag.charAt(0) + n.tag.slice(1).toLowerCase()}
              </Badge>
              <span className="muted text-xs">{n.time}</span>
            </div>
            <h3>{n.title}</h3>
            <p>{n.desc}</p>
            {n.actionLabel && n.actionRoute && (
              <a
                href={`#${n.actionRoute}`}
                onClick={close}
                className="text-link mt-2"
              >
                {n.actionLabel}
                <ArrowRight />
              </a>
            )}
          </article>
        ))}
      </div>
    </div>
  );
}
