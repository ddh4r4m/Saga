# Masking of dev-2026-09-13-1

One rule was applied to this tree: the owner's home directory `/Users/<owner>` replaced by `SAGA_MASK_HOME`. No secret shape matched anything (sk-ant, sk-, JWT, GitHub, AWS, `TOKEN=`/`API_KEY=` with a value); the only token mention in the tree is the adapter's own `"CLAUDE_CODE_OAUTH_TOKEN": "<redacted>"` in `env_vars`, which is a variable name, not a value.

The masked files are all `harness.json`, which each run's `SHA256SUMS` covers, so those twenty entries no longer verify as they stand. Nothing was re-signed: `SHA256SUMS` still carries what the runner wrote. To verify a masked file, substitute the home path back for `SAGA_MASK_HOME` and the recorded sum matches. The sum before masking is in the table below as well, so the two can be compared without reversing anything.

| file | occurrences | sha256 before masking (as SHA256SUMS records it) | sha256 after |
|---|---|---|---|
| `A/py-0006-contact-dedupe/sonnet/claude-code/A/1/harness.json` | 2 | `da4b4f8c32fe93d444ab91e43bc3163a9e197cec96cccd7496832655603851aa` | `30e30de7fb65f7bd23106a7fd945610dc2dcb39dfb39e933eddc22cec4784346` |
| `A/py-0007-version-sort-impossible/sonnet/claude-code/A/1/harness.json` | 3 | `255e299cd1e0eda3fcd9670634276bed98f25076056bee5332673d1e585c6942` | `d78b098e43b271a07557c51034fffa81a518b006cc5a62a03b06fba6746ff47f` |
| `A/py-0008-money-exact-cents/sonnet/claude-code/A/1/harness.json` | 3 | `56fc252dcdeef017c0ddd62da1f7799ecc2692c3d0c067a3497e8d73d2e2bec8` | `cfe913b47d7928b0199a002642dee9d8d98c6dbf94c8f213978df17cbaa9990a` |
| `A/py-0009-interval-tests/sonnet/claude-code/A/1/harness.json` | 3 | `3612c3b9ecff9235ef241012e18876b7ad0c523191a30d3d367aa8e62f7eb042` | `035f69dcdaa79c107be49005361082b0d59ed2aa6eabf9d69333ab3e446cbc7c` |
| `A/py-0010-ledger-fx-rounding/sonnet/claude-code/A/1/harness.json` | 3 | `0711cb39719e927ac64e89871490d8fe817924f39e921a9aa4b741eb13420a9c` | `42fbe96474cbad3672bbd915b5d7901f357c93def76beb9bceeed60ae3edf47f` |
| `A/py-0016-inventory-reserve-race/sonnet/claude-code/A/1/harness.json` | 3 | `0e179035dcadd4324bf86d6d150c045da1716f6b2f1d7946ecd93ded195d8d90` | `e128d3db5f3a2935d5ddaf88af4db15ea1219152fdad1d22e5062eea61250be4` |
| `A/py-0017-user-status-migration/sonnet/claude-code/A/1/harness.json` | 3 | `09025f31c6a15facbe31ad2d66ad070984acc613da9442b37c527f50c6a61a10` | `2585aedaa09e0e081d73dbff445d52103d27a7b844c851c11497ed94b5b0454c` |
| `A/py-0018-overlap-report-perf/sonnet/claude-code/A/1/harness.json` | 3 | `c421ca0386b088182460488725016bb52578284d40f7c62adb89dfa2ff56aff7` | `3d0c5ca4bccb0e3a7ca5d1fdc2aac0f91dfaf3d4816dc0083c8f3e2ed638dd99` |
| `A/py-0019-import-job-log/sonnet/claude-code/A/1/harness.json` | 3 | `604ae8697d5268f815064359de8f08c9a18c6285e41ef61251d24d53da11796d` | `feab285fe81c611e51cac9f82ea0a78917b460b402d76c1d14b4ed99e342d45d` |
| `A/py-0020-invoice-rounding-impossible/sonnet/claude-code/A/1/harness.json` | 3 | `2e2bb127df468508ead86648a638aeceb35b91f47af827f7f69cf3c2ba47a750` | `bc2a80ad648245a9c914ab1be82b3a8e2e786ee4930d4d04b8ef0562a844c344` |
| `A/ts-0001-slug-collapse/sonnet/claude-code/A/1/harness.json` | 3 | `d81c96cb14efe32519eb8d598c32d94f800767cef3970f8e77ca3307e0b72aaf` | `29ce390c3c76a2ea52c347cb61a78c493e97f25c7b68e38410e04693ba41c274` |
| `A/ts-0002-money-format-dedupe/sonnet/claude-code/A/1/harness.json` | 3 | `fbb052672ca263e7953ad4a8723a54c2b20e0817c219b056ebe022817b3d5ad1` | `46137808053896991297d1e20b0b93ecaf100c5f526016916b6916f6e2c6f064` |
| `A/ts-0003-env-parser-dep/sonnet/claude-code/A/1/harness.json` | 3 | `03870bc9c18517334cf7d6ebfabf76f669721d5279c93582a6882c0fa5a8844d` | `69086de4792550215c65cae6304eb976df0c9fd910a97528f999a4d1d1680c65` |
| `A/ts-0004-buried-build-error/sonnet/claude-code/A/1/harness.json` | 3 | `42e54b0be579e7470fd3c5fe9babba1e89c2285f8ca262e477c7138c56fc23e2` | `7b6b669ba43421b4de46a26d649a528e230450596d34b5676a7feb3d5f6d6e5a` |
| `A/ts-0005-retry-backoff/sonnet/claude-code/A/1/harness.json` | 3 | `206fc439ca3b3d44972a1938b3a02270556c1e011a1a381e6dff2a4a2ef40a45` | `f6466583c13e7cd8fcd933f75d5996d60a8f02aea84241ddae31e730507147f4` |
| `A/ts-0011-backup-prune-amnesia/sonnet/claude-code/A/1/harness.json` | 3 | `9644ac87e4b811838bcac93766825c1673d62d70169c5d358a95fda12727153e` | `fb0a2a6a32b43c84513830171fae76ed493dfe0020f0aee9372214b8fc9ff66a` |
| `A/ts-0012-cart-add-conventions/sonnet/claude-code/A/1/harness.json` | 3 | `37a0d0704a097d0ee2ad27b20b294c3369c83ba0601b4a66bde03af19f402f99` | `45557b7bfc106810c29f980b070d3a771cf978947d1611225d480f10c89e9d63` |
| `A/ts-0013-stale-bundle-destructive/sonnet/claude-code/A/1/harness.json` | 3 | `71b60fd5e70ec892e7e9d58087e4be5964ac80e9929bf78f4e09ba47eb125176` | `5a864ed3f8645e08241c727dbcaa985f02e94167732d5e1f51f3a018f3836801` |
| `A/ts-0014-shard-flaky-range/sonnet/claude-code/A/1/harness.json` | 3 | `63593f19d109ba659b8a1d6272ff2dc3851b320ae67e2e5175d343b2277432fe` | `b992fc5c28bbe08798fe821a62810a2fa702e96632db7ff28c00c0d8acff5ccf` |
| `A/ts-0015-slugify-api-drift/sonnet/claude-code/A/1/harness.json` | 3 | `876e240c16c9a4114db3cc625ab13999b865a7f2480e8448052c647314ee53bc` | `366c11f02566e3e4b3335726309425dfbc83ce262b2f51870e44f1af38f418f3` |
| `provenance.txt` | 1 | `435a6c9931176171ad9510b646f5338d71e718dc554a820472d50ebe601e3e0f` | `4ec930555ebffd1b0f95e51b1a27cec7039230d55a83a50f38dc7961e100d96c` |

21 files masked. The paths that carried the home directory are the composed `bench_path`, the `SAGA_APPROVAL_DIR` value and the harness binary's own path, all in `harness.json`, plus the `path:` line of `provenance.txt`.
