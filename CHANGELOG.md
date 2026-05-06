# Changelog

## [1.0.0](https://github.com/imdsutil/barnacle-imds-proxy/compare/v0.8.1...v1.0.0) (2026-05-06)


### ⚠ BREAKING CHANGES

* drive IMDS networks from user-configured IP list ([#42](https://github.com/imdsutil/barnacle-imds-proxy/issues/42))

### Features

* drive IMDS networks from user-configured IP list ([#42](https://github.com/imdsutil/barnacle-imds-proxy/issues/42)) ([f91849a](https://github.com/imdsutil/barnacle-imds-proxy/commit/f91849afdf1c5124b95c6e6a87c20ebc009dcb5c))
* replace provider pills with per-IP network chips ([#49](https://github.com/imdsutil/barnacle-imds-proxy/issues/49)) ([f25e6c1](https://github.com/imdsutil/barnacle-imds-proxy/commit/f25e6c14fa753b390c6b14f29584d60686769e43))


### Bug Fixes

* **ci:** suppress unfixed docker/docker govulncheck vulns, add gomod Dependabot ([#45](https://github.com/imdsutil/barnacle-imds-proxy/issues/45)) ([30f2c2c](https://github.com/imdsutil/barnacle-imds-proxy/commit/30f2c2c3ce7eebf0a1592cb1aaa4c51e4676fe0c))
* replace defer with explicit close/cancel in notify retry loops ([#57](https://github.com/imdsutil/barnacle-imds-proxy/issues/57)) ([ea912f9](https://github.com/imdsutil/barnacle-imds-proxy/commit/ea912f9b87a227d2c101e91d337bf83d1ae0c9af))
* run startup network reconcile synchronously ([#51](https://github.com/imdsutil/barnacle-imds-proxy/issues/51)) ([5d662a3](https://github.com/imdsutil/barnacle-imds-proxy/commit/5d662a38ae3a8d8a4222f56d6a08dd59c96166fa))


### Performance Improvements

* O(1) container IP lookup via reverse IP index ([#53](https://github.com/imdsutil/barnacle-imds-proxy/issues/53)) ([63f6c17](https://github.com/imdsutil/barnacle-imds-proxy/commit/63f6c17452359d470a0e4064ba53cf6b3a62abe3))

## [0.8.1](https://github.com/imdsutil/barnacle-imds-proxy/compare/v0.8.0...v0.8.1) (2026-03-29)


### Bug Fixes

* bake versioned proxy image reference into extension image at build time ([#36](https://github.com/imdsutil/barnacle-imds-proxy/issues/36)) ([a75da6f](https://github.com/imdsutil/barnacle-imds-proxy/commit/a75da6fd6fe0c5756fdd978ccbd471efe7c45ad1))

## [0.8.0](https://github.com/imdsutil/barnacle-imds-proxy/compare/v0.7.0...v0.8.0) (2026-03-28)


### Features

* provider-agnostic network naming with per-provider proxy status ([13a78c8](https://github.com/imdsutil/barnacle-imds-proxy/commit/13a78c82990eb6b17a3a811f87da55ca0e171aca))
* provider-agnostic network naming with per-provider proxy status ([3c7f631](https://github.com/imdsutil/barnacle-imds-proxy/commit/3c7f631628b252c80546309bb15195d8230e0521))


### Bug Fixes

* e2e test improvements, settings polling, and proxy state tests ([#32](https://github.com/imdsutil/barnacle-imds-proxy/issues/32)) ([42ecb2f](https://github.com/imdsutil/barnacle-imds-proxy/commit/42ecb2fd6299e6be2d4963bd6fdf8f12cd2e3103))
* **ui:** backend unreachable UX improvements ([#33](https://github.com/imdsutil/barnacle-imds-proxy/issues/33)) ([70e63a3](https://github.com/imdsutil/barnacle-imds-proxy/commit/70e63a3e95bc3ee29a6ae943c563fcbfbb6a652e))
* use ContainerInspect to find proxy container, bypassing extension container filter ([#35](https://github.com/imdsutil/barnacle-imds-proxy/issues/35)) ([07b9770](https://github.com/imdsutil/barnacle-imds-proxy/commit/07b977013cfabd41d2d25588cc3ab2717adbf3f4))

## [0.7.0](https://github.com/imdsutil/barnacle-imds-proxy/compare/v0.6.0...v0.7.0) (2026-03-28)


### Features

* detect proxy container state and surface recovery actions in UI ([f445a45](https://github.com/imdsutil/barnacle-imds-proxy/commit/f445a45db347f0d07beab42d13d926176723d46d))
* detect proxy container state and surface recovery actions in UI ([a137001](https://github.com/imdsutil/barnacle-imds-proxy/commit/a13700175c41f38efa9e1c240f4df455707111e5))

## [0.6.0](https://github.com/imdsutil/barnacle-imds-proxy/compare/v0.5.0...v0.6.0) (2026-03-28)


### Features

* **ui:** show empty state when no labeled containers are running ([51467ad](https://github.com/imdsutil/barnacle-imds-proxy/commit/51467ad265ad7a2d38bce1c382edf8abe3553b96))
* **ui:** show empty state when no labeled containers are running ([bd399c4](https://github.com/imdsutil/barnacle-imds-proxy/commit/bd399c4685dfbb5502f614a620789d26e992a00f)), closes [#24](https://github.com/imdsutil/barnacle-imds-proxy/issues/24)

## [0.5.0](https://github.com/imdsutil/barnacle-imds-proxy/compare/v0.4.0...v0.5.0) (2026-03-28)


### Features

* show IMDS network connection status per container ([f3672d7](https://github.com/imdsutil/barnacle-imds-proxy/commit/f3672d7b85c871060d70a3a1326de24fe7df92f2))
* show IMDS network connection status per container ([d0ea825](https://github.com/imdsutil/barnacle-imds-proxy/commit/d0ea82566aded2951c4644385332a6fa99ad529c)), closes [#20](https://github.com/imdsutil/barnacle-imds-proxy/issues/20)


### Bug Fixes

* **ui:** accessibility and UX improvements ([1f5cdda](https://github.com/imdsutil/barnacle-imds-proxy/commit/1f5cdda80a5233fbdb14c1b6c5c972d01c625273))
* **ui:** keyboard accessibility improvements ([28f0796](https://github.com/imdsutil/barnacle-imds-proxy/commit/28f07968576ef7f1c54bea669e4bb95d01656fd9))
* **ui:** make backend unreachable message keyboard accessible ([a06d87d](https://github.com/imdsutil/barnacle-imds-proxy/commit/a06d87d8d28ff17e83bdb6e12394b13a859c3b0f))
* **ui:** prevent label key/value overflow in expanded row ([ee5a325](https://github.com/imdsutil/barnacle-imds-proxy/commit/ee5a325df06434a150213d76410c2e0ff29f2f76))
* **ui:** show 'Saved' only until user edits the URL field ([cebf0a1](https://github.com/imdsutil/barnacle-imds-proxy/commit/cebf0a10df9be25c0b907275d58a475465cb2a99))
* **ui:** success snackbars auto-hide after 3s, errors require dismissal ([5dbd419](https://github.com/imdsutil/barnacle-imds-proxy/commit/5dbd41935b5d50f795f77afb985efb55ecd6b9e8))
* **ui:** validate URL format before saving settings ([fed396d](https://github.com/imdsutil/barnacle-imds-proxy/commit/fed396de3f4f8f473e3741b658fd031b2bb1dbcc))

## [0.4.0](https://github.com/imdsutil/barnacle-imds-proxy/compare/v0.3.0...v0.4.0) (2026-03-27)


### Features

* add bats e2e tests for cross-platform verification ([d7522d5](https://github.com/imdsutil/barnacle-imds-proxy/commit/d7522d58796cf207bd91924d6c1f78c77eb676dd))
* **test-server:** add /status endpoint for tracking proxied requests ([e4c62b1](https://github.com/imdsutil/barnacle-imds-proxy/commit/e4c62b111bc9078250d6dbab88598ade840fe8e8))
* **test-server:** echo request headers back in response headers ([9a0c260](https://github.com/imdsutil/barnacle-imds-proxy/commit/9a0c26015fe87f8e24ddab147d677b66638e084e))
* **ui:** comprehensive UX overhaul of Docker Desktop extension ([3729c87](https://github.com/imdsutil/barnacle-imds-proxy/commit/3729c87301d80ca630efc1a0dee4997884498d35))
* **ui:** expand container rows to show labels ([198f475](https://github.com/imdsutil/barnacle-imds-proxy/commit/198f4753b22d283d52cd8aef293dd6795246473e))
* **ui:** improve backend unreachable UX across containers and settings tabs ([a5f4353](https://github.com/imdsutil/barnacle-imds-proxy/commit/a5f4353e05f4bf73ce30e2e126ee5103c2c61ea8))
* **ui:** redesign layout with tabs, full-height table, and tighter spacing ([5bfc136](https://github.com/imdsutil/barnacle-imds-proxy/commit/5bfc136532aa88a7f7597f53219d4f5ca3cf90a8))


### Bug Fixes

* **ui:** address empty state, copy affordance, error retry, and loading consistency ([79aff0b](https://github.com/imdsutil/barnacle-imds-proxy/commit/79aff0b4ecd27ac574c72c13f1e99c10891d139a))
* **ui:** align left margin with Docker Desktop native sections ([9b2e9fb](https://github.com/imdsutil/barnacle-imds-proxy/commit/9b2e9fb22aa8f8276e13ae80eab265cf8af3ac68))
* **ui:** improve Save button state clarity ([24d1a83](https://github.com/imdsutil/barnacle-imds-proxy/commit/24d1a83facd903672b8caff0c2b437524d7ab686))
* **ui:** suppress scrollbar flicker and skeleton flash during container polls ([3ab62ed](https://github.com/imdsutil/barnacle-imds-proxy/commit/3ab62ed0deb1ff426a83ef27dee0123b04e0daf9))
* use standard alert variant for light mode readability ([a3b8c51](https://github.com/imdsutil/barnacle-imds-proxy/commit/a3b8c5150e54fb62b979fcb98dd9705a8644c416))

## [0.3.0](https://github.com/imdsutil/barnacle-imds-proxy/compare/v0.2.3...v0.3.0) (2026-03-22)


### Features

* simplify container label and add marketplace description ([71ff7e0](https://github.com/imdsutil/barnacle-imds-proxy/commit/71ff7e08f13b9ed29383db39d50188d677467092))

## [0.2.3](https://github.com/imds-tools/barnacle-imds-proxy/compare/v0.2.2...v0.2.3) (2026-03-13)


### Bug Fixes

* update changelog URL in Dockerfile ([4cddf7c](https://github.com/imds-tools/barnacle-imds-proxy/commit/4cddf7c0435cb333a6c2bc5a638753acb5341165))
* update changelog URL in Dockerfile ([efa76b8](https://github.com/imds-tools/barnacle-imds-proxy/commit/efa76b8d9c14e9eb7fef4313ec4a590b7b43df18))

## [0.2.2](https://github.com/imds-tools/barnacle-imds-proxy/compare/v0.2.1...v0.2.2) (2026-03-12)


### Bug Fixes

* update permissions and use correct token for GHCR login ([12eccd0](https://github.com/imds-tools/barnacle-imds-proxy/commit/12eccd0498dc9e12aca9e7eb2979a94986ad5c04))
* update permissions and use correct token for GHCR login ([5de7f3d](https://github.com/imds-tools/barnacle-imds-proxy/commit/5de7f3dd180bd3c100fc9f5387df41b952bb9d8c))

## [0.2.1](https://github.com/imds-tools/barnacle-imds-proxy/compare/v0.2.0...v0.2.1) (2026-03-12)


### Bug Fixes

* pin release-please-action version ([7f3af5a](https://github.com/imds-tools/barnacle-imds-proxy/commit/7f3af5a423bb96bc53833e05dd6cba84216c430a))
* pin release-please-action version ([c72d20b](https://github.com/imds-tools/barnacle-imds-proxy/commit/c72d20b1bce194f9c27596b165a4a250829a92ea))

## [0.2.0](https://github.com/imds-tools/barnacle-imds-proxy/compare/v0.1.1...v0.2.0) (2026-03-12)


### Features

* add release workflow and configuration for versioning ([13236c4](https://github.com/imds-tools/barnacle-imds-proxy/commit/13236c46c89f118969582fcfe422faeedd2ceb95))
* add release workflow and configuration for versioning ([3b00231](https://github.com/imds-tools/barnacle-imds-proxy/commit/3b00231032335ed62fa0d5a753b1578b434d82e1))
* initial commit ([2b1f073](https://github.com/imds-tools/barnacle-imds-proxy/commit/2b1f07343a7c0738729b535463c3df4ecb308f9e))
