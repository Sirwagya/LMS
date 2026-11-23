// SQLite adapters. SQL stays behind the domain-specific repository ports.
export function createRepositories(db) {
  return {
    settings: {
      get: (...args) =>
        db.prepare("SELECT value FROM settings WHERE key=?").get(...args),
      create: (...args) =>
        db.prepare("INSERT INTO settings VALUES(?,?)").run(...args),
      update: (...args) =>
        db.prepare("UPDATE settings SET value=? WHERE key=?").run(...args),
    },
    users: {
      first: (...args) =>
        db.prepare("SELECT id FROM users LIMIT 1").get(...args),
      create: (...args) =>
        db
          .prepare(
            "INSERT INTO users(id,email,name,role,password) VALUES(?,?,?,?,?)",
          )
          .run(...args),
      findByRole: (...args) =>
        db.prepare("SELECT * FROM users WHERE id=? AND role=?").get(...args),
      byEmail: (...args) =>
        db.prepare("SELECT * FROM users WHERE email=?").get(...args),
      emailExists: (...args) =>
        db.prepare("SELECT id FROM users WHERE email=?").get(...args),
      byId: (...args) =>
        db.prepare("SELECT * FROM users WHERE id=?").get(...args),
      saveProfile: (...args) =>
        db.prepare("UPDATE users SET name=?,profile=? WHERE id=?").run(...args),
      savePassword: (...args) =>
        db.prepare("UPDATE users SET password=? WHERE id=?").run(...args),
      all: (...args) => db.prepare("SELECT * FROM users").all(...args),
    },
    attendance: {
      seed: (...args) =>
        db.prepare("INSERT INTO attendance VALUES(?,?,?,?)").run(...args),
      forCourse: (...args) =>
        db
          .prepare("SELECT * FROM attendance WHERE user_id=? AND course=?")
          .get(...args),
      all: (...args) => db.prepare("SELECT * FROM attendance").all(...args),
      save: (...args) =>
        db
          .prepare(
            "INSERT INTO attendance VALUES(?,?,?,?) ON CONFLICT(user_id,course) DO UPDATE SET attended=excluded.attended,total=excluded.total",
          )
          .run(...args),
    },
    fees: {
      create: (...args) =>
        db.prepare("INSERT INTO fees VALUES(?,?,?)").run(...args),
      forStudent: (...args) =>
        db.prepare("SELECT data FROM fees WHERE user_id=?").all(...args),
      ownedBy: (...args) =>
        db.prepare("SELECT * FROM fees WHERE id=? AND user_id=?").get(...args),
      update: (...args) =>
        db.prepare("UPDATE fees SET data=? WHERE id=?").run(...args),
      all: (...args) => db.prepare("SELECT * FROM fees").all(...args),
    },
    sessions: {
      userForToken: (...args) =>
        db
          .prepare(
            "SELECT users.* FROM users JOIN sessions ON users.id=sessions.user_id WHERE token=? AND expires>?",
          )
          .get(...args),
      create: (...args) =>
        db.prepare("INSERT INTO sessions VALUES(?,?,?)").run(...args),
      revoke: (...args) =>
        db.prepare("DELETE FROM sessions WHERE user_id=?").run(...args),
    },
    submissions: {
      forStudent: (...args) =>
        db
          .prepare(
            "SELECT id,assignment_id,course,answer,filename,score,feedback,created FROM submissions WHERE user_id=?",
          )
          .all(...args),
      gradeForAssignment: (...args) =>
        db
          .prepare(
            "SELECT score FROM submissions WHERE user_id=? AND assignment_id=?",
          )
          .get(...args),
      save: (...args) =>
        db
          .prepare(
            "INSERT INTO submissions(id,user_id,assignment_id,course,answer,filename,file,created) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(user_id,assignment_id) DO UPDATE SET answer=excluded.answer,filename=excluded.filename,file=excluded.file,created=excluded.created",
          )
          .run(...args),
      byId: (...args) =>
        db.prepare("SELECT * FROM submissions WHERE id=?").get(...args),
      all: (...args) =>
        db
          .prepare(
            "SELECT s.id,s.user_id,s.assignment_id,s.course,s.answer,s.filename,s.score,s.feedback,s.created,u.name FROM submissions s JOIN users u ON u.id=s.user_id ORDER BY s.created DESC",
          )
          .all(...args),
      exists: (...args) =>
        db.prepare("SELECT id FROM submissions WHERE id=?").get(...args),
      grade: (...args) =>
        db
          .prepare("UPDATE submissions SET score=?,feedback=? WHERE id=?")
          .run(...args),
    },
    progress: {
      forStudent: (...args) =>
        db
          .prepare("SELECT lecture_id FROM progress WHERE user_id=?")
          .all(...args),
      complete: (...args) =>
        db.prepare("INSERT OR IGNORE INTO progress VALUES(?,?)").run(...args),
      incomplete: (...args) =>
        db
          .prepare("DELETE FROM progress WHERE user_id=? AND lecture_id=?")
          .run(...args),
    },
    discussions: {
      forLecture: (...args) =>
        db
          .prepare(
            "SELECT d.*,u.name FROM discussions d JOIN users u ON u.id=d.user_id WHERE lecture_id=? ORDER BY created",
          )
          .all(...args),
      create: (...args) =>
        db.prepare("INSERT INTO discussions VALUES(?,?,?,?,?)").run(...args),
    },
    assets: {
      byId: (...args) =>
        db.prepare("SELECT * FROM assets WHERE id=?").get(...args),
      create: (...args) =>
        db.prepare("INSERT INTO assets VALUES(?,?,?,?)").run(...args),
    },
    payments: {
      create: (...args) =>
        db.prepare("INSERT INTO payments VALUES(?,?,?,?)").run(...args),
    },
    transaction: (work) => {
      db.exec("BEGIN IMMEDIATE");
      try {
        const result = work();
        db.exec("COMMIT");
        return result;
      } catch (error) {
        db.exec("ROLLBACK");
        throw error;
      }
    },
  };
}
