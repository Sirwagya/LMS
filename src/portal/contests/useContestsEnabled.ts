import { useEffect, useState } from "react";
import { api } from "../api";

export function useContestsEnabled() {
  const [enabled, setEnabled] = useState(false);
  useEffect(() => {
    let current = true;
    void api<{ enabled: boolean }>("/contests/status")
      .then((status) => { if (current) setEnabled(status.enabled); })
      .catch(() => {});
    return () => { current = false; };
  }, []);
  return enabled;
}
