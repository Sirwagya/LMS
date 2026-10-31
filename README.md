# Vedam LMS · Student Workspace

A high-craft, editorial learning management and academic tracking system built for students at **Vedam School of Technology**. Designed with a focus on typography, calm information density, responsive layouts, and interactive academic performance analytics.

---

## ✦ Key Features

### 1. Dashboard (`#/dashboard`)
- **Academic Metrics**: Side-by-side overall attendance percentage (with SVG radial dial) and average contest score across evaluated courses.
- **Running / Upcoming Lecture**: Dynamic hero status showing current or next scheduled lecture with venue, time slot, instructor, and one-click lecture room join link.
- **Enrolled Courses**: Grid of active semester subjects with instructors, course topics, credit counts, and quick navigation.
- **Notice Board**: Institution announcements with priority tags and detailed side drawer reader.

### 2. Overview & Analytics (`#/overview`)
- **High-Level Statistics**: Semester-wide attendance percentage and contest mark performance.
- **Subject-Wise Cards**: Minimalist course cards displaying attendance % and contest performance % at a glance.
- **Interactive Contest Graphs**: Click any subject to reveal a smooth cubic-bezier contest progression chart featuring:
  - Interactive cursor hairline tracking
  - Glowing active data nodes
  - Floating metric tooltip card displaying contest date, title, and score
  - Clean horizontal gridlines and contextual percentage reference marks

### 3. Lectures & Course Materials (`#/lectures`)
- **Course Archive**: Browse lecture modules by course with real-time text search.
- **Interactive Reader**: Structured notes view with downloadable reference slides, syllabus attachments, and supplementary code files.
- **Key Takeaways & Discussion**: Summarized learning outcomes and simulated peer discussion thread.

### 4. Schedule & Timetable (`#/schedule`)
- **Interactive Monthly Calendar**: Day-by-day date grid with class indicator dots and active date highlighting.
- **Day Agenda**: Hourly chronological schedule for any selected date with instructor and venue badges.
- **Weekly Master Timetable**: Comprehensive matrix covering Monday through Friday classes and lab sessions.

### 5. Fees & Ledger (`#/fees`)
- **Tuition Overview**: Total semester fees, paid balance, and pending dues with deadline tracking.
- **Online Payment Modal**: Simulated secure payment flow with card and net-banking options.
- **Transaction History**: Downloadable fee receipts and itemized payment breakdown.

### 6. Fully Editable Account Profile (`#/account`)
- **Cover Banner & Avatar**: Upload custom cover banners and profile photos via local image picker with automatic preview, or use custom avatar initials.
- **Student Profile**: Customize full name, degree headline, and enrollment status badge.
- **Coding Profiles**: Edit handles, descriptions, and profile URLs for GitHub, LeetCode, CodeChef, and Codeforces, or add custom platforms.
- **Student Details**: Edit Enrollment ID, Programme, Current Semester, and Cohort.
- **Persistence**: All edits are automatically saved to `localStorage` with one-click **Save**, **Cancel**, and **Reset defaults** actions.

### 7. Global Schedule Rail & Theming
- **Persistent Side Rail**: Right-hand calendar rail with quick day agenda available across views, collapsing smoothly into an accordion on mobile viewports.
- **System Theme Switcher**: Full support for dark and light modes with custom CSS tokens and high-contrast accessibility.

---

## 🛠 Tech Stack

- **Framework**: [React 19](https://react.dev/) + [TypeScript](https://www.typescriptlang.org/)
- **Bundler & Tooling**: [Vite 8](https://vite.dev/)
- **Icons**: [Lucide React](https://lucide.dev/)
- **Linter**: [Oxlint](https://oxc.rs/)
- **Styling**: Handcrafted CSS design tokens (`--surface`, `--canvas`, `--ink`, `--accent`, etc.) with responsive grid layouts and fluid typography.

---

## 🚀 Getting Started

### Prerequisites
- Node.js 18.0 or later
- npm 9.0 or later

### Installation

```bash
# Clone the repository
git clone https://github.com/sirwagya/LMS.git
cd LMS

# Install dependencies
npm install
```

### Development

```bash
# Start the Vite development server
npm run dev
```

Open [http://localhost:5173](http://localhost:5173) in your browser.

### Linting & Type Checking

```bash
# Run Oxlint
npm run lint

# Type-check and build for production
npm run build
```

---

## 📂 Project Structure

```
LMS/
├── public/                 # Static public assets
├── src/
│   ├── assets/             # Brand logos & imagery
│   ├── components/
│   │   ├── layout/         # TopHeader, Navigation, ScheduleSidebar
│   │   ├── modals/         # NoticesDrawer, PaymentModal, Toast
│   │   ├── screens/        # Dashboard, Overview, Lectures, Schedule, Fees, Account
│   │   └── ui/             # Reusable UI primitives (PageHeader, Metric, Badge, etc.)
│   ├── context/            # LMSContext state provider & useLMS hook
│   ├── data/               # Academic curriculum, schedule, notices & contest datasets
│   ├── hooks/              # Custom React hooks (useDialog)
│   ├── utils/              # Academic calculations & formatters
│   ├── types.ts            # TypeScript interfaces & domain types
│   ├── index.css           # Global CSS variables, typography & responsive layouts
│   ├── App.tsx             # Root workspace container & hash router
│   └── main.tsx            # Application entry point
├── package.json
├── tsconfig.json
└── README.md
```

---

## 📄 License

Private educational and portfolio project. Built for Vedam School of Technology.
