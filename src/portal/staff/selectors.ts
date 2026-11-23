import { semesterSubjects } from "../../data/lmsData";
export const courses = () => Object.values(semesterSubjects).flat();
