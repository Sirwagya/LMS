import { test } from "node:test";
import assert from "node:assert/strict";
import { DatabaseSync } from "node:sqlite";
import { createRepositories } from "./infrastructure/repositories.mjs";
import { createPaymentService } from "./domain/payments.mjs";

test("payment unit of work rolls back both records if ledger persistence fails", () => {
  const db = new DatabaseSync(":memory:");
  db.exec(`CREATE TABLE payments(id TEXT,user_id TEXT,fee_id TEXT,created TEXT);
    CREATE TABLE fees(id TEXT,user_id TEXT,data TEXT);
    INSERT INTO fees VALUES('invoice','student','{"status":"Pending"}');
    CREATE TRIGGER reject_update BEFORE UPDATE ON fees BEGIN SELECT RAISE(ABORT,'ledger unavailable'); END;`);
  const payment = createPaymentService(createRepositories(db));
  assert.throws(
    () => payment.pay("student", "invoice", { status: "Pending" }),
    /ledger unavailable/,
  );
  assert.equal(db.prepare("SELECT COUNT(*) AS n FROM payments").get().n, 0);
  assert.equal(
    JSON.parse(db.prepare("SELECT data FROM fees").get().data).status,
    "Pending",
  );
  db.close();
});

test("payment service accepts in-memory repository ports without HTTP or SQLite dependencies", () => {
  const payments = [],
    invoices = new Map([["invoice", { status: "Pending", amount: 100 }]]);
  const payment = createPaymentService({
    payments: { create: (...entry) => payments.push(entry) },
    fees: { update: (data, id) => invoices.set(id, JSON.parse(data)) },
    transaction: (operation) => operation(),
  });
  payment.pay("student", "invoice", invoices.get("invoice"));
  assert.equal(payments.length, 1);
  assert.equal(invoices.get("invoice").status, "Paid");
  assert.equal(invoices.get("invoice").amount, 100);
  assert.equal(invoices.get("invoice").simulated, true);
});
