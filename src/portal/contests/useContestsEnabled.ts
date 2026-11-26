import { useEffect, useState } from "react";
import { api } from "../api";

type ContestStatusResponse = {
  enabled: boolean;
  attemptsEnabled: boolean;
  integrityEnabled?: boolean;
  resultsEnabled?: boolean;
};

export type ContestFeatures = {
  enabled: boolean;
  attemptsEnabled: boolean;
  integrityEnabled: boolean;
  resultsEnabled: boolean;
};

export function useContestFeatures(): ContestFeatures {
  const [features, setFeatures] = useState<ContestFeatures>({
    enabled: false,
    attemptsEnabled: false,
    integrityEnabled: false,
    resultsEnabled: false,
  });
  useEffect(() => {
    let current = true;
    void api<ContestStatusResponse>("/contests/status")
      .then((status) => {
        if (current) {
          setFeatures({
            enabled: Boolean(status.enabled),
            attemptsEnabled: Boolean(status.attemptsEnabled),
            integrityEnabled: Boolean(status.integrityEnabled),
            resultsEnabled: Boolean(status.resultsEnabled),
          });
        }
      })
      .catch(() => {});
    return () => { current = false; };
  }, []);
  return features;
}

export function useContestsEnabled() {
  return useContestFeatures().enabled;
}
