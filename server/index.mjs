// Composition root: concrete adapters are wired here, not inside feature modules.
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";
import * as seed from "./seed.ts";
import { openDatabase } from "./infrastructure/database.mjs";
import { createRepositories } from "./infrastructure/repositories.mjs";
import { seedAccounts } from "./infrastructure/seed.mjs";
import { createCatalog } from "./domain/catalog.mjs";
import { createAuth } from "./domain/auth.mjs";
import { createWorkspace } from "./domain/workspace.mjs";
import { createPaymentService } from "./domain/payments.mjs";
import { createSessionTransport } from "./http/sessions.mjs";
import { createServer } from "./http/router.mjs";
import { createRoutes as routes0 } from "./routes/health.mjs";
import { createRoutes as routes1 } from "./routes/auth.mjs";
import { createRoutes as routes2 } from "./routes/workspace.mjs";
import { createRoutes as routes3 } from "./routes/profile.mjs";
import { createRoutes as routes4 } from "./routes/learning.mjs";
import { createRoutes as routes5 } from "./routes/fees.mjs";
import { createRoutes as routes6 } from "./routes/staff-records.mjs";
import { createRoutes as routes7 } from "./routes/staff-academic.mjs";
import { createRoutes as routes8 } from "./routes/staff-admin.mjs";
import { createRoutes as routes9 } from "./routes/staff-assessment.mjs";
import { createRoutes as routes10 } from "./routes/staff-schedule.mjs";
import { createJudge } from "./coding/judge.mjs";
import { createCodingRepository } from "./coding/repository.mjs";
import { createCodingService } from "./coding/service.mjs";
import { installCodingSamples } from "./coding/catalog.mjs";
import { createRoutes as codingRoutes } from "./routes/coding.mjs";
import { createRoutes as staffCodingRoutes } from "./routes/staff-coding.mjs";
const root = resolve(fileURLToPath(new URL("..", import.meta.url)));
const db = openDatabase(root),
  repos = createRepositories(db),
  catalog = createCatalog(repos, seed);
seedAccounts(repos, catalog.academic, seed);
installCodingSamples(catalog);
const codingService = createCodingService({
  judge: createJudge(),
  attempts: createCodingRepository(db),
  assignment: catalog.assignment,
  submissions: repos.submissions,
});
const auth = { ...createAuth(repos), ...createSessionTransport(repos) },
  bootstrap = createWorkspace(repos, catalog),
  paymentService = createPaymentService(repos);
const dependencies = {
  ...repos,
  ...catalog,
  ...auth,
  bootstrap,
  paymentService,
  codingService,
};
const routes = [
  ...codingRoutes(dependencies),
  ...staffCodingRoutes(dependencies),
  ...routes0(dependencies),
  ...routes1(dependencies),
  ...routes2(dependencies),
  ...routes3(dependencies),
  ...routes4(dependencies),
  ...routes5(dependencies),
  ...routes6(dependencies),
  ...routes7(dependencies),
  ...routes8(dependencies),
  ...routes9(dependencies),
  ...routes10(dependencies),
];
export const server = createServer({ routes, auth, root });
const port = Number(process.env.PORT || 3001);
server.listen(port, "127.0.0.1", () =>
  console.log("Vedam local LMS: http://127.0.0.1:" + port),
);
