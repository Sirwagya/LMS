import { X } from "lucide-react";
import { useLMS } from "../../context/useLMS";

export function Toast() {
  const { toastMessage, showToast } = useLMS();
  return (
    <div role="status" aria-live="polite" aria-atomic="true">
      {toastMessage && (
        <div className="toast">
          <span>{toastMessage}</span>
          <button
            className="icon-button"
            onClick={() => showToast("")}
            aria-label="Dismiss notification"
          >
            <X />
          </button>
        </div>
      )}
    </div>
  );
}
