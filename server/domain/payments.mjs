import { randomUUID } from "node:crypto";
// A unit of work keeps the payment and invoice transition atomic.
export function createPaymentService({ payments, fees, transaction }) {
  return {
    pay(userId, feeId, item) {
      return transaction(() => {
        payments.create(randomUUID(), userId, feeId, new Date().toISOString());
        fees.update(
          JSON.stringify({
            ...item,
            status: "Paid",
            paidDate: new Date().toISOString().slice(0, 10),
            receiptAvailable: true,
            simulated: true,
          }),
          feeId,
        );
      });
    },
  };
}
