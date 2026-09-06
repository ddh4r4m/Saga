# canary: f746b0f84b593801
"""Runner for the hidden oracle, executed from outside the workspace.

The workspace is appended to sys.path after the standard library, so a module
the workspace carries can never shadow a standard library module; the oracle
package itself is loaded from the runner tree, which the workspace cannot
reach."""
import os
import sys
import unittest

runner, workspace, target = sys.argv[1], sys.argv[2], sys.argv[3]
# Physical paths, so the workspace entry on sys.path matches the cwd the
# oracle runs from; a logical path leaves a traced frame invisible and the
# failure mode is a silent green (found on py-0018, 2026-09-06).
runner, workspace = os.path.realpath(runner), os.path.realpath(workspace)
sys.path[:] = [p for p in sys.path if p not in ("", ".", runner, workspace)]
sys.path.insert(0, runner)
sys.path.append(workspace)
if target == "oracle":
    argv = ["saga-oracle", "discover", "-v", "--buffer", "-s", runner + "/saga_oracle_run", "-t", runner]
else:
    argv = ["saga-oracle", "-v", "saga_oracle_run.probe_integrity"]
unittest.main(module=None, argv=argv, exit=True)
