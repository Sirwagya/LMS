import { useEffect, useState } from "react";
import { api } from "../api";
import type { StaffData } from "../staff/types";

export function RosterPicker({ studentIds, onChange }: {
  studentIds: string[];
  onChange: (ids: string[]) => void;
}) {
  const [page, setPage] = useState(0);
  const [result, setResult] = useState<StaffData | null>(null);
  const [error, setError] = useState("");
  useEffect(() => {
    let current = true;
    void api<StaffData>(`/staff?usersPage=${page}`)
      .then((data) => { if (current) { setResult(data); setError(""); } })
      .catch((e: Error) => { if (current) setError(e.message); });
    return () => { current = false; };
  }, [page]);
  function navigate(next: number) {
    setResult(null);
    setPage(next);
  }
  const students = result?.users.filter((user) => user.role === "student") ?? [];
  return (
    <fieldset className="portal-form">
      <legend>Eligible students</legend>
      <p>{studentIds.length} selected</p>
      {error && <p role="alert">{error}</p>}
      {!result && !error && <p role="status">Loading students…</p>}
      {result && students.length === 0 && <p className="muted">No student accounts on this page.</p>}
      <div className="space-y-2">
        {students.map((user) => (
          <label key={user.id} className="contest-roster-choice">
            <input type="checkbox" checked={studentIds.includes(user.id)} onChange={(e) => onChange(e.target.checked
              ? [...studentIds, user.id]
              : studentIds.filter((id) => id !== user.id))} />
            <span>{user.name} · {user.email}</span>
          </label>
        ))}
      </div>
      {(page > 0 || result?.hasMore.users) && (
        <div className="inline-actions">
          <button type="button" className="btn btn-secondary" disabled={page === 0} onClick={() => navigate(page - 1)}>Previous students</button>
          <button type="button" className="btn btn-secondary" disabled={!result?.hasMore.users || page >= 10000} onClick={() => navigate(page + 1)}>Next students</button>
        </div>
      )}
    </fieldset>
  );
}
