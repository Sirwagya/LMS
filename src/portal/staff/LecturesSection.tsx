import { fileBase64 } from "../api";
import { Field, CourseSelect, LectureSelect, ActionForm } from "./Forms";
export function LecturesSection({ reload }: { reload: () => Promise<void> }) {
  return (
    <div className="staff-grid">
      <ActionForm
        title="Publish lecture notes"
        endpoint="/staff/lectures"
        onDone={reload}
      >
        <CourseSelect />
        <Field label="Lecture title" name="title" />
        <label>
          Lecture notes
          <textarea name="summary" rows={8} maxLength={20000} required />
        </label>
      </ActionForm>
      <ActionForm
        title="Upload materials or recording"
        endpoint="/staff/assets"
        onDone={reload}
        transform={async (d, form) => {
          const f = (form.elements.namedItem("upload") as HTMLInputElement)
            .files?.[0];
          if (!f) throw new Error("Select a file.");
          return {
            ...d,
            name: f.name,
            bytes: await fileBase64(f),
            recording: d.kind === "recording",
          };
        }}
      >
        <LectureSelect />
        <label>
          Purpose
          <select name="kind">
            <option value="material">Downloadable material</option>
            <option value="recording">Lecture recording (MP4/WebM)</option>
          </select>
        </label>
        <label>
          File (up to 10 MB)
          <input name="upload" type="file" required />
        </label>
        <p className="muted">Use a small sample recording for local testing.</p>
      </ActionForm>
    </div>
  );
}
