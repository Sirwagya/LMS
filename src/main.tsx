import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "./index.css";
import Portal from "./portal/Portal.tsx";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <Portal />
  </StrictMode>,
);
