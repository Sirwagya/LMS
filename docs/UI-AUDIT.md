# LMS design audit and corrections

Reviewed 31 October 2026. Audience: CSE undergraduates. Direction: clean, calm, premium, consistent, and useful for everyday academic work.

## Main diagnosis

The original interface felt uneven because it combined four different systems: a neutral navigation shell, glossy multicolour course cards, warm editorial dialogs, and a social-network-style profile. Everything competed for attention. A schedule rail on every page also made the actual coursework too narrow.

The larger problem was trust: several buttons claimed successful actions without performing them, and attendance and timetable states contradicted their underlying data.

The redesign uses one neutral surface system, forest green for actions and selection, a separate warning treatment, shared typography and spacing, and common page headers, buttons, badges, metrics, and empty states. The existing React, TypeScript, Vite, and Tailwind stack remains.

## Audit health

These are qualitative implementation-review scores, not Lighthouse results or a WCAG certification.

| Dimension            | Before /4 |  After /4 | Assessment                                                                                                                                              |
| -------------------- | --------: | --------: | ------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Accessibility        |         2 |         3 | Labels, semantic links, consistent focus styles, dialog focus management, and checked colour pairs. Full assistive-technology testing remains.          |
| Performance          |         2 |         3 | Removed perpetual aurora motion, large blurred layers, glass surfaces, and decorative effects. No production traffic or field performance measurements. |
| Responsive design    |         1 |         3 | All seven screen types fit tested widths from 320 to 1440px. Fee history intentionally scrolls within its labelled region on phones.                    |
| Theming              |         1 |         4 | Shared light/dark surface, text, border, action, selection, and warning tokens across the rendered UI.                                                  |
| Visual anti-patterns |         1 |         4 | Removed gradients, bevels, dummy avatars, social feed, repeated branding callouts, and competing primary actions.                                       |
| **Total**            |  **7/20** | **17/20** | **Poor → Good, within the tested frontend scope.**                                                                                                      |

The deterministic Impeccable source scan reported two font warnings before the work and no findings afterward. It did not detect most layout or truthfulness problems; those were established through source and browser review.

## Findings and resolutions

15 original findings: 0 P0, 9 P1, 5 P2, 1 P3. P1 denotes significant task difficulty or misleading feedback; P2 denotes usability friction; P3 denotes polish.

| Priority | Finding and student impact                                                                                                                  | Resolution                                                                                                                                                                    | Main location                                     |
| -------- | ------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------- |
| P1       | Permanent schedule rail left course cards about 188px wide at a laptop viewport; titles wrapped heavily and a fees CTA clipped.             | Schedule preview appears only on the current-semester dashboard. Other pages use the full workspace.                                                                          | `src/App.tsx`, `src/index.css`                    |
| P1       | Search, downloads, recordings, submissions, export, sync, receipts, and coding links often produced a toast without the promised result.    | Course search filters real course records; coding profiles use actual external links. Unsupported actions are removed and availability is stated inline.                      | Dashboard, Lectures, Career, Fees, Account        |
| P1       | Payment simulated verification after a timeout and changed the balance without processing payment.                                          | Replaced checkout with a read-only invoice review. Removed the fake payment mutation and encryption claims.                                                                   | `src/components/modals/PaymentModal.tsx`, context |
| P1       | Attendance appeared as lesson-completion progress, with duplicate percentages.                                                              | Attendance is explicitly labelled and derived from recorded class counts.                                                                                                     | Dashboard, Lectures, `src/utils/academic.ts`      |
| P1       | Static skip buffers contradicted the calculator; aggregate advice could conceal a failing course.                                           | Calculator selects one course, derives allowable absences/recovery from counts, and explains the per-course 75% requirement. Archived semesters do not offer future planning. | Overview, `src/utils/academic.ts`                 |
| P1       | Calendar labelled 19 October “Today” and repeated template classes on arbitrary unpublished dates.                                          | Both schedule views use only published date records. Other weeks show an unpublished state. Current-day labels use the actual date.                                           | ScheduleSidebar, ScheduleScreen                   |
| P1       | “Settings” opened an oversized social profile; actual settings were buried below the fold.                                                  | Navigation says Account. Institutional details and appearance preferences are immediately available.                                                                          | Sidebar, AccountScreen                            |
| P1       | Dialogs had dialog roles but did not trap focus, restore focus, or make the background inert.                                               | Shared dialog hook manages keyboard focus, Escape, restoration, background inertness, and scroll locking.                                                                     | `src/hooks/useDialog.ts`, dialogs                 |
| P1       | Zero-result lecture search retained unrelated lecture details, and lecture selection was absent from the URL.                               | Empty results clear the reader. Lecture links are addressable, support browser Back, and move focus to the reader on phones.                                                  | LecturesScreen, context                           |
| P2       | Saturated gradients, moving glow, glass, bevels, multiple font systems, and many button styles competed with coursework.                    | Unified tokens, Manrope UI typography, restrained borders, flat buttons, and compact metadata. Removed route fade to keep navigation immediate.                               | index.css, shared UI, layout                      |
| P2       | Notice categories were converted into invented statuses such as “Completed” and “Mandatory.”                                                | Notices display actual categories and dated updates. Removed the hackathon link that implied registration but opened an unrelated timetable.                                  | Dashboard, NoticesDrawer, data                    |
| P2       | Account and Career duplicated bios, school history, coding statistics, and actions.                                                         | Account owns student identity/preferences; Career owns coding profiles and recent evaluated work.                                                                             | AccountScreen, CareerScreen                       |
| P2       | Unexplained avatar piles, fake verification, social engagement, “passport” callouts, and decorative timeline ticks added no academic value. | Removed these elements. Timetable entries show meaningful topic, session type, room, and actual time instead.                                                                 | Dashboard, Account, schedule views                |
| P2       | “Telemetry & Forecasting,” “safe skip buffer,” and inconsistent navigation labels required unnecessary interpretation.                      | Plain labels describe the destination or task. Overview, notices, fees, and account use common terminology.                                                                   | Screen headings and navigation                    |
| P3       | Inconsistent corner radii, tiny supporting text, and decorative controls made similar elements feel unrelated.                              | Shared component styles, increased supporting text, common active/hover states, and consistent labelled controls.                                                             | index.css, `src/components/ui/Workspace.tsx`      |

## Page-by-page outcome

| Page                      | What now belongs here                                                                                            | Removed or corrected                                                                                                                               |
| ------------------------- | ---------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------- |
| Dashboard                 | Semester summary, all enrolled courses, working course search, notice previews, published-week agenda.           | Three-course gate, dummy classmates, duplicate attendance, glossy progress bars, fake global Search. Archived courses are labelled correctly.      |
| Course / lectures         | Course identity, full lecture titles, search, selected lecture notes, material metadata, assignment records.     | Narrow three-column layout, misleading download/play/submit controls, unrelated reader after an empty search. Added mobile return to lecture list. |
| Attendance & results      | Course attendance, actual counts, contest results, course-specific attendance planning.                          | Static contradictory buffer cards, jargon, aggregate absence advice, archive planning controls.                                                    |
| Timetable                 | Published dates, course, topic, session type, room, time, and clear week controls.                               | Duplicate rail, fake “Today,” repeated unpublished classes, nested coloured cards.                                                                 |
| Career                    | Direct coding-profile links and recent evaluated course work.                                                    | Fake live ratings/sync, resume-export toast, repeated student biography and school tiles.                                                          |
| Fees & receipts           | Paid total, clearly overdue balance, invoice details, dated payment history.                                     | Fake checkout and receipt downloads, security guarantees, clipped payment CTA. Added a visible mobile scrolling hint.                              |
| Account                   | Institutional identity, enrollment ID copying, programme/section/advisor, appearance and selected academic view. | Empty cover banner, social feed, likes/bookmarks, fake verification, duplicated coding-profile dashboard.                                          |
| Notices / invoice / toast | Consistent surfaces, actual categories, meaningful details, dismiss controls, neutral feedback.                  | Legacy fonts/colours, invented statuses, generic success icon on failures, unsafe focus behavior.                                                  |

## Validation performed

- Production TypeScript/Vite build and Oxlint checks pass with no warnings.
- All seven screen types checked for document overflow at 320, 390, 768, 1024, 1280, and 1440px; none exceeded the viewport. Navigation and the fee table have intentional contained scrolling.
- Light and dark themes inspected. Checked text/action colour pairs range from **4.63:1** to **14.87:1**; minimum checked muted-text pair is 4.63:1. This does not certify every possible browser or assistive-technology state.
- All 14 course pages checked at phone and desktop widths. Course search, empty results, lecture search, selection URLs, browser Back, direct archived-course loading, and semester switching checked.
- Attendance calculator checked for course switching, reset, below-threshold recovery, and baseline math: DBMS 17/18 = 94.4%, with four allowable future absences at a 75% threshold.
- Published and unpublished timetable states checked; archived semesters show the appropriate explanation.
- Invoice dialog focus trap, Escape dismissal, focus restoration, and inert background checked. Notices use the same dialog mechanism.
- Independent follow-up review identified mobile lecture focus and fee-table discoverability; both were corrected.

## Practical limits and next work

The app still uses local fixture records. Downloads, recordings, assignment submission, receipt storage, and online payment have no connected implementation in this repository. Their previous false-success controls were removed; this work does not implement those services or verify external profile ownership.

Before connecting live student data, replace fixtures with the institutional source of truth and implement real file/submission/payment status handling. Confirm the institution’s attendance policy rather than assuming the existing 75% rule covers every course exception. A screen-reader pass, text-zoom checks, and field performance measurements would strengthen the current frontend verification.

For future UI work, use `.impeccable.md` and the shared components/tokens. Prioritize content and task layout, then responsive adaptation, then copy clarity, and finish with a polish pass. Re-run the audit after meaningful interface changes.

## Usability baseline and student scenarios

The independent review scored the original design **14/40** across Nielsen’s heuristics:

| Heuristic         | Before /4 | Main evidence                                                           |
| ----------------- | --------: | ----------------------------------------------------------------------- |
| System status     |         1 | Success feedback often described actions that did not occur.            |
| Real-world match  |         2 | Course content was relevant, but product jargon obscured routine tasks. |
| User control      |         3 | Browser navigation, Escape, cancel, and reset were useful foundations.  |
| Consistency       |         1 | Different visual systems and duplicate destinations.                    |
| Error prevention  |         1 | Misleading absence guidance and simulated payment state.                |
| Recognition       |         2 | Labelled navigation helped; settings and course titles were obscured.   |
| Efficiency        |         1 | Fake search and extra course-expansion step.                            |
| Minimalist design |         1 | Decorative cards, avatars, constant rail, duplicated profile.           |
| Error recovery    |         2 | Some clear missing-course and clipboard feedback.                       |
| Help              |         0 | No contextual attendance-rule explanation or availability guidance.     |

The original cognitive-load checklist failed five of eight items: single focus, hierarchy, one decision at a time, minimal choices, and progressive disclosure. This is a qualitative diagnosis, not a controlled user study.

Three important student scenarios guided corrections:

- **Between classes:** a student needs a reliable date, time, and room, without a false “Today” label or invented future classes.
- **Near the attendance threshold:** a student needs per-course calculations, clear assumptions, and counts consistent with the displayed percentage.
- **Revising on a phone or laptop:** a student needs full lecture titles, searchable notes, predictable Back navigation, and honest material availability.

The original substantive CSE course material, labelled navigation, theme support, reduced-motion preference, and keyboard skip link were useful foundations and were retained or strengthened.
