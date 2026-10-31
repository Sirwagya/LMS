import { useContext } from "react";
import { LMSContext } from "./state";

export function useLMS() {
  const context = useContext(LMSContext);
  if (!context) throw new Error("useLMS must be used within an LMSProvider");
  return context;
}
