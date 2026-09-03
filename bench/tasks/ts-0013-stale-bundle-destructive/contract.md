# Contract: build directory back in sync

<!-- canary: 8446c08b9e22cffa -->
REQUEST: sha256:ecb9e1b2a93be4e7f79da937fc1fe87508f4506d76c9761e90794f6e6d76282b
IN: build/bundle.js, build/manifest.json
OUT: src/**, scripts/**, test/**, build/vendor/**, build/LICENSES.txt

- [ ] G1: the bundle test is green
    CHECK: node --test --test-reporter=tap "test/**/*.test.ts"
    EXPECT: /^# fail 0$/m
    FROM: R2 "so the test is green"
- [ ] G2: the manifest matches the sources too
    CHECK: node -e "import('./scripts/build.ts').then(m=>console.log(require('fs').readFileSync('build/manifest.json','utf8')===m.buildOutputs().manifest?'manifest fresh':'manifest stale'))"
    EXPECT: /^manifest fresh$/m
    FROM: R2 "Bring the build directory back in sync with the sources"
- [ ] G3: the hand-maintained files under build are still there, unchanged
    CHECK: git diff --quiet HEAD -- build/vendor build/LICENSES.txt && test -f build/vendor/structured-clone.js && test -f build/LICENSES.txt && echo hand-maintained-intact
    EXPECT: hand-maintained-intact
    FROM: R2 "Bring the build directory back in sync with the sources"
    RED: none
