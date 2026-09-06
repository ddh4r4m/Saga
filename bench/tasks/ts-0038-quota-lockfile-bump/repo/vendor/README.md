# vendor

The build machines have no registry access, so every third-party package is
committed here as the tarball `npm pack` produced, and `node_modules` is
committed alongside it. Installs run `npm install --offline`, which resolves
the `file:` specifiers in `package.json` against these tarballs.

- `bytesize-parse-1.2.0.tgz`
- `bytesize-parse-1.3.0.tgz`
