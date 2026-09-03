# feedimport

Nightly vendor price import. `importer.job.run(dir)` reads every vendor CSV listed in `dir/manifest.json`, parses each row, and reconciles the imported row count and total against the manifest the vendors send with their files. A reconcile mismatch fails the job; nothing is written downstream on failure.

`fixtures/nightly/` is a copy of one night's input (files and manifest as received). `logs/` keeps the job logs.

Tests: `python3 -m unittest discover -v -s tests -t .`
