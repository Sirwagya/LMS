export function createCatalog({ settings }, seed) {
  const academicKey = "academic";
  let academic;
  const saved = settings.get(academicKey);
  if (saved) academic = JSON.parse(saved.value);
  else {
    academic = {
      semesterSubjects: seed.semesterSubjects,
      lecturesBySubject: seed.lecturesBySubject,
      contestSummary: seed.contestSummary,
      recentContests: seed.recentContests,
      demoWeek: seed.demoWeek,
      drawerNotices: seed.drawerNotices,
    };
    // Bundled sample notes are actual downloads; recordings need a staff upload.
    for (const lectures of Object.values(academic.lecturesBySubject))
      for (const l of lectures) {
        l.recordingUrl = null;
        l.resources = [
          {
            title: `${l.title} — notes.txt`,
            type: "Text notes",
            url: `/api/notes/${l.id}`,
          },
        ];
        for (const a of l.assignments) {
          a.status = "Pending";
          delete a.score;
          delete a.url;
        }
      }
    settings.create(academicKey, JSON.stringify(academic));
  }
  function persist() {
    settings.update(JSON.stringify(academic), academicKey);
  }

  const course = (code) =>
    Object.values(academic.semesterSubjects)
      .flat()
      .find((s) => s.code === code);
  const lecture = (id) =>
    Object.values(academic.lecturesBySubject)
      .flat()
      .find((l) => l.id === Number(id));
  const assignment = (id) =>
    Object.entries(academic.lecturesBySubject)
      .flatMap(([c, ls]) =>
        ls.flatMap((l) => l.assignments.map((a) => ({ ...a, course: c }))),
      )
      .find((a) => String(a.id) === String(id));

  return { academic, persist, course, lecture, assignment };
}
