import React from "react";
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

const AppContent: React.FC = () => {
  const { route, currentSemester } = useLMS();
  const showSchedule = route === "/dashboard" && currentSemester === 3;

  const screenKey = route.startsWith("/subjects/")
    ? `subject-${route.split("/")[2]}`
    : route;

  const renderCurrentScreen = () => {
    if (route.startsWith("/subjects/")) {
      return <LecturesScreen />;
    }
    switch (route) {
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
