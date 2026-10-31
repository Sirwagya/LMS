import { useLMS } from "../../context/useLMS";
import { formatCurrency } from "../../utils/academic";
import { PageHeader, SectionHeader, Metric, Badge } from "../ui/Workspace";

export function FeesScreen() {
  const { fees, setIsPaymentModalOpen } = useLMS();
  const paid = fees.filter((f) => f.status === "Paid");
  const overdue = fees.filter((f) => f.status === "Overdue");
  const totalPaid = paid.reduce((sum, f) => sum + f.amount, 0);
  const outstanding = overdue.reduce((sum, f) => sum + f.amount, 0);
  return (
    <section className="screen">
      <PageHeader
        title="Fees & receipts"
        description="Your fee history and outstanding balance across all semesters."
      />
      <dl className="fee-summary">
        <Metric
          className="panel"
          label="Total paid"
          value={formatCurrency(totalPaid)}
          detail={`${paid.length} recorded installments`}
        />
        <Metric
          className={`panel ${outstanding ? "fee-due" : ""}`}
          label={outstanding ? "Overdue balance" : "Outstanding balance"}
          value={formatCurrency(outstanding)}
          detail={
            outstanding
              ? `${overdue.length} overdue installment · Due ${overdue[0]?.dueDate}`
              : "All recorded installments are paid"
          }
        />
      </dl>
      {outstanding > 0 && (
        <div className="flex flex-wrap items-center justify-between gap-4">
          <p className="muted text-xs">
            Review the invoice breakdown before arranging payment with the
            finance office.
          </p>
          <button
            className="btn btn-primary"
            onClick={() => setIsPaymentModalOpen(true)}
          >
            Review invoice
          </button>
        </div>
      )}
      <section>
        <SectionHeader title="Payment history" />
        <p className="muted text-xs mb-3 md:hidden">
          Swipe the table to view amounts and status →
        </p>
        <div
          className="panel table-scroll"
          tabIndex={0}
          role="region"
          aria-label="Fee history, scroll horizontally on smaller screens"
        >
          <table className="data-table">
            <caption className="sr-only">
              Fee installments, paid or due dates, amounts, and status
            </caption>
            <thead>
              <tr>
                <th scope="col">Installment</th>
                <th scope="col">Date</th>
                <th scope="col" className="amount">
                  Amount
                </th>
                <th scope="col">Status</th>
              </tr>
            </thead>
            <tbody>
              {fees.map((item) => (
                <tr key={item.id}>
                  <td>
                    <strong className="font-semibold">
                      {item.description}
                    </strong>
                    <small>
                      {item.semester} · {item.invoiceNo}
                    </small>
                  </td>
                  <td>
                    {item.paidDate || item.dueDate}
                    <small>
                      {item.status === "Paid" ? "Paid on" : "Due on"}
                    </small>
                  </td>
                  <td className="amount">{formatCurrency(item.amount)}</td>
                  <td>
                    <Badge
                      tone={item.status === "Paid" ? "success" : "warning"}
                    >
                      {item.status}
                    </Badge>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <p className="muted text-xs mt-4">
          Receipt files are not available here yet. Contact the finance office
          for an official copy.
        </p>
      </section>
    </section>
  );
}
