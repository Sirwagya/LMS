export function createCodingRepository(db) {
  db.exec(
    `CREATE TABLE IF NOT EXISTS coding_attempts(id TEXT PRIMARY KEY,user_id TEXT REFERENCES users(id),assignment_id TEXT,language TEXT,source TEXT,verdict TEXT,passed INTEGER,total INTEGER,result TEXT,created TEXT);`,
  );
  return {
    save: (a) =>
      db
        .prepare("INSERT INTO coding_attempts VALUES(?,?,?,?,?,?,?,?,?,?)")
        .run(
          a.id,
          a.userId,
          a.assignmentId,
          a.language,
          a.source,
          a.verdict,
          a.passed,
          a.total,
          JSON.stringify(a.result),
          a.created,
        ),
    history: (userId, assignmentId) =>
      db
        .prepare(
          "SELECT id,language,source,verdict,passed,total,result,created FROM coding_attempts WHERE user_id=? AND assignment_id=? ORDER BY created DESC LIMIT 20",
        )
        .all(userId, assignmentId)
        .map((a) => ({ ...a, result: JSON.parse(a.result) })),
  };
}
