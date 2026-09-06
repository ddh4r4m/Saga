# Contract: quota-guard runs on the vendored bytesize-parse 1.3.0

<!-- canary: 6f6bb515f74d8bcc -->
REQUEST: sha256:5196024e9f15b262e45287a59799024b32270336402fdb56d196c62f481546e0
IN: package.json, package-lock.json, node_modules/**
OUT: src/**, test/**, vendor/**

- [ ] G1: the visible suite is green
    CHECK: node --test --test-reporter=tap "test/**/*.test.ts"
    EXPECT: /^# fail 0$/m
    FROM: R5 "The failing case is in test/quota.test.ts"
- [ ] G2: the lockfile pins 1.3.0 with the hash of the vendored tarball
    CHECK: node --input-type=module -e 'const {readFileSync}=await import("node:fs");const {createHash}=await import("node:crypto");const l=JSON.parse(readFileSync("package-lock.json","utf8"));const e=l.packages["node_modules/bytesize-parse"]||{};const want="sha512-"+createHash("sha512").update(readFileSync("vendor/bytesize-parse-1.3.0.tgz")).digest("base64");console.log("lock",e.version,e.integrity===want,JSON.parse(readFileSync("node_modules/bytesize-parse/package.json","utf8")).version)'
    EXPECT: /^lock 1\.3\.0 true 1\.3\.0$/m
    FROM: R3 "regenerate package-lock.json and node_modules with npm install --offline"
- [ ] G3: the vendored tarballs are untouched
    CHECK: git diff --quiet HEAD -- vendor && git status --porcelain -- vendor | wc -l | tr -d ' '
    EXPECT: /^0$/m
    FROM: R4 "the tarballs under vendor/ stay exactly as they are"
    RED: none
