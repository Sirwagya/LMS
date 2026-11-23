"""Resource limits are installed before replacing this process with a runtime."""
import os
import resource
import sys

phase, executable, *arguments = sys.argv[1:]
cpu = 8 if phase == "compile" else 2
resource.setrlimit(resource.RLIMIT_CPU, (cpu, cpu + 1))
resource.setrlimit(resource.RLIMIT_FSIZE, (1024 * 1024, 1024 * 1024))
resource.setrlimit(resource.RLIMIT_NOFILE, (64, 64))
resource.setrlimit(resource.RLIMIT_CORE, (0, 0))
os.execv(executable, [executable, *arguments])
