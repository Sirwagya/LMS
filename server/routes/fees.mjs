import { fail } from "../domain/validation.mjs";
import { send, file, body } from "../http/response.mjs";
export function createRoutes({ fees, paymentService }) {
  return [
    {
      match: ({ path, method }) =>
        path === "/api/payments" && method === "POST",
      access: "authenticated",
      handle: async ({ req, res, u }) => {
        if (u.role !== "student") fail(403, "Student access required.");
        const b = await body(req);
        const f = fees.ownedBy(b.feeId, u.id);
        if (!f) fail(404, "Invoice not found.");
        const item = JSON.parse(f.data);
        if (item.status === "Paid") fail(409, "Invoice already paid.");
        paymentService.pay(u.id, f.id, item);
        return send(res, 200, { ok: true });
      },
    },
    {
      match: ({ path, method }) =>
        path.startsWith("/api/receipts/") && method === "GET",
      access: "authenticated",
      handle: async ({ res, path, u }) => {
        const f = fees.ownedBy(path.split("/").pop(), u.id);
        if (!f) fail(404, "Receipt not found.");
        const a = JSON.parse(f.data);
        if (a.status !== "Paid") fail(409, "Invoice is unpaid.");
        return file(
          res,
          `VEDAM LOCAL LMS — SAMPLE RECEIPT\nNot an official payment receipt. No real money collected.\n\nStudent: ${u.name}\nInvoice: ${a.invoiceNo}\nDescription: ${a.description}\nAmount: INR ${a.amount}\nPaid: ${a.paidDate}\n`,
          `${a.invoiceNo || a.id}-sample-receipt.txt`,
          "text/plain; charset=utf-8",
        );
      },
    },
  ];
}
