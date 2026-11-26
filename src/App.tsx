import { SubjectScreen } from "./components/screens/SubjectScreen";
import { AssignmentDetail } from "./portal/AssignmentDetail";
import { lecturesBySubject } from "./data/lmsData";
import { resolveLearningRoute } from "./utils/learningRoutes";
import React from "react";
import { AssignmentsScreen } from "./portal/AssignmentsScreen";
import { useSession } from "./portal/session";
import { LMSProvider } from "./context/LMSContext";
import { useLMS } from "./context/useLMS";
import { PageHeader } from "./components/ui/Workspace";
import { TopHeader } from "./components/layout/TopHeader";
import { ScheduleSidebar } from "./components/layout/ScheduleSidebar";
import { DashboardScreen } from "./components/screens/DashboardScreen";
import { LecturesScreen } from "./components/screens/LecturesScreen";
import { OverviewScreen } from "./components/screens/OverviewScreen";
import { ScheduleScreen } from "./components/screens/ScheduleScreen";
import { FeesScreen } from "./components/screens/FeesScreen";
import { AccountScreen } from "./components/screens/AccountScreen";
import { PaymentModal } from "./components/modals/PaymentModal";
import { NoticesDrawer } from "./components/modals/NoticesDrawer";
import { Toast } from "./components/modals/Toast";

const CodingWorkspace = React.lazy(() =>
  import("./portal/coding/CodingWorkspace").then((m) => ({
    default: m.CodingWorkspace,
  })),
);

const ContestAttempt = React.lazy(() =>
  import("./portal/contests/ContestAttempt").then((m) => ({ default: m.ContestAttempt })),
);

const ContestResults = React.lazy(() =>
  import("./portal/contests/ContestResults").then((m) => ({ default: m.ContestResults })),
);

const AppContent: React.FC = () => {
  const { route, currentSemester } = useLMS();
  const { user } = useSession();
  const contest = /^\/contests\/([a-f0-9-]{36})$/.exec(route);
  if (contest) {
    return (
      <main id="main-content" className="contest-shell" tabIndex={-1}>
        <React.Suspense fallback={<p role="status">Opening contest…</p>}>
          <ContestAttempt key={`${user.id}:${contest[1]}`} id={contest[1]} />
        </React.Suspense>
      </main>
    );
  }
  const showSchedule = route === "/dashboard" && currentSemester === 3;

  const screenKey = route;

  const renderCurrentScreen = () => {
    const contestResults = /^\/contests\/([a-f0-9-]{36})\/results$/.exec(route);
    if (contestResults) {
      return (
        <React.Suspense fallback={<p role="status">Loading contest results…</p>}>
          <ContestResults key={`${user.id}:${contestResults[1]}`} id={contestResults[1]} />
        </React.Suspense>
      );
    }
    const learning = resolveLearningRoute(route, lecturesBySubject);
    if (learning?.assignment?.kind === "coding")
      return (
        <React.Suspense fallback={<p role="status">Opening code editor…</p>}>
          <CodingWorkspace />
        </React.Suspense>
      );
    if (learning?.kind === "assignment") return <AssignmentDetail />;
    if (learning?.kind === "subject") return <SubjectScreen />;
    if (learning?.kind === "lecture") return <LecturesScreen />;
    switch (route) {
      case "/assignments":
        return <AssignmentsScreen />;
      case "/overview":
        return <OverviewScreen />;
      case "/schedule":
        return <ScheduleScreen />;
      case "/fees":
        return <FeesScreen />;
      case "/account":
        return <AccountScreen />;
      case "/dashboard":
        return <DashboardScreen />;
      default:
        return (
          <section className="screen">
            <PageHeader
              title="Page not found"
              description="This page is not part of your workspace. Return to your courses to continue."
            />
            <a className="btn btn-primary" href="#/dashboard">
              Back to dashboard
            </a>
          </section>
        );
    }
  };

  return (
    <div className="app-shell">
      <a
        href="#main-content"
        className="skip-link"
        onClick={(e) => {
          e.preventDefault();
          document.getElementById("main-content")?.focus();
        }}
      >
        Skip to main content
      </a>
      <div className="workspace-shell">
        <TopHeader />
        <div className={`workspace-body ${showSchedule ? "with-rail" : ""}`}>
          <main id="main-content" tabIndex={-1}>
            <div key={screenKey}>{renderCurrentScreen()}</div>
          </main>
          {showSchedule && <ScheduleSidebar />}
        </div>
      </div>
      <PaymentModal />
      <NoticesDrawer />
      <Toast />
    </div>
  );
};

export const App: React.FC = () => {
  return (
    <LMSProvider>
      <AppContent />
    </LMSProvider>
  );
};

export default App;
