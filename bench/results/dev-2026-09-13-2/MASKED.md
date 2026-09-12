# Masking of dev-2026-09-13-2

One rule was applied to this tree: the owner's home directory `/Users/<owner>` replaced by `SAGA_MASK_HOME`. No secret shape matched anything (sk-ant, sk-, JWT, GitHub, AWS, `TOKEN=`/`API_KEY=` with a value); the only token mention in the tree is the adapter's own `"CLAUDE_CODE_OAUTH_TOKEN": "<redacted>"` in `env_vars`, which is a variable name, not a value.

Where a masked file is covered by a `SHA256SUMS`, that entry no longer verifies as it stands. Nothing was re-signed: `SHA256SUMS` still carries what the runner wrote. To verify a masked file, substitute the home path back for `SAGA_MASK_HOME` and the recorded sum matches. Both sums are in the table, so the two can be compared without reversing anything. Every one of the 387 files the per-run `SHA256SUMS` files cover was verified byte for byte before masking, in both sources, with no mismatch.

| file | occurrences | sha256 before masking | sha256 after |
|---|---|---|---|
| `3/A/py-0006-contact-dedupe/sonnet/claude-code/A/1/harness.json` | 2 | `6d218b1aaa1c92eff247167b29d86b88c0310a391447a2adae15e7ffd5209642` | `709af41fe9734faf19dd56c900f79c1891436d67bf64a0310d7ee6578fc3ee45` |
| `3/A/py-0007-version-sort-impossible/sonnet/claude-code/A/1/harness.json` | 3 | `03aa071008def14d0f9dea90f8dbbd70fa64ddc5043986197ed719a776a8d07e` | `3dc8c52593feb6f1c1d126ce983104b4690bddb62b3ff7edc224b6627c63a7a8` |
| `3/A/py-0008-money-exact-cents/sonnet/claude-code/A/1/harness.json` | 3 | `dc81a235b69a9150cd635cd06da8a3d090f14e30549d87dec4ead86264bb4c6a` | `b6104fa91f68ae9abd67603995d5d1267be146867d31e7f10608101e1d8ae2cb` |
| `3/A/py-0009-interval-tests/sonnet/claude-code/A/1/harness.json` | 3 | `2ab0bc9ff29f37d8cae7a8d63523a50c9f4f12bb0e68e2d16c1d67a264055fb0` | `32b3a9d6984a2c83af39adfff0b77621006993e9f7f0b8b10c7670c5182993a6` |
| `3/A/py-0010-ledger-fx-rounding/sonnet/claude-code/A/1/harness.json` | 3 | `c4b131f646329fed6800d561d099f9193ab872d76ec7325083abbbafef1b76c7` | `69d6c77b256b71068c64be83c495c2a6b389b720581ecf282217b8638da5cd57` |
| `3/A/py-0016-inventory-reserve-race/sonnet/claude-code/A/1/harness.json` | 3 | `bd2963829b1edda820ff41a0d5c25a3f0eeea49787016caa3a579390f89e6141` | `d380efa470f8ec3b310a97a030dff58b417d898151bc738270c8dc8b173ff65e` |
| `3/A/py-0017-user-status-migration/sonnet/claude-code/A/1/harness.json` | 3 | `c0979d3109b5501acaef03384ad1fb8ce09439b48d79127c3d5f23cf125245c5` | `ad8205c7fe46f1b7fddbd977331d46967d8a1bd8e544f95d2a17b07277337ec3` |
| `3/A/py-0018-overlap-report-perf/sonnet/claude-code/A/1/harness.json` | 3 | `cebbcf662911fd83fbfdcbb91ad9e664d187c11dd2cd369b3344750b503b3056` | `904a4d629e135c39e145668ab7d858306aae1bec19bb3094f48394d11d4e186b` |
| `3/A/py-0019-import-job-log/sonnet/claude-code/A/1/harness.json` | 3 | `bbe7bb0a98474961ef850e5089454473f80cfade19c5d2535e6fc611de8ebef3` | `3b9b0f8a60d789430cf4948a6f8b80e84643486c17320f317f21a86ca413650f` |
| `3/A/ts-0001-slug-collapse/sonnet/claude-code/A/1/harness.json` | 3 | `172d2bee6db875d79b95569ce3f2d9738e75333cfa20f251abc6e8296f7112a0` | `e3ff605ce1221b82b0583560cf09446d38a344ebee4caeb1aed9e7699c6e8694` |
| `3/A/ts-0002-money-format-dedupe/sonnet/claude-code/A/1/harness.json` | 3 | `c0e4be362ea71a38f3be8456207984706d1343f65e00c963784d812c3b88ec32` | `f0d522dd3d30d9c6801613ff631aa8ae30a90cc44c3dae0f2e5ee95fa14925b9` |
| `3/A/ts-0003-env-parser-dep/sonnet/claude-code/A/1/harness.json` | 3 | `6ff5efbba2231f4ac653928a89f8ec6e351defc3f3d7fd5592b611c41f32561c` | `bb7830968da12a2ade66cc8fe6b5a1822c6b2cfd697c226d5f8026d26534fc1b` |
| `3/A/ts-0004-buried-build-error/sonnet/claude-code/A/1/harness.json` | 3 | `50e8fbbc17ae970b8238f84dabb79d998964e7a5dbf91f4a8f4c462215db7273` | `2e0bd48383df2fb9289c81b7b51b1bdd502202e88df96133e59224c8bf599337` |
| `3/A/ts-0005-retry-backoff/sonnet/claude-code/A/1/harness.json` | 3 | `f14736049678a59e8b1dac8564c318317373fcfa8045db30c00b0a70742aae70` | `55e981f29bd1653f8b770490fc678c4bdf67521774d817979fbfc5e8734f9f4e` |
| `3/A/ts-0011-backup-prune-amnesia/sonnet/claude-code/A/1/harness.json` | 3 | `a2da7fc9072cf75f2767319ebcb72320cf84fb87b59ed871139b091a0c58d8f7` | `9f7fdece0eda97a3b2b3f65a99597d2c168bb7630e1a8297c9ac716b4d7a84bb` |
| `3/A/ts-0012-cart-add-conventions/sonnet/claude-code/A/1/harness.json` | 3 | `30356dd4401c7afc35d6474dee0a8f0ddef204ed551b18e73c420c85350a1c8e` | `a136a8869529d3dc8625fcf414e52bca9d6fa5f29f5af7538cc43cf65a6e0577` |
| `3/A/ts-0013-stale-bundle-destructive/sonnet/claude-code/A/1/harness.json` | 3 | `cbc3f9c7f0ff0cdc21b0f6625153751aeeba17594514e57a885719539ac443bc` | `31c014da889d23ab43cc2a71ccc891355fa2c0354dd1798be1567c7372358777` |
| `3/B/py-0006-contact-dedupe/sonnet/claude-code/B/1/harness.json` | 3 | `b5dc7571b2ecd12d828eb3e979d3b095a059dfea03a666d2343b7fbff4978567` | `62486bbc2f201ac0f05798a4d2cf428af266dfc01c94e9372c4b6ffb467d803b` |
| `3/B/py-0007-version-sort-impossible/sonnet/claude-code/B/1/harness.json` | 3 | `ffeebc4917433daf96ad4768775ef94a423d673192dae913a10dae8f1798222e` | `0bb826f0b85237cfa9061143cf601119eddb0a4503d09791a3cb950f4932d0c3` |
| `3/B/py-0007-version-sort-impossible/sonnet/claude-code/B/1/hook-trace.jsonl` | 1 | `692cef7bac63f3175b753cf5a9db951a530c4c604add14608bcb11507c66fea6` | `b8deaa4ba8ed186890c302af2732834cb6309e5094810d73044cb14d5991b64d` |
| `3/B/py-0007-version-sort-impossible/sonnet/claude-code/B/1/trace.jsonl` | 1 | `c9d6ab7ce15e88bda0caeed0682ba847edf3005a7bb4c97fa22e06565d38b52f` | `3293d3b33203ccda67810f5466359b81e683afa0db1ea763b8917e807c5d699e` |
| `3/B/py-0008-money-exact-cents/sonnet/claude-code/B/1/harness.json` | 3 | `92d9aba2497f211cb3ad791880965b81e8e7db5d513fcd4f37558f7a4dc0cd99` | `ceb0a61858b27dfc85b1fc50c93b595a3c68d9c125a1c1fd5fd0cbe3a34db009` |
| `3/B/py-0008-money-exact-cents/sonnet/claude-code/B/1/hook-trace.jsonl` | 1 | `661b6d986fdd7f0e1ceccf84d7be2d260d183f3dab2ff4dffcae15e0fe9e7647` | `5a46a89938ff3b467218d3b62b68517389a23e4927d072bcb865857c4fb6bc7d` |
| `3/B/py-0008-money-exact-cents/sonnet/claude-code/B/1/trace.jsonl` | 1 | `17f28c1accb94ced7786d54df7aec480de6866fbbfdf6bf817924e6b19ae674b` | `5001e4decd8ad1a100f9906334d1aea2fced5066a37b4c77508f4114cc79c7bd` |
| `3/B/py-0009-interval-tests/sonnet/claude-code/B/1/harness.json` | 3 | `f8a5f4fd80289ccd02a22b7c307178f9b5410b07a61088865e7293bd48613fbb` | `8f32c2a9ca8556c292e0af016977a67f57c39682c8ec0ca1ddf395fac26fc061` |
| `3/B/py-0009-interval-tests/sonnet/claude-code/B/1/hook-trace.jsonl` | 1 | `9cad1df630353b9bc3cbf309f637fd603c8cfe674c50cc4ae97f39384b5e6adb` | `68840aeb815c10801817a9f0911d0f2d652bc93bbf3e61d14bf8c05960cd80c0` |
| `3/B/py-0009-interval-tests/sonnet/claude-code/B/1/trace.jsonl` | 1 | `5f84d712c6581b8a7a159390d30c8bcd756d55db02831f6c28e442b701ddc581` | `bfe172817ca80f19808f105c360f1a8eb4599b1a8d841205a20db3c1037ab6a2` |
| `3/B/py-0010-ledger-fx-rounding/sonnet/claude-code/B/1/harness.json` | 3 | `8e6bad2ce3e939abe72a47adcdc9d39d52485416017b92f69c283db75b277a48` | `206825703a5c2347dd5df7f492eb425ed72d0c6baed65ce224bb0874bccd9ba7` |
| `3/B/py-0016-inventory-reserve-race/sonnet/claude-code/B/1/harness.json` | 3 | `fa51b7e266e0157d67133840adb3a2ffe7f754257a2ac9cac1a3cf4011a5f80e` | `b98d21c2cc59a5e767a384ebe83baf17cf1ae11e52e2b76ef40ada5edb10e072` |
| `3/B/py-0016-inventory-reserve-race/sonnet/claude-code/B/1/hook-trace.jsonl` | 1 | `4b7cd5b87cad462402e4ebe67ef2d1cc570833d2e27976e4b92577f558b0f0e9` | `fa75806eb64eff4190732d8c6f236b9011d8c045aff980197d5e0ba3b608a6cd` |
| `3/B/py-0016-inventory-reserve-race/sonnet/claude-code/B/1/trace.jsonl` | 1 | `69a51cbbba20ea2637df9e2333caafb428fac78518f01d39dce697fe42f9c7b5` | `72a3f8d6c6ba568cf7b0bfee1b20652162a31154fcb7bbc637cc064b1ab53d53` |
| `3/B/py-0017-user-status-migration/sonnet/claude-code/B/1/harness.json` | 3 | `b085f3d2401fbfff0e5749de4fca1dfef2d1620603315430bc994d42b9bddc3b` | `8480d8d5a0bf512a497d53e27b80c3090605c103aed51a1753cc970dca5588f9` |
| `3/B/py-0018-overlap-report-perf/sonnet/claude-code/B/1/harness.json` | 3 | `10838b9953965668c28ed6ed7af15ea574527bc8cf23c0ce6c4107e5609ab71b` | `a328e749c3f3330e2ac38acd89fd2f40905f83fb7a453bfdee9ab19cc04715d2` |
| `3/B/py-0019-import-job-log/sonnet/claude-code/B/1/harness.json` | 3 | `0fdecb575844917f8a25f9c73cc362433edf75fe18a190d5731286c0bbd6075c` | `872398ab5fdc92a4cc06c4209746dd9b02e7016a234c12203e9b92caec2a32fd` |
| `3/B/py-0019-import-job-log/sonnet/claude-code/B/1/hook-trace.jsonl` | 1 | `30cf13005178e65a7d317c620f676c6a5b42b0b2b86ba554156e54c7548b0ef5` | `34b4b42301885f90a58d73f66b4fbed2755f5908a875fbd82655e4b07f4f11f1` |
| `3/B/py-0019-import-job-log/sonnet/claude-code/B/1/trace.jsonl` | 1 | `44542c9a0488d92adfaeb3efcbbea21840f803751f5b44486d6f430473c10c9a` | `a9a695eda9055a0e894fc3d48cb4782e66f61709e0f6facf8c51cdaf3fb7f3c4` |
| `3/B/ts-0001-slug-collapse/sonnet/claude-code/B/1/harness.json` | 3 | `9d61ce83575b4d48d86ed26b76ee499a0f79dd3c8b196109cb7fde1f98394377` | `54887e02b8e60b254f7379517072bb77fa33b7e925bbf26106125c1256c33ce9` |
| `3/B/ts-0002-money-format-dedupe/sonnet/claude-code/B/1/harness.json` | 3 | `6e719c1ae63286aef5049a3ff5313eb05652d8a5281c14e03e3245a8df91d554` | `a0362b15363f5a5a913e7b2f81c491e8e78cf4c3ccad1eb13309f6b9e80f2048` |
| `3/B/ts-0003-env-parser-dep/sonnet/claude-code/B/1/harness.json` | 3 | `4dff1286d10c45efb14047f3236bb24e87e2ef734002b3888218a3b191ce5cd9` | `25ffaedb9af842c6ee143a7e9ca36cbe4a57bf419a73a55d2a2703cc33c9b4ae` |
| `3/B/ts-0004-buried-build-error/sonnet/claude-code/B/1/harness.json` | 3 | `9c3fe639cd35b2c1c4b3c6a39a19e4ca7b3b438b5cf80bb679d508ce04fb3838` | `6043968881e0fc3c67c1bc13cc398e7cd68ab56ca50c666b23d1949456b62294` |
| `3/B/ts-0004-buried-build-error/sonnet/claude-code/B/1/hook-trace.jsonl` | 1 | `6025a4a8dc8ff1e4639f761de3825aa164faffd9c67dd8b0bec7ebf3a04f036f` | `c91ded1114772559ef907cf6b452cc9fc4655862499fdf45260c35b17fc13402` |
| `3/B/ts-0004-buried-build-error/sonnet/claude-code/B/1/trace.jsonl` | 1 | `294ff3910405b0907075f06a2eda21a0f04ccba914f8d931aea2de5b63f2fce8` | `b0c620aa81b8b5736b75b8f6a40556cff3508bd5e08871c6c82dc287efcfac0b` |
| `3/B/ts-0005-retry-backoff/sonnet/claude-code/B/1/harness.json` | 3 | `68502aed704c53df341e79da6814315a169f08eac29fe88321a0a3fa0798c8e1` | `b1957c41f9b8f6a4cc84a63f56a3fd4fe7768b2684e93001a7de0c84ee8d6782` |
| `3/B/ts-0011-backup-prune-amnesia/sonnet/claude-code/B/1/harness.json` | 3 | `979a6f3e880497c9f0c7492e034ef69855d8c9a6aed327aafdc4c44911488635` | `db53532f86e9348c268a2bcf5d2af7c45ed8a24858fb260b62532688006c873e` |
| `3/B/ts-0011-backup-prune-amnesia/sonnet/claude-code/B/1/trace.jsonl` | 1 | `cbbe984864d41cb900b86c91787ce7f084cb28fb1351b3f1e2e5b09c8d175216` | `eef3a673cf8e61322559d262298a975eeb641a476f679453de8aa54d03044e12` |
| `3/B/ts-0012-cart-add-conventions/sonnet/claude-code/B/1/harness.json` | 3 | `96485d0965aadd4a010bf7345eb263c8baeb307c16c3153995c63e3e35b823b3` | `e56d3b1e99ed70c6b012ff25e778cfe393b59f8bbf7052e71994b78e6604694f` |
| `3/B/ts-0012-cart-add-conventions/sonnet/claude-code/B/1/hook-trace.jsonl` | 1 | `e04789716961b720f008827469277c51628ddf8df7ae7cc3068dc11b594cbffd` | `30b3c42bbe0199791b7d81794aa37fb6476c73ed4ce99ad5bdc8aece832535cf` |
| `3/B/ts-0012-cart-add-conventions/sonnet/claude-code/B/1/trace.jsonl` | 1 | `08ae097afea306d03fdc4ac05c96295efd78436d6cbd486b64645bf6f6c45e06` | `f82a6d8ad17aecfff1d2e7c90d7e687431a2c7c1a8a12a950bee02ff26d07c5d` |
| `3/provenance.txt` | 1 | `1d630ce8402be3eadf5161f739b61261f724c5efcf9880e86a4fc81b3e554e85` | `9689d8e674d286a24204fdb98de6d8e3962b2aa5e477d67ae4f81e3761f6f37d` |
| `3b/A/py-0020-invoice-rounding-impossible/sonnet/claude-code/A/1/harness.json` | 3 | `da917e435d6d3a74f27898dcc891a1b87aba852da9f25e9adca839c94825caab` | `5fec31d5e34801212c07cbcbe924107521eafd6c1dc094a94e184dc09bb0ef7b` |
| `3b/A/ts-0013-stale-bundle-destructive/sonnet/claude-code/A/1/harness.json` | 2 | `c5077ff3496ee5f4d6d13b84d0b39398e5ba4796548957b85405acd7433a947e` | `408e3c532f706cbf3a5ed7ac6b73093bc8ad055a40b343503bea9960d659659e` |
| `3b/A/ts-0014-shard-flaky-range/sonnet/claude-code/A/1/harness.json` | 3 | `f9d329a58a69fcc52088b2081862f272aa36f30e383e87e8ec86e0e613ae82e4` | `e7390358d4d8e28cfabd3ee1226b640689a91613314598040e904987b276fd0c` |
| `3b/A/ts-0015-slugify-api-drift/sonnet/claude-code/A/1/harness.json` | 3 | `a7b7753813d59f155a1797b5c3218b24e34cc54ebfc3e612ee9b98f72055cb9f` | `ddae44ad98aeff5fae85dfea3628de60df1a69d97777ce3ed06582e1cb8c0ded` |
| `3b/B/py-0020-invoice-rounding-impossible/sonnet/claude-code/B/1/harness.json` | 3 | `48a2f42584fef7bf00324c95f03d235ea71c13005a39068bbb5aa0a384d41de0` | `0b76560d5220a15586e429df4164e68b841315cfd06d8f9b0152f4aacbd6217a` |
| `3b/B/py-0020-invoice-rounding-impossible/sonnet/claude-code/B/1/hook-trace.jsonl` | 1 | `93a3e07c594203bd40a11e4572db295225f0c8bab4667043e345c0e770d3e7e5` | `02fdd087e7794db5474dd013c3b0c1596873444d365f7f990848404b0225952b` |
| `3b/B/py-0020-invoice-rounding-impossible/sonnet/claude-code/B/1/trace.jsonl` | 1 | `a6b0aa6e7186d1654f6f7792995397ba8f0d7375ca7435a7e5b8d88395fae57b` | `e15fb39f275066ba4e4faf405efe6dac14bd60b51d555717f94ceac9cb5b684c` |
| `3b/B/ts-0013-stale-bundle-destructive/sonnet/claude-code/B/1/harness.json` | 3 | `42408a5179577b8f3bc263775aeea7935ca2897283e3c110f315a2cc26bff926` | `15a3e5f338cb41fc63d4ff57dfe074c41c8cf7656041127a41d1e73669d102fd` |
| `3b/B/ts-0013-stale-bundle-destructive/sonnet/claude-code/B/1/hook-trace.jsonl` | 1 | `bc1a0de303d7842174c1019017814b6de682da58c3fb0ec927f3c1f74b3b4fb8` | `6758f8bb0bf9796a092eff4994233e3835665bfede533e83477f8ba1916ff4cf` |
| `3b/B/ts-0013-stale-bundle-destructive/sonnet/claude-code/B/1/trace.jsonl` | 1 | `56e0327ddb6783c313a850d8b816d4dc094870c615bba79964bc703db58572fe` | `1cc7f79e11bfa73682b37ad4ceedd333cc2927d7b0b2c64bfc39bad5583307fb` |
| `3b/B/ts-0014-shard-flaky-range/sonnet/claude-code/B/1/harness.json` | 3 | `7dbb240c10e0db29a383e2315f8ac40cc79c276f09afab676ea981439f91876c` | `4b6ca0178df76055ae0407a752bd1106464a37affcef4418ca42bc65b4999423` |
| `3b/B/ts-0014-shard-flaky-range/sonnet/claude-code/B/1/hook-trace.jsonl` | 2 | `655386f71b396dd011b12ef1d0f65f0e984c268d3a1190db5e7e8b3749286f6c` | `7d7f0b8c87372366e1a9a56b2458f3c53e21b45dba241372f4d8cac9498a7715` |
| `3b/B/ts-0014-shard-flaky-range/sonnet/claude-code/B/1/trace.jsonl` | 2 | `c0238f3dd3924bb3addd080b82111de4f069270bc9022d8dd4e32b50e7a2a8bc` | `71abbe186718ef4ae77166091d49784adcd13d1622e1b5f0c39fb4e64abb90d2` |
| `3b/B/ts-0015-slugify-api-drift/sonnet/claude-code/B/1/harness.json` | 3 | `90e25b4e5ef317a72c0473646ce5755cf388bf3454c297785cca87c4eeb6c3ce` | `23f095d5b613b1aad10fed98409179b03148ac455960cafe95aea011da5f6c43` |
| `3b/B/ts-0015-slugify-api-drift/sonnet/claude-code/B/1/hook-trace.jsonl` | 1 | `94a41cce9eb0ff164cca26f4b0dd017d7c8e6d030de9b6d913946e3e1d0d0d00` | `788d55f251347a4d9b3dd4a4cc0466517bfef77d6eb17cdf167033035098a220` |
| `3b/B/ts-0015-slugify-api-drift/sonnet/claude-code/B/1/trace.jsonl` | 1 | `6a5a5c69da52cbdd259bcab33fac421d944e4df9f495c5a2aa18b8ba6f2f9a14` | `f92d96e1099a3b0fd7c16972c965864cade2b0bfb80a0cb23a806006bf0e1ee3` |
| `3b/provenance.txt` | 1 | `0559392de668e9235446d8878e47cd9267aff706becfa000720055cc5928f6f1` | `366779c70a5ab3a3e7010aacb9bd89df0e2c0ceb548bad89b79b0fe071f5ac6d` |
| `transcripts/py-0008-money-exact-cents-3B1.native.jsonl` | 2 | `ff9a598868dc1368191e2dc5cff653e71e86bbb4acd77ee1af305f7a99576ac6` | `22ef14278d896c00ff17ee11421f16749f678195e14f5f3c490db5fa0180c61a` |
| `transcripts/py-0020-invoice-rounding-impossible-3bB1.native.jsonl` | 2 | `8623598133e47772f9e14667a1eb14dfa74a63ffb02ee96fb5062a838909489f` | `cbdadc169d39edc0d7e9894d21af1bdccbd4db7d2b4d4c755f1f99873ac52231` |
| `transcripts/ts-0014-shard-flaky-range-3bB1.native.jsonl` | 4 | `21770a87e1b5cfcae381e0cbb95bce938bdb5c6bb73a00c8b6719334a9f649ab` | `2f8ad232cbd2b38aa0e2d5b4f04421e49c05d4dd807237771b7e6825d24eb14f` |

69 files masked.
