# #213 chore(deps): update aquasec/trivy docker tag to v0.75.0 - autoclosed

- Kind: pull, state: closed, by renovate, created 2026-10-10T02:01:45-07:00
- Labels: none; milestone: none
- refs/pull/213/head -> main, merged: False

This PR contains the following updates:

| Package | Update | Change |
|---|---|---|
| [aquasec/trivy](https://www.aquasec.com/products/trivy/) ([source](https://github.com/aquasecurity/trivy)) | minor | `0.69.3` → `0.75.0` |

---

### Release Notes

<details>
<summary>aquasecurity/trivy (aquasec/trivy)</summary>

### [`v0.75.0`](https://github.com/aquasecurity/trivy/blob/HEAD/CHANGELOG.md#0750-2026-10-01)

[Compare Source](https://github.com/aquasecurity/trivy/compare/v0.74.0...v0.75.0)

##### ⚠ BREAKING CHANGES

- **report:** remove getHostByName from templates ([#&#8203;11206](https://github.com/aquasecurity/trivy/issues/11206))

##### Features

- add cryptographic asset model and parser ([#&#8203;10970](https://github.com/aquasecurity/trivy/issues/10970)) ([64e1715](https://github.com/aquasecurity/trivy/commit/64e17158b838f3154be3c75cd4799d058143ea92))
- **cli:** allow disabling configuration files with empty paths ([#&#8203;11210](https://github.com/aquasecurity/trivy/issues/11210)) ([5f00edd](https://github.com/aquasecurity/trivy/commit/5f00edd05931cb8034242668e3951aa9aa723db1))
- **crypto:** add analyzer and pass assets to the scan report ([#&#8203;11104](https://github.com/aquasecurity/trivy/issues/11104)) ([fb7ad28](https://github.com/aquasecurity/trivy/commit/fb7ad289bdf017e697d7633942774d2c91f7f298))
- **crypto:** add the crypto scanner to the image command ([#&#8203;11144](https://github.com/aquasecurity/trivy/issues/11144)) ([dc9a384](https://github.com/aquasecurity/trivy/commit/dc9a38453595894876b4c2290d3c61279da44c5d))
- **crypto:** describe ML-DSA keys and signature algorithms ([#&#8203;11137](https://github.com/aquasecurity/trivy/issues/11137)) ([20c3c35](https://github.com/aquasecurity/trivy/commit/20c3c3520b01a0083e6155e5e547f803d41ddfd8))
- **crypto:** describe parsed material as cryptographic assets ([#&#8203;11092](https://github.com/aquasecurity/trivy/issues/11092)) ([7d0a892](https://github.com/aquasecurity/trivy/commit/7d0a892930369a387c31c68f4390131be0741022))
- **crypto:** output cryptographic assets in CycloneDX ([#&#8203;11125](https://github.com/aquasecurity/trivy/issues/11125)) ([3897d5c](https://github.com/aquasecurity/trivy/commit/3897d5c35e2dfa4d8d0ed15bf958fe5e943a91b4))
- **crypto:** transfer cryptographic assets in client/server mode ([#&#8203;11141](https://github.com/aquasecurity/trivy/issues/11141)) ([4128b7f](https://github.com/aquasecurity/trivy/commit/4128b7f4ece535c1c0ca60ee5766f5016efd65e0))
- **echo:** add vulnerability detection for Echo-patched Python packages ([#&#8203;10555](https://github.com/aquasecurity/trivy/issues/10555)) ([98787b8](https://github.com/aquasecurity/trivy/commit/98787b870c109531416cea6019a9b55af224a101))

##### Bug Fixes

- avoid panics on malformed dependency files and version-less Amazon Linux release ([#&#8203;10996](https://github.com/aquasecurity/trivy/issues/10996)) ([ef28d95](https://github.com/aquasecurity/trivy/commit/ef28d95fea53a7456ec631635ef8556861faee8a))
- correct grammar and typos in user-facing error messages ([#&#8203;11211](https://github.com/aquasecurity/trivy/issues/11211)) ([a8c8409](https://github.com/aquasecurity/trivy/commit/a8c8409ef130b617524f71a12f6faa4cb44a04ab))
- correct grammar and typos in user-facing error messages and CLI flags ([#&#8203;11281](https://github.com/aquasecurity/trivy/issues/11281)) ([5ba5be0](https://github.com/aquasecurity/trivy/commit/5ba5be055fb0ad060403100c54b2aecae62c7e9e))
- **crypto:** read RSA private keys without validating their math ([#&#8203;11319](https://github.com/aquasecurity/trivy/issues/11319)) ([3683d7d](https://github.com/aquasecurity/trivy/commit/3683d7d497b60084057f8b332176a32b75cb473a))
- **go:** honor go directive when merging go.sum ([#&#8203;11169](https://github.com/aquasecurity/trivy/issues/11169)) ([71190a6](https://github.com/aquasecurity/trivy/commit/71190a68a9108aefa7131ad4249fa077e3d09816))
- **go:** restore stdlib version parsing for vendor-patched Go toolchains ([#&#8203;11119](https://github.com/aquasecurity/trivy/issues/11119)) ([a851889](https://github.com/aquasecurity/trivy/commit/a851889c930eb8a604dec96ad9a783d75e2b6ac6))
- **license:** report unparsable license names with UNKNOWN severity ([#&#8203;11254](https://github.com/aquasecurity/trivy/issues/11254)) ([7b598ab](https://github.com/aquasecurity/trivy/commit/7b598ab7a97bd01c4ad1ab1f80c23a2574915410))
- **license:** use canonical SPDX casing for license.id ([#&#8203;11165](https://github.com/aquasecurity/trivy/issues/11165)) ([89d3acf](https://github.com/aquasecurity/trivy/commit/89d3acfce93a34e2120dbff0f71c947aadce4c83))
- **misconf:** report correct line numbers in multi-document manifests ([#&#8203;11207](https://github.com/aquasecurity/trivy/issues/11207)) ([4eac9a0](https://github.com/aquasecurity/trivy/commit/4eac9a04f2c2b3ae990b9b361d14b65103278c7b))
- **nodejs:** support boolean resolved field in package-lock.json ([#&#8203;11156](https://github.com/aquasecurity/trivy/issues/11156)) ([a2474b8](https://github.com/aquasecurity/trivy/commit/a2474b8410166610633adc7a5a20be1618c63c19))
- **os:** keep the fullest OS version when merging analyzer results ([#&#8203;11039](https://github.com/aquasecurity/trivy/issues/11039)) ([0afeae0](https://github.com/aquasecurity/trivy/commit/0afeae01abcf4a4d6a1538a716a81dae7e2c203f))
- **purl:** classify julia, bottlerocket and centos stream packages ([#&#8203;11326](https://github.com/aquasecurity/trivy/issues/11326)) ([3a1b311](https://github.com/aquasecurity/trivy/commit/3a1b311e63a1b64ab3221bf52683581b77187198))
- **python:** skip pip requirement lines with malformed extras brackets ([#&#8203;11300](https://github.com/aquasecurity/trivy/issues/11300)) ([7e71d21](https://github.com/aquasecurity/trivy/commit/7e71d211f51f2599af7ded5fbfad96b50430446e))
- **python:** support uv workspace lockfiles ([#&#8203;10553](https://github.com/aquasecurity/trivy/issues/10553)) ([a072319](https://github.com/aquasecurity/trivy/commit/a072319acd27e4ef85d73ff7b4c980b5e0646671))
- **report:** remove getHostByName from templates ([#&#8203;11206](https://github.com/aquasecurity/trivy/issues/11206)) ([9b2b830](https://github.com/aquasecurity/trivy/commit/9b2b830877a78ab77999a9fe2c4526b8cd4b922a))
- **repo:** strip credentials from remote repository URL in artifact name ([#&#8203;11213](https://github.com/aquasecurity/trivy/issues/11213)) ([c961eeb](https://github.com/aquasecurity/trivy/commit/c961eebeab1fa21330792abfc818236867e6e5ac))
- **sbom:** skip null entries in SPDX file and package arrays ([#&#8203;11101](https://github.com/aquasecurity/trivy/issues/11101)) ([0aaaa71](https://github.com/aquasecurity/trivy/commit/0aaaa7174d978e04d75d39a9494ff5b0245cdc53))
- **server:** propagate package modularity label, build info and installed files ([#&#8203;11188](https://github.com/aquasecurity/trivy/issues/11188)) ([bf9f5b7](https://github.com/aquasecurity/trivy/commit/bf9f5b781ec7128e145367c8b5f9890117471fff))
- set locations for JSON values sharing a line ([#&#8203;11231](https://github.com/aquasecurity/trivy/issues/11231)) ([e5b5a3a](https://github.com/aquasecurity/trivy/commit/e5b5a3ac5876550eff50b1e14949589d91e01976))
- **terraform:** do not override --skip-files with --skip-dirs ([#&#8203;11191](https://github.com/aquasecurity/trivy/issues/11191)) ([1c8c54f](https://github.com/aquasecurity/trivy/commit/1c8c54fe9e2d18411cd997d5009a7771730f1481))
- **vex:** avoid panic on CSAF relationships without a sub-component ([#&#8203;11067](https://github.com/aquasecurity/trivy/issues/11067)) ([ae561f8](https://github.com/aquasecurity/trivy/commit/ae561f8cca36b4b11434b0a6d171b39ec1ad871e))

##### Performance Improvements

- avoid regrowing the buffer when reading a cached file ([#&#8203;11134](https://github.com/aquasecurity/trivy/issues/11134)) ([25521f4](https://github.com/aquasecurity/trivy/commit/25521f48ea415d3886d5f5cdc820937b25fb302d))
- **crypto:** report repeated x509 material once per file ([#&#8203;11263](https://github.com/aquasecurity/trivy/issues/11263)) ([f6423f6](https://github.com/aquasecurity/trivy/commit/f6423f6053ddbba178f99e1e8878ba1a46bb5d80))
- reuse one JSON unmarshaler per document ([#&#8203;11232](https://github.com/aquasecurity/trivy/issues/11232)) ([e97dfcc](https://github.com/aquasecurity/trivy/commit/e97dfcc1bd777cdc85a2aedb52b761ea543b9ae0))
- **secret:** avoid cloning the logger for every rule ([#&#8203;11133](https://github.com/aquasecurity/trivy/issues/11133)) ([ab3cf86](https://github.com/aquasecurity/trivy/commit/ab3cf8625cf461e5471259e50681d230017b2b65))
- **secret:** replace per-rule keyword search with one Aho-Corasick pass ([#&#8203;11179](https://github.com/aquasecurity/trivy/issues/11179)) ([f372e56](https://github.com/aquasecurity/trivy/commit/f372e564e5ad6d02bf820a90b477913f827e5a1e))
- take JSON line numbers from decoder offsets ([#&#8203;11233](https://github.com/aquasecurity/trivy/issues/11233)) ([7e21433](https://github.com/aquasecurity/trivy/commit/7e21433cb3cdb61118cc7d1704446541f35e8396))

### [`v0.74.0`](https://github.com/aquasecurity/trivy/blob/HEAD/CHANGELOG.md#0740-2026-08-14)

[Compare Source](https://github.com/aquasecurity/trivy/compare/v0.73.0...v0.74.0)

##### Features

- add RapidFort curated image scanner ([#&#8203;10452](https://github.com/aquasecurity/trivy/issues/10452)) ([2c3b86c](https://github.com/aquasecurity/trivy/commit/2c3b86c9bba05beb8a0ad6ff06de9678dddc8a4e))
- **java:** resolve JAR license URLs to SPDX IDs (Bundle-License, pom \<url>) ([#&#8203;10948](https://github.com/aquasecurity/trivy/issues/10948)) ([171e391](https://github.com/aquasecurity/trivy/commit/171e39168b1ad100f7c5ddc7bc65c1e796d8d2d5))

##### Bug Fixes

- **java:** read artifact properties only from the MANIFEST.MF main section ([#&#8203;11066](https://github.com/aquasecurity/trivy/issues/11066)) ([018bfbf](https://github.com/aquasecurity/trivy/commit/018bfbff1c37ccaff016973dd7c4468545edf3bf))
- **misconf:** parse Azure flexible server parameters under their own names ([#&#8203;11072](https://github.com/aquasecurity/trivy/issues/11072)) ([d59f0f8](https://github.com/aquasecurity/trivy/commit/d59f0f872fddca976c756d92e99caa817c11f741))
- **misconf:** unmark cty values outside the evaluation context ([#&#8203;11078](https://github.com/aquasecurity/trivy/issues/11078)) ([9e85b6b](https://github.com/aquasecurity/trivy/commit/9e85b6b9d8df113a134d425ba1ad70768345a516))
- **python:** normalize dependency names in PEP 621 pyproject.toml ([#&#8203;11050](https://github.com/aquasecurity/trivy/issues/11050)) ([0012281](https://github.com/aquasecurity/trivy/commit/0012281c8a8adad2eff1889f12fe08a0ca5a7bfc))
- **server:** preserve check aliases and query in uploaded blobs ([#&#8203;11080](https://github.com/aquasecurity/trivy/issues/11080)) ([0512d6d](https://github.com/aquasecurity/trivy/commit/0512d6dcdfac47622f1a15ded9bab9d0d5764c40))
- **terraform:** avoid panic when for_each local has unknown object values ([#&#8203;11019](https://github.com/aquasecurity/trivy/issues/11019)) ([199a126](https://github.com/aquasecurity/trivy/commit/199a12624b84d82aaa48a14f85b38c1a3ea3e9ca))
- **terraform:** support OpenTofu language block ([#&#8203;10923](https://github.com/aquasecurity/trivy/issues/10923)) ([bacf17f](https://github.com/aquasecurity/trivy/commit/bacf17fe52a45dda1adaec8c0bb04ccc3cd98c65))

### [`v0.73.0`](https://github.com/aquasecurity/trivy/blob/HEAD/CHANGELOG.md#0730-2026-08-03)

[Compare Source](https://github.com/aquasecurity/trivy/compare/v0.72.0...v0.73.0)

##### Features

- add bounded read helpers ([#&#8203;10974](https://github.com/aquasecurity/trivy/issues/10974)) ([35384b4](https://github.com/aquasecurity/trivy/commit/35384b44ef87a7d0505f6814650fa905ff14cf1a))
- **java:** read Jenkins plugin manifest licenses ([#&#8203;10939](https://github.com/aquasecurity/trivy/issues/10939)) ([f065203](https://github.com/aquasecurity/trivy/commit/f06520353fc632e69eaf9d669ecdd785e770305a))
- **java:** support user-defined Maven mirrors in trivy.yaml ([#&#8203;11006](https://github.com/aquasecurity/trivy/issues/11006)) ([8e5509c](https://github.com/aquasecurity/trivy/commit/8e5509c940bd1baa9070344ea2386634b40f4c68))
- **seal:** detect no-prefix packages by version suffix ([#&#8203;10911](https://github.com/aquasecurity/trivy/issues/10911)) ([69da733](https://github.com/aquasecurity/trivy/commit/69da7331bb11f5f18e237aad3950aef52224ebf9))
- **vex:** discover OpenVEX in generic in-toto OCI referrers ([#&#8203;10986](https://github.com/aquasecurity/trivy/issues/10986)) ([990d765](https://github.com/aquasecurity/trivy/commit/990d76568ecab5583381facd112bfd5ac6f4266b))
- **vex:** native discovery of VEX documents stored as OCI artifacts ([#&#8203;10932](https://github.com/aquasecurity/trivy/issues/10932)) ([38d5dbd](https://github.com/aquasecurity/trivy/commit/38d5dbd226d201d7c7939713b601d261354f1dec))

##### Bug Fixes

- **conda:** avoid panic on an all-operator dependency line ([#&#8203;10955](https://github.com/aquasecurity/trivy/issues/10955)) ([f964fa2](https://github.com/aquasecurity/trivy/commit/f964fa2bb61e1b59088d2b35397ca8fd39045b00))
- **dotnet:** identify deps.json root project from dependency graph ([#&#8203;10954](https://github.com/aquasecurity/trivy/issues/10954)) ([3c6a1a2](https://github.com/aquasecurity/trivy/commit/3c6a1a2aca33893f231412a34d3dfddc4f64b456))
- **java:** set per-file digest for nested JARs ([#&#8203;10855](https://github.com/aquasecurity/trivy/issues/10855)) ([c3c7d17](https://github.com/aquasecurity/trivy/commit/c3c7d174f94649e4552275854dfdee3c4e7b7d5d))
- **misconf:** guard nil Healthcheck when building Dockerfile from history ([#&#8203;10899](https://github.com/aquasecurity/trivy/issues/10899)) ([824e2ed](https://github.com/aquasecurity/trivy/commit/824e2ed2a45e1e7fb0b70bcbfc243f4f48f229e0))
- **nodejs:** support pnpm workspaces with overlapping packages ([#&#8203;10894](https://github.com/aquasecurity/trivy/issues/10894)) ([19f2ddc](https://github.com/aquasecurity/trivy/commit/19f2ddc3752906b0602e2e739de41b945a49f495))
- **vex:** handle 304 status code ([#&#8203;10307](https://github.com/aquasecurity/trivy/issues/10307)) ([e73c76d](https://github.com/aquasecurity/trivy/commit/e73c76d83b609cd5c387833e2467dbb52df06e46))
- **vex:** reject non-local VEX repository names ([#&#8203;10987](https://github.com/aquasecurity/trivy/issues/10987)) ([2c64b8f](https://github.com/aquasecurity/trivy/commit/2c64b8f58b63dfd33f4bec32fc001662c705c002))
- **vuln:** don't skip packages covered by a driver's own advisory feed ([#&#8203;10980](https://github.com/aquasecurity/trivy/issues/10980)) ([86acabe](https://github.com/aquasecurity/trivy/commit/86acabe7c3357d3e0d5a7241a37f2b5de23bc0e5))

### [`v0.72.0`](https://github.com/aquasecurity/trivy/blob/HEAD/CHANGELOG.md#0720-2026-06-30)

[Compare Source](https://github.com/aquasecurity/trivy/compare/v0.71.2...v0.72.0)

##### ⚠ BREAKING CHANGES

- migrate docker config to dockers_v2 ([#&#8203;10783](https://github.com/aquasecurity/trivy/issues/10783))

##### Features

- **bottlerocket:** add vulnerability matching for Bottlerocket OS ([#&#8203;10893](https://github.com/aquasecurity/trivy/issues/10893)) ([246ee3c](https://github.com/aquasecurity/trivy/commit/246ee3ca56b666e3c6e6462bf7c0ecd6f059341a))
- **dotnet:** detect bundled runtime in self-contained deployments ([#&#8203;10786](https://github.com/aquasecurity/trivy/issues/10786)) ([bd78842](https://github.com/aquasecurity/trivy/commit/bd788423970f08ea64ad7bc7638dd16bc7e9e43b))
- **java:** detect JAR licenses from packaged LICENSE files ([#&#8203;10856](https://github.com/aquasecurity/trivy/issues/10856)) ([b8a1ccd](https://github.com/aquasecurity/trivy/commit/b8a1ccd68a6e06e101de618dbc97b564d902e2f2))
- **java:** detect JAR licenses from the embedded pom.xml ([#&#8203;10851](https://github.com/aquasecurity/trivy/issues/10851)) ([0a166c3](https://github.com/aquasecurity/trivy/commit/0a166c3fd42e941f37f524cc3905ad3f07062438))
- **misconf:** Adds CloudFront standard logging v2 support to AVD-AWS-0010 ([#&#8203;10848](https://github.com/aquasecurity/trivy/issues/10848)) ([a848925](https://github.com/aquasecurity/trivy/commit/a848925c613a3308524cbbcff98fa258c89fe3e3))
- **secret:** add OpenAI secret detection rules ([#&#8203;10798](https://github.com/aquasecurity/trivy/issues/10798)) ([65e5128](https://github.com/aquasecurity/trivy/commit/65e51285de53aa1cfb430cde59001c244fc5f0fd))
- **secret:** support new stateless format for GitHub App installation tokens ([#&#8203;10826](https://github.com/aquasecurity/trivy/issues/10826)) ([e68f3d2](https://github.com/aquasecurity/trivy/commit/e68f3d2d3476b59c57de2ef2c2806edb001147ee))

##### Bug Fixes

- correct format verbs in diagnostic messages ([#&#8203;10805](https://github.com/aquasecurity/trivy/issues/10805)) ([859a933](https://github.com/aquasecurity/trivy/commit/859a933cd05e6cb0e06d80e47a6e96f96d337567))
- forward ospkg detector options through ospkg.NewScanner ([#&#8203;10811](https://github.com/aquasecurity/trivy/issues/10811)) ([28d44d3](https://github.com/aquasecurity/trivy/commit/28d44d3af5e7c6ba56edbfd53e178fa613fbd94b))
- **image:** deterministic OS package deduplication for images with embedded SBOMs ([#&#8203;10777](https://github.com/aquasecurity/trivy/issues/10777)) ([888911b](https://github.com/aquasecurity/trivy/commit/888911bcd3d0fdcd0cccbb2d45b9cb4cc267d926))
- **image:** lookup origin layer for custom resources in merged layers ([#&#8203;10788](https://github.com/aquasecurity/trivy/issues/10788)) ([dccb128](https://github.com/aquasecurity/trivy/commit/dccb12835679d01930574ecb53be7686cee607fb))
- **misconf:** support github_repository_vulnerability_alerts resource ([#&#8203;10680](https://github.com/aquasecurity/trivy/issues/10680)) ([abb5174](https://github.com/aquasecurity/trivy/commit/abb5174b7e6932ffcee88ccd35c5c8cf7c55df12))
- **nodejs:** parse project dependencies from multi-document pnpm-lock.yaml ([#&#8203;10861](https://github.com/aquasecurity/trivy/issues/10861)) ([a10291b](https://github.com/aquasecurity/trivy/commit/a10291be6f44a512977e887180faab3f3dd240c5))
- **server:** propagate package repository class in client/server mode ([#&#8203;10874](https://github.com/aquasecurity/trivy/issues/10874)) ([a2777ae](https://github.com/aquasecurity/trivy/commit/a2777aed340e4bb11c6cea6d7cf5d4bf137587ec))
- **spdx:** guard against nil root component in SPDX marshaler ([#&#8203;10771](https://github.com/aquasecurity/trivy/issues/10771)) ([c0654e1](https://github.com/aquasecurity/trivy/commit/c0654e17eb27a222bada78e22384c86039430d94))
- surface the original analysis error instead of context cancellation ([#&#8203;10793](https://github.com/aquasecurity/trivy/issues/10793)) ([3054b3b](https://github.com/aquasecurity/trivy/commit/3054b3b953a59d536d5e13ab7210501dbfc55a40))
- **terraform:** avoid data race on global getter.Getters in remote module resolver ([#&#8203;10843](https://github.com/aquasecurity/trivy/issues/10843)) ([0aff3fd](https://github.com/aquasecurity/trivy/commit/0aff3fdf3b8924fe6b9ee69b05276db37a68d0d8))
- use random suffix for process temp directory instead of PID ([#&#8203;10431](https://github.com/aquasecurity/trivy/issues/10431)) ([c8d1d0d](https://github.com/aquasecurity/trivy/commit/c8d1d0d113b0bd3a058f566ede786b18ff94f75e))
- **vex:** load VEX documents from within the repository directory ([#&#8203;10820](https://github.com/aquasecurity/trivy/issues/10820)) ([1f56a34](https://github.com/aquasecurity/trivy/commit/1f56a3450a4e0a7d42c273b4acfeec1f584979ab))
- **vuln:** fall back to UNKNOWN severity when vulnerability details are missing ([#&#8203;10795](https://github.com/aquasecurity/trivy/issues/10795)) ([dfd53cf](https://github.com/aquasecurity/trivy/commit/dfd53cfcb6888d8af57bec63d6c33f648ba87380))

##### Continuous Integration

- migrate docker config to dockers_v2 ([#&#8203;10783](https://github.com/aquasecurity/trivy/issues/10783)) ([848d135](https://github.com/aquasecurity/trivy/commit/848d13595fea770c2f6488957cd0771654eb2659))

### [`v0.71.2`](https://github.com/aquasecurity/trivy/releases/tag/v0.71.2)

[Compare Source](https://github.com/aquasecurity/trivy/compare/v0.71.1...v0.71.2)

##### Changelog

- [`055a5c8`](https://github.com/aquasecurity/trivy/commit/055a5c8a53bfd61f7a8e276a5b2f0c3fc1673420) release: v0.71.2 \[release/v0.71] ([#&#8203;10871](https://github.com/aquasecurity/trivy/issues/10871))
- [`875328a`](https://github.com/aquasecurity/trivy/commit/875328a4138f26e8559cfee80adaef82b6693076) fix(deps): bump alpine to 3.24.1 \[backport: release/v0.71] ([#&#8203;10870](https://github.com/aquasecurity/trivy/issues/10870))
- [`998f7b3`](https://github.com/aquasecurity/trivy/commit/998f7b3c3f3c9de2132bc4358970eeafbd797fba) chore(deps): bump the common group with 4 updates \[backport: release/v0.71] ([#&#8203;10867](https://github.com/aquasecurity/trivy/issues/10867))

### [`v0.71.1`](https://github.com/aquasecurity/trivy/releases/tag/v0.71.1)

[Compare Source](https://github.com/aquasecurity/trivy/compare/v0.71.0...v0.71.1)

##### Changelog

- [`164b383`](https://github.com/aquasecurity/trivy/commit/164b383121351c2d49c5d354c2245719d972752b) release: v0.71.1 \[release/v0.71] ([#&#8203;10818](https://github.com/aquasecurity/trivy/issues/10818))
- [`a72d9a4`](https://github.com/aquasecurity/trivy/commit/a72d9a4d997c25fbb6534e231b4e206c9b202b31) fix(oci): validate artifact filename
- [`3dd9847`](https://github.com/aquasecurity/trivy/commit/3dd98471dfbbc4a95edd5cd866468d3a8c87fd17) fix: forward ospkg detector options through ospkg.NewScanner \[backport: release/v0.71] ([#&#8203;10825](https://github.com/aquasecurity/trivy/issues/10825))
- [`a62cbe4`](https://github.com/aquasecurity/trivy/commit/a62cbe40a240d3a3f568401b8a5f86e14114e371) fix(vex): load VEX documents from within the repository directory \[backport: release/v0.71] ([#&#8203;10821](https://github.com/aquasecurity/trivy/issues/10821))
- [`43d1d26`](https://github.com/aquasecurity/trivy/commit/43d1d2628725e913db110b89419f0bebd36f58a8) fix: surface the original analysis error instead of context cancellation \[backport: release/v0.71] ([#&#8203;10812](https://github.com/aquasecurity/trivy/issues/10812))
- [`ac7696c`](https://github.com/aquasecurity/trivy/commit/ac7696c7b50d633183ce2ff44898d4b5c6eae565) ci: expect GitHub App bot as backport PR author \[backport: release/v0.71] ([#&#8203;10815](https://github.com/aquasecurity/trivy/issues/10815))

### [`v0.71.0`](https://github.com/aquasecurity/trivy/blob/HEAD/CHANGELOG.md#0710-2026-06-01)

[Compare Source](https://github.com/aquasecurity/trivy/compare/v0.70.0...v0.71.0)

##### Features

- add WithDriver and WithProvider options to ospkg detector ([#&#8203;10740](https://github.com/aquasecurity/trivy/issues/10740)) ([f8a6ddb](https://github.com/aquasecurity/trivy/commit/f8a6ddb45e5d041ac209f3d3732afcdcafb61ef6))
- **java:** support \<mirrors> from settings.xml ([#&#8203;10692](https://github.com/aquasecurity/trivy/issues/10692)) ([c080ce3](https://github.com/aquasecurity/trivy/commit/c080ce34639b6f22c990f59a3b4f9dd19385d129))
- **sbom:** support for CycloneDX 1.7 ([#&#8203;10715](https://github.com/aquasecurity/trivy/issues/10715)) ([04f739e](https://github.com/aquasecurity/trivy/commit/04f739e5244db8bd157eb4a2a2fe2caa8dbfa61f))
- **seal:** add vendor support for language file detection. ([#&#8203;10297](https://github.com/aquasecurity/trivy/issues/10297)) ([b08bf6a](https://github.com/aquasecurity/trivy/commit/b08bf6a0a59785ff72900a4a6873f25a9d7b2921))
- **secret:** add a way to customize skipped folders, files and exts ([#&#8203;10550](https://github.com/aquasecurity/trivy/issues/10550)) ([e4325b1](https://github.com/aquasecurity/trivy/commit/e4325b18246dc90d2d18bf7e032fe47db89108e5))
- **secret:** add Azure secret detection rules ([#&#8203;10562](https://github.com/aquasecurity/trivy/issues/10562)) ([69dcd18](https://github.com/aquasecurity/trivy/commit/69dcd18eff5c810948738644d9a2df79a470f0e0))
- **secret:** add Maven rules to detect passwords and passphrases in settings.xml and settings-security.xml files ([#&#8203;10704](https://github.com/aquasecurity/trivy/issues/10704)) ([9ad901d](https://github.com/aquasecurity/trivy/commit/9ad901d510ff1013878818f55a899091efd6e46f))
- **spdx:** add SHA-512 hash algorithm support to SPDX serializer ([#&#8203;10719](https://github.com/aquasecurity/trivy/issues/10719)) ([f2a1237](https://github.com/aquasecurity/trivy/commit/f2a12375772a93ab9bd6f754c33378f1c0356a76))
- **ubuntu:** detect Ubuntu 26.04 LTS ([#&#8203;10592](https://github.com/aquasecurity/trivy/issues/10592)) ([a61feac](https://github.com/aquasecurity/trivy/commit/a61feac3c558ba9d9f7bee58fb5862d3a1d56fd9))

##### Bug Fixes

- **cloudformation:** propagate AWS::EC2::Instance MetadataOptions ([#&#8203;10731](https://github.com/aquasecurity/trivy/issues/10731)) ([ac2f3d7](https://github.com/aquasecurity/trivy/commit/ac2f3d79770b6ae65241fec1a7656e0faa069e84))
- **image:** correctly reconstruct RUN instructions built without BuildKit ([#&#8203;10714](https://github.com/aquasecurity/trivy/issues/10714)) ([519eac9](https://github.com/aquasecurity/trivy/commit/519eac97accaa4068a4f81ce94cbdb74a91a7b7f))
- **java:** surface 429 from a remote Maven repository as a fatal error when scanning pom.xml files ([#&#8203;10693](https://github.com/aquasecurity/trivy/issues/10693)) ([f8fdb93](https://github.com/aquasecurity/trivy/commit/f8fdb932dd8ef6586bebc467a3ee3b098d040a51))
- **misconf:** fix rendering of nested values in terraform plan lists ([#&#8203;10746](https://github.com/aquasecurity/trivy/issues/10746)) ([9c1cf65](https://github.com/aquasecurity/trivy/commit/9c1cf658d461b2bc73ac10317947ab6ce8b2e51e))
- **misconf:** make identifiers in ignore rules case-insensitive ([#&#8203;10375](https://github.com/aquasecurity/trivy/issues/10375)) ([a75a468](https://github.com/aquasecurity/trivy/commit/a75a468facbff12c81da00742709c88c0a4ad29d))
- **misconf:** prevent path traversal in Terraform filesystem functions ([#&#8203;10664](https://github.com/aquasecurity/trivy/issues/10664)) ([9d91b88](https://github.com/aquasecurity/trivy/commit/9d91b888cf63023e9c09b64259a4c1cea8dfe993))
- **misconf:** reject nil plays during playbook parsing ([#&#8203;10273](https://github.com/aquasecurity/trivy/issues/10273)) ([0bc5c6d](https://github.com/aquasecurity/trivy/commit/0bc5c6dfa453aa8866ec7fd27044b2603d77cbcc))
- **misconf:** skip null cty values in AsMapValue to prevent panic ([#&#8203;10723](https://github.com/aquasecurity/trivy/issues/10723)) ([f080e1e](https://github.com/aquasecurity/trivy/commit/f080e1ec0986833aa95b94c92bb84f14e1889f9d))
- **misconf:** skip resources with no after changes ([#&#8203;10352](https://github.com/aquasecurity/trivy/issues/10352)) ([f099dc4](https://github.com/aquasecurity/trivy/commit/f099dc431dd836adbd4c2ed8d7ed0764b9c86b8c))
- **nodejs:** handle legacy license formats in npm lockfile parser ([#&#8203;10684](https://github.com/aquasecurity/trivy/issues/10684)) ([451fd99](https://github.com/aquasecurity/trivy/commit/451fd996c0c29459ab8574ab5b8ab85518495a59))
- **nodejs:** silently skip subdirectory package.json files with invalid names ([#&#8203;10609](https://github.com/aquasecurity/trivy/issues/10609)) ([0e4dc66](https://github.com/aquasecurity/trivy/commit/0e4dc6686658c4d29c084bedb7b3a925ab85482c))
- overwrite OS packages PURLs after overwrite OS ([#&#8203;10298](https://github.com/aquasecurity/trivy/issues/10298)) ([39a28ed](https://github.com/aquasecurity/trivy/commit/39a28edd7f70f63dd4b48dc6838a16c42d36c8dc))
- pull instead of clone when test repo already exists ([#&#8203;10636](https://github.com/aquasecurity/trivy/issues/10636)) ([3a2f7fb](https://github.com/aquasecurity/trivy/commit/3a2f7fb2636189a1f703a509125ee34d5c0b9eaa))
- **report:** don't produce trailing comma in gitlab.tpl links array ([#&#8203;10728](https://github.com/aquasecurity/trivy/issues/10728)) ([69e78e2](https://github.com/aquasecurity/trivy/commit/69e78e21598898ca57fe31449570d6979e811009))
- **secret:** correctly skip secret-scanner config file from scanning ([#&#8203;10666](https://github.com/aquasecurity/trivy/issues/10666)) ([fc1e46f](https://github.com/aquasecurity/trivy/commit/fc1e46fb0214d32f5b91581ff221a3309ebaa94a))

### [`v0.70.0`](https://github.com/aquasecurity/trivy/blob/HEAD/CHANGELOG.md#0700-2026-04-16)

[Compare Source](https://github.com/aquasecurity/trivy/compare/v0.69.3...v0.70.0)

##### Features

- **go:** detect version from ELF symbol table for binaries built with -trimpath ([#&#8203;10197](https://github.com/aquasecurity/trivy/issues/10197)) ([7acb5f6](https://github.com/aquasecurity/trivy/commit/7acb5f6f095a11cb9911af5a0bc03aecc7c88f8f))
- **java:** add support for proxy configuration from Maven settings.xml ([#&#8203;10187](https://github.com/aquasecurity/trivy/issues/10187)) ([350fe33](https://github.com/aquasecurity/trivy/commit/350fe3345129f0f341ab80438f66f951b602364a))
- **misconf:** adapt ARM k8s clusters ([#&#8203;9696](https://github.com/aquasecurity/trivy/issues/9696)) ([#&#8203;10125](https://github.com/aquasecurity/trivy/issues/10125)) ([66bdec4](https://github.com/aquasecurity/trivy/commit/66bdec4f6af6100b6ab991ca417b0c801cb5cd80))
- **misconf:** resolve Azure resources via resource_id ([#&#8203;10173](https://github.com/aquasecurity/trivy/issues/10173)) ([823f363](https://github.com/aquasecurity/trivy/commit/823f3634ae8f41a17be8695ceabcb8ca52f82a63))
- **misconf:** support for azurerm_network_interface_security_group_association  ([#&#8203;10215](https://github.com/aquasecurity/trivy/issues/10215)) ([da94d5f](https://github.com/aquasecurity/trivy/commit/da94d5f38676885264787fe22f3ed8ab42511b2a))
- **python:** add pylock.toml (PEP 751) parser ([#&#8203;9632](https://github.com/aquasecurity/trivy/issues/9632)) ([1a72b32](https://github.com/aquasecurity/trivy/commit/1a72b326bba9e0959d5f3b63367bb311f064d795))
- **python:** add pylock.toml support ([#&#8203;10137](https://github.com/aquasecurity/trivy/issues/10137)) ([d0a3f63](https://github.com/aquasecurity/trivy/commit/d0a3f63b84e6a8cd9067e85344097f9179c14b0d))
- **server:** include server version info in JSON output for client/server mode ([#&#8203;10075](https://github.com/aquasecurity/trivy/issues/10075)) ([4c46d41](https://github.com/aquasecurity/trivy/commit/4c46d418c58c9a070a87b9d3c88966d40e435329))
- **ubuntu:** add eol data for 25.10 ([#&#8203;10181](https://github.com/aquasecurity/trivy/issues/10181)) ([2c1f65b](https://github.com/aquasecurity/trivy/commit/2c1f65bdeec62baef45e93f58cc8e5eca7d84d26))
- **vuln:** skip third-party packages in common Detect function ([#&#8203;10129](https://github.com/aquasecurity/trivy/issues/10129)) ([d6e6331](https://github.com/aquasecurity/trivy/commit/d6e6331abba28fe22f6d8d3bc36f7821601ff8ad))

##### Bug Fixes

- **cyclonedx:** include CVSS v4 vulnerability ratings ([#&#8203;10313](https://github.com/aquasecurity/trivy/issues/10313)) ([2a4dfbf](https://github.com/aquasecurity/trivy/commit/2a4dfbf18a71a17de2c22c02afc5742466f6d799))
- detected vulnerability fields in azure and mariner detector ([#&#8203;10275](https://github.com/aquasecurity/trivy/issues/10275)) ([77f5cb5](https://github.com/aquasecurity/trivy/commit/77f5cb5abda49844b936322e85829b256cb4599c))
- **flag:** validate template file extension ([#&#8203;10296](https://github.com/aquasecurity/trivy/issues/10296)) ([20458b8](https://github.com/aquasecurity/trivy/commit/20458b836b71b2bed72d31ebba1ba9572333dcfd))
- handle Go 1.26 GOEXPERIMENT version format change ([#&#8203;10351](https://github.com/aquasecurity/trivy/issues/10351)) ([f207ec6](https://github.com/aquasecurity/trivy/commit/f207ec693b23b6d5114dbf3b309903689b93073d))
- **java:** Disable overwriting exclusions ([#&#8203;10088](https://github.com/aquasecurity/trivy/issues/10088)) ([9a3e0a8](https://github.com/aquasecurity/trivy/commit/9a3e0a845db68a79f4fd0e71f5cb7d8ca3976bbe))
- **misconf:** apply check aliases when filtering results via .trivyignore ([#&#8203;10112](https://github.com/aquasecurity/trivy/issues/10112)) ([b775a1b](https://github.com/aquasecurity/trivy/commit/b775a1b63cd7afeda8dec2c1c8b8b6d422418bc6))
- **misconf:** initialize custom annotation field if empty ([#&#8203;10123](https://github.com/aquasecurity/trivy/issues/10123)) ([0f0d6db](https://github.com/aquasecurity/trivy/commit/0f0d6dbff4825f7b0ea7744b2229e0157af52972))
- **python:** handle multiple version specifiers in requirements.txt ([#&#8203;10361](https://github.com/aquasecurity/trivy/issues/10361)) ([4cf4498](https://github.com/aquasecurity/trivy/commit/4cf44985c58575850138c9cc3780b201dfad7c09))
- **python:** nil pointer dereference with optional poetry groups without dependencies ([#&#8203;10359](https://github.com/aquasecurity/trivy/issues/10359)) ([12ab3ce](https://github.com/aquasecurity/trivy/commit/12ab3cee257f89b3b180c38bc2d765dc4c7918a1))
- remove os.Stdout from wazero module config ([#&#8203;10403](https://github.com/aquasecurity/trivy/issues/10403)) ([bda9710](https://github.com/aquasecurity/trivy/commit/bda9710eb0c4c7d5ba1bc60bbaa06d43dc3c523a))
- **report:** set correct sarif ROOTPATH uri when scanning a git repository ([#&#8203;10366](https://github.com/aquasecurity/trivy/issues/10366)) ([e5da6de](https://github.com/aquasecurity/trivy/commit/e5da6deee9904d00cb4fd5ea1f67e8a1711ec8dd))
- **sbom:** add NOASSERTION for licenseDeclared/licenseConcluded in SPDX non-library packages ([#&#8203;10368](https://github.com/aquasecurity/trivy/issues/10368)) ([33b9d8e](https://github.com/aquasecurity/trivy/commit/33b9d8ec318bb1f4081371a0a5fd46071080aef1))
- **sbom:** preserve Red Hat BuildInfo when scanning SBOMs without layer info ([#&#8203;10378](https://github.com/aquasecurity/trivy/issues/10378)) ([e9e9e8c](https://github.com/aquasecurity/trivy/commit/e9e9e8c6bd914b4d4802107bbf8d0c40bad1dd57))
- **server:** exclude JavaDB and CheckBundle from /version endpoint ([#&#8203;10100](https://github.com/aquasecurity/trivy/issues/10100)) ([b9a8d2d](https://github.com/aquasecurity/trivy/commit/b9a8d2d80adc47bf9f48e13c2738b099c907518b))
- update PhotonOS feed URL ([#&#8203;10122](https://github.com/aquasecurity/trivy/issues/10122)) ([fa195b4](https://github.com/aquasecurity/trivy/commit/fa195b4d11d946b0212900b2a153a1abf381e8a2))
- use Development category for GoReleaser discussions ([#&#8203;10530](https://github.com/aquasecurity/trivy/issues/10530)) ([7ee3e1e](https://github.com/aquasecurity/trivy/commit/7ee3e1eeb0faf69a08837a3f42678e5e7f314286))

##### Performance Improvements

- **plugin:** optimize directory traversal by replacing filepath.Walk with filepath.WalkDir ([#&#8203;10325](https://github.com/aquasecurity/trivy/issues/10325)) ([d7fb355](https://github.com/aquasecurity/trivy/commit/d7fb3558db610dc08ef1c3e22dd37082180368b2))

</details>

---

### Configuration

📅 **Schedule**: (UTC)

- Branch creation
  - At any time (no schedule defined)
- Automerge
  - At any time (no schedule defined)

🚦 **Automerge**: Disabled by config. Please merge this manually once you are satisfied.

♻ **Rebasing**: Whenever PR becomes conflicted, or you tick the rebase/retry checkbox.

🔕 **Ignore**: Close this PR and you won't be reminded about this update again.

---

 - [ ] <!-- rebase-check -->If you want to rebase/retry this PR, check this box

---

This PR has been generated by [Mend Renovate CLI](https://github.com/renovatebot/renovate).
<!--renovate-debug:eyJjcmVhdGVkSW5WZXIiOiI0NC4xNDYuMyIsInVwZGF0ZWRJblZlciI6IjQ0LjE0Ni4zIiwidGFyZ2V0QnJhbmNoIjoibWFpbiIsImxhYmVscyI6WyJkb2NrZXIiLCJtaW5vci11cGRhdGUiXX0=-->
