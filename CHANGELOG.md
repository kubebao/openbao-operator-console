# Changelog

## 0.1.0 (2026-11-14)


### Features

* add a local evaluation environment with make evaluation ([829a987](https://github.com/kubebao/openbao-operator-console/commit/829a987d23e7ca49d9420ca6d21abbaf13fbafde))
* **api:** serve typed problems and paginated structured history ([d146779](https://github.com/kubebao/openbao-operator-console/commit/d1467790974e9844c8b0213dad299a2d613ff5de))
* **chart:** derive the callback from publicURL and read the setup token from the runtime Secret ([65b4c9a](https://github.com/kubebao/openbao-operator-console/commit/65b4c9aaeaad9ec4cb0710f932dd1893b271a0e7))
* **chart:** serve HTTPS from the console with tls.secretName ([6d08e6d](https://github.com/kubebao/openbao-operator-console/commit/6d08e6d746ce34560b8c6184da55d2f16b79b12c))
* **client:** migrate console to shared operator client ([#24](https://github.com/kubebao/openbao-operator-console/issues/24)) ([c5a85a1](https://github.com/kubebao/openbao-operator-console/commit/c5a85a1526b655b5371b47f919cb7e4fc33c538e))
* **console:** accept encoded keys, trim settings, and allow container demo ([7157cc3](https://github.com/kubebao/openbao-operator-console/commit/7157cc318c012315db8e63c98b2c80caa13ce54e))
* **console:** add local account model, credential store, and verification ([5411cc2](https://github.com/kubebao/openbao-operator-console/commit/5411cc2b81e5b444494e0b5330834c4742334a65))
* **console:** manage local accounts with enrollment links and recovery ([76f9f41](https://github.com/kubebao/openbao-operator-console/commit/76f9f412e13b644f90ec9a69b785665afd4cc720))
* **console:** resolve client addresses behind trusted proxies ([77d850d](https://github.com/kubebao/openbao-operator-console/commit/77d850d0eec3b6f5d4c631f18a01067cd984ce64))
* **console:** sign in and set up the first administrator with a local account ([47d5c91](https://github.com/kubebao/openbao-operator-console/commit/47d5c91d69d63bf338b185a2750d262f37e1268c))
* **deployment:** redesign deployment view around evidence and history ([4d54fa1](https://github.com/kubebao/openbao-operator-console/commit/4d54fa1682b590b49ead227b2ee00e2e3acfbe46))
* **enrollment:** adopt native operator API ([#34](https://github.com/kubebao/openbao-operator-console/issues/34)) ([de426cc](https://github.com/kubebao/openbao-operator-console/commit/de426ccabd9218ff4dc84a3f5609a25a41c2ab95))
* initial public version ([e409887](https://github.com/kubebao/openbao-operator-console/commit/e4098877eeaafefaa7bbea580f16600f06914347))
* **release:** publish static binary archives and a first-release checklist ([be396a8](https://github.com/kubebao/openbao-operator-console/commit/be396a8e775dc0501046446b4f04ff8c780ac9ce))
* **storage:** default to embedded SQLite with controlled schema upgrades ([9e7b690](https://github.com/kubebao/openbao-operator-console/commit/9e7b6900f781e8fbac736ca42e6a51f7f807087b))
* **web:** add topology map with cluster and relationship views ([481b7a5](https://github.com/kubebao/openbao-operator-console/commit/481b7a5428f6f7a7ac3a67a2d23e34e107bd51e5))
* **web:** adopt OpenBao brand colors and typography ([3e5925d](https://github.com/kubebao/openbao-operator-console/commit/3e5925de85c1bb6dba929fd499812c54d1855232))
* **web:** adopt the KubeBao logo ([bc89613](https://github.com/kubebao/openbao-operator-console/commit/bc89613c2940946af4d41766bbf1fc6c3d8024f4))
* **web:** drop the sign-in description ([7e9ec63](https://github.com/kubebao/openbao-operator-console/commit/7e9ec6388ef5badad9064c9c12eb0d1eed1bc727))
* **web:** manage local accounts, enrollment links, and account credentials ([8076f57](https://github.com/kubebao/openbao-operator-console/commit/8076f57493cfe7bf702b2f56ed163f9054d00246))
* **web:** prefill operator defaults and explain hosting-cluster RBAC in onboarding ([5cf6ecb](https://github.com/kubebao/openbao-operator-console/commit/5cf6ecbe8ec6d3427d64cb92a3357f4c2ea8d61b))
* **web:** redesign sign-in and adopt the OpenBao Operator mark ([c517a08](https://github.com/kubebao/openbao-operator-console/commit/c517a08a6194b47880e8d996804d00b027c131a8))
* **web:** show backup activity as a week grid with policy timing ([30e8b65](https://github.com/kubebao/openbao-operator-console/commit/30e8b65363b577e1187b1d48c7842a57b188049b))


### Bug Fixes

* **api:** add the pending operator collection state ([5ae0043](https://github.com/kubebao/openbao-operator-console/commit/5ae0043e06221fc3d2d5cb47190d2696c056a229))
* **auth:** bind access to its issuer and harden settings and login state ([536d42b](https://github.com/kubebao/openbao-operator-console/commit/536d42ba2d9c5b2673c3a2c2cd68b86925329a4d))
* **charts:** load complete history independently of observations ([#33](https://github.com/kubebao/openbao-operator-console/issues/33)) ([180b9c3](https://github.com/kubebao/openbao-operator-console/commit/180b9c351daf7ec7676f1d840d53346f119a1e0e))
* **ci:** prepare public launch validation ([#28](https://github.com/kubebao/openbao-operator-console/issues/28)) ([c4e2018](https://github.com/kubebao/openbao-operator-console/commit/c4e20188f5ffc62a49f95b63613d3769b2905c92))
* **client:** restore backup coverage for successful attempts ([#38](https://github.com/kubebao/openbao-operator-console/issues/38)) ([82354bd](https://github.com/kubebao/openbao-operator-console/commit/82354bd519de52792790db17eb0cd7b54d4d5fbf))
* **collection:** preserve evidence and maintain authorized durable history ([84635a5](https://github.com/kubebao/openbao-operator-console/commit/84635a585efe26f8113fe5f0f21767ca2d2e97ea))
* **console:** explain how to supply a missing setup token ([daf5d98](https://github.com/kubebao/openbao-operator-console/commit/daf5d988ce09ac9c4bedb24818f26b9d641cd4a2))
* **edge:** unpack bundles in a fresh publisher checkout ([1316923](https://github.com/kubebao/openbao-operator-console/commit/1316923bcb83aa778408439b2972b03d5cf56e6a))
* **history:** record only visible changes and move caveats into tooltips ([5cf8624](https://github.com/kubebao/openbao-operator-console/commit/5cf8624d35177896d7691a449351351f25ce1da5))
* **restore:** adopt qualified restore retention diagnostics ([#36](https://github.com/kubebao/openbao-operator-console/issues/36)) ([759c0f2](https://github.com/kubebao/openbao-operator-console/commit/759c0f2b0333b85c7967afdf675ff2cdaa3e25d8))
* **settings:** keep local accounts when saving settings ([583316d](https://github.com/kubebao/openbao-operator-console/commit/583316d54cd8574c11379fa4f2ec93606b05d0e1))
* **view:** keep the real collection attempt time for stale evidence ([33c8af0](https://github.com/kubebao/openbao-operator-console/commit/33c8af07995c178e1bce1a2d5ab839f2e61aa2e0))
* **web:** clarify evidence and preserve accessible workflows ([d7a7b73](https://github.com/kubebao/openbao-operator-console/commit/d7a7b73a46470a3850ab384732bd9455c870b957))


### Documentation

* **console:** document operations and refresh evidence screenshots ([d9e8ad3](https://github.com/kubebao/openbao-operator-console/commit/d9e8ad342665cac967cf27c197653ed81a776aff))
* describe local account management, local sign-in, and recovery ([4a46e03](https://github.com/kubebao/openbao-operator-console/commit/4a46e03062fd8a8d3faa5e1ae85d9a5249d7fea3))
* lead the quick start with the container demo ([192fa1c](https://github.com/kubebao/openbao-operator-console/commit/192fa1c9b34dcfd2b87da520d8ad1949aef43183))
* shorten the README and mark features that await operator support ([db878ba](https://github.com/kubebao/openbao-operator-console/commit/db878ba9a9ac68d9d0abc9e2c7612703d081c9df))
* show the screenshots in the guides they illustrate ([922688b](https://github.com/kubebao/openbao-operator-console/commit/922688be86a421893fa309fe7bb4af92221a9aba))

## Changelog

Release notes are generated by [release-please](https://github.com/googleapis/release-please) from
[Conventional Commits](https://www.conventionalcommits.org/). See [releasing](docs/releasing.md).
