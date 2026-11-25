import { createJudge } from "./judge.mjs";

try {
  const chunks = [];
  for await (const chunk of process.stdin) chunks.push(chunk);
  const input = JSON.parse(Buffer.concat(chunks).toString());
  const result = await createJudge().evaluate(input);
  process.stdout.write(JSON.stringify({ result }));
} catch (error) {
  const status = error.status || 500;
  const message = error.status ? error.message : "Grader failed.";
  process.stdout.write(JSON.stringify({ status, message }));
}
