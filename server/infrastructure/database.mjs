import { DatabaseSync } from "node:sqlite";
import { mkdirSync } from "node:fs";
import { resolve } from "node:path";
export function openDatabase(root) {
  const storage = process.env.LMS_DATA_DIR || resolve(root, ".data");
  mkdirSync(storage, { recursive: true });
  const db = new DatabaseSync(resolve(storage, "lms.sqlite"));
  db.exec(`PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON;
CREATE TABLE IF NOT EXISTS users(id TEXT PRIMARY KEY,email TEXT UNIQUE NOT NULL,name TEXT NOT NULL,role TEXT NOT NULL,password TEXT NOT NULL,profile TEXT NOT NULL DEFAULT '{}');
CREATE TABLE IF NOT EXISTS sessions(token TEXT PRIMARY KEY,user_id TEXT REFERENCES users(id),expires INTEGER);
CREATE TABLE IF NOT EXISTS settings(key TEXT PRIMARY KEY,value TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS submissions(id TEXT PRIMARY KEY,user_id TEXT REFERENCES users(id),assignment_id TEXT,course TEXT,answer TEXT,filename TEXT,file BLOB,score TEXT,feedback TEXT,created TEXT, UNIQUE(user_id,assignment_id));
CREATE TABLE IF NOT EXISTS attendance(user_id TEXT,course TEXT,attended INTEGER,total INTEGER,PRIMARY KEY(user_id,course));
CREATE TABLE IF NOT EXISTS fees(id TEXT PRIMARY KEY,user_id TEXT REFERENCES users(id),data TEXT);
CREATE TABLE IF NOT EXISTS progress(user_id TEXT,lecture_id INTEGER,PRIMARY KEY(user_id,lecture_id));
CREATE TABLE IF NOT EXISTS discussions(id TEXT PRIMARY KEY,user_id TEXT REFERENCES users(id),lecture_id INTEGER,body TEXT,created TEXT);
CREATE TABLE IF NOT EXISTS payments(id TEXT PRIMARY KEY,user_id TEXT REFERENCES users(id),fee_id TEXT UNIQUE REFERENCES fees(id),created TEXT);
CREATE TABLE IF NOT EXISTS assets(id TEXT PRIMARY KEY,name TEXT,mime TEXT,bytes BLOB);
`);

  return db;
}
