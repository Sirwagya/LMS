import { useCallback } from "react";
import { X } from "lucide-react";
import { useLMS } from "../../context/useLMS";
import { useDialog } from "../../hooks/useDialog";
import { formatCurrency } from "../../utils/academic";
import { Badge } from "../ui/Workspace";

export function PaymentModal() {
  const { isPaymentModalOpen, setIsPaymentModalOpen, fees } = useLMS();
  const close = useCallback(
    () => setIsPaymentModalOpen(false),
    [setIsPaymentModalOpen],
  );
  const ref = useDialog(isPaymentModalOpen, close);
  const invoice = fees.find((f) => f.status === "Overdue");
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
            <dt>Academic tuition fee</dt>
            <dd>{formatCurrency(115000)}</dd>
          </div>
          <div>
            <dt>Systems & GPU lab infrastructure</dt>
            <dd>{formatCurrency(16250)}</dd>
          </div>
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
          Online payment is not available in this workspace. Contact the finance
          office to arrange payment and confirm your balance.
        </p>
        <button className="btn btn-secondary w-full mt-6" onClick={close}>
          Close invoice
        </button>
      </div>
    </div>
  );
}
