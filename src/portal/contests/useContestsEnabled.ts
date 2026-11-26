import { useEffect, useState } from "react";
import { api } from "../api";

export function useContestFeatures() {
  const [features, setFeatures] = useState({ enabled: false, attemptsEnabled: false });
  useEffect(() => {
    let current = true;
    void api<typeof features>("/contests/status")
      .then((status) => { if (current) setFeatures(status); })
      .catch(() => {});
    return () => { current = false; };
  }, []);
  return features;
}

export function useContestsEnabled() {
  return useContestFeatures().enabled;
}
