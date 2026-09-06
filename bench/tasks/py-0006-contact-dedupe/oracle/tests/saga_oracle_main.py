# canary: b382a4c6249ef36a
"""Runner for the hidden oracle, executed from outside the workspace.

The workspace is appended to sys.path after the standard library, so a module
the workspace carries can never shadow a standard library module; the oracle
package itself is loaded from the runner tree, which the workspace cannot
reach."""
import os
import sys
import unittest

runner, workspace, target = sys.argv[1], sys.argv[2], sys.argv[3]
if target != "oracle":
    raise SystemExit("unknown runner target: " + target)
# Physical paths, so the workspace entry on sys.path matches the cwd the oracle
# sees and a hidden test that compares module filenames still recognises them.
runner, workspace = os.path.realpath(runner), os.path.realpath(workspace)
sys.path[:] = [p for p in sys.path if p not in ("", ".", runner, workspace)]
sys.path.insert(0, runner)
sys.path.append(workspace)
argv = ["saga-oracle", "discover", "-v", "--buffer", "-s", runner + "/saga_oracle_run", "-t", runner]
unittest.main(module=None, argv=argv, exit=True)
