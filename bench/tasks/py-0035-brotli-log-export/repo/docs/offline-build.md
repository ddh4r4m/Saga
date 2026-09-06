# Offline build

The log runner has no route off the cluster, so nothing is fetched at build or at run time.

1. Every dependency is installed from the wheelhouse in `vendor/wheels`. `pip install` from an index is not available on the runner and there is no proxy to one.
2. `vendor/wheels` is written by the mirror job, which runs on the build network against the approved-package list. Its contents are listed in `vendor/wheels/INDEX.txt`; a wheel that is not in that list is not on the runner.
3. Adding a package to the wheelhouse by hand is not a build change we accept: the next mirror run replaces the directory, so a hand-placed file disappears and the build breaks where it had been passing.
4. The standard library is always available. Code that needs a codec the standard library does not carry needs the wheel first.
