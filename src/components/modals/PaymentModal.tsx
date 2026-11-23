import { useCallback, useState } from "react";
import { X } from "lucide-react";
import { useLMS } from "../../context/useLMS";
import { useDialog } from "../../hooks/useDialog";
import { formatCurrency } from "../../utils/academic";
import { Badge } from "../ui/Workspace";

import { api } from "../../portal/api";
import { useSession } from "../../portal/session";

export function PaymentModal() {
  const { refresh } = useSession();
  const [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const { isPaymentModalOpen, setIsPaymentModalOpen, fees } = useLMS();
  const close = useCallback(
    () => setIsPaymentModalOpen(false),
    [setIsPaymentModalOpen],
  );
  const ref = useDialog(isPaymentModalOpen, close);
  const invoice = fees.find((f) => f.status !== "Paid");
  if (!isPaymentModalOpen || !invoice) return null;
  return (
    <div className="overlay" onClick={close}>
      <div
        ref={ref}
        role="dialog"
        aria-modal="true"
        aria-labelledby="invoice-title"
        className="dialog"
        tabIndex={-1}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="dialog-heading">
          <h2 id="invoice-title">Invoice details</h2>
          <button
            className="icon-button"
            onClick={close}
            aria-label="Close invoice"
          >
            <X />
          </button>
        </div>
        <p className="record-id mb-3">{invoice.invoiceNo}</p>
        <Badge tone="warning">{invoice.status}</Badge>
        <p className="mt-4 mb-6 font-semibold">
          {invoice.semester} · {invoice.description}
        </p>
        <dl className="detail-list">
          <div>
            <dt>Due date</dt>
            <dd>{invoice.dueDate}</dd>
          </div>
          <div>
            <dt>Total payable</dt>
            <dd>{formatCurrency(invoice.amount)}</dd>
          </div>
        </dl>
        <p className="inline-note mt-6">
          Local payment simulation. No card or bank details are needed and no
          real money will be collected. A sample receipt will be generated.
        </p>
        {error && <p role="alert">{error}</p>}
        <button
          className="btn btn-primary w-full mt-6"
          disabled={busy}
          onClick={async () => {
            setBusy(true);
            setError("");
            try {
              await api("/payments", "POST", { feeId: invoice.id });
              await refresh();
              close();
            } catch (e) {
              setError((e as Error).message);
            } finally {
              setBusy(false);
            }
          }}
        >
          {busy ? "Processing…" : "Simulate payment"}
        </button>
        <button className="btn btn-secondary w-full mt-3" onClick={close}>
          Close invoice
        </button>
      </div>
    </div>
  );
}
