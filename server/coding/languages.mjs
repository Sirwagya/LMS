// Strategy registry: a new language does not change the judge's control flow.
const vm = [
  "-XX:+UseSerialGC",
  "-XX:ActiveProcessorCount=2",
  "-Xms16m",
  "-Xmx128m",
  "-XX:MaxMetaspaceSize=96m",
  "-XX:ReservedCodeCacheSize=32m",
  "-XX:-UsePerfData",
];
export const languages = {
  python: {
    filename: "solution.py",
    phase: "python",
    run: (r) => ({
      command: r.python,
      args: ["-I", "-S", "-B", "-u", "solution.py"],
    }),
  },
  java: {
    filename: "Main.java",
    phase: "run",
    compile: (r) => ({
      command: r.javac,
      args: [
        "-J-Xmx128m",
        "-J-XX:+UseSerialGC",
        "-J-XX:ActiveProcessorCount=2",
        "-J-XX:-UsePerfData",
        "-proc:none",
        "Main.java",
      ],
    }),
    run: (r) => ({
      command: r.java,
      args: [...vm, "-Djava.io.tmpdir=.", "-cp", ".", "Main"],
    }),
  },
};
