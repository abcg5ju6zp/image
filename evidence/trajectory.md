# 签名策略配置变更的可回退验证

> **Workspace:** /Users/ding/Documents/swe/0910061/project-06

---

**User**

管理员修改签名策略后立即拉取镜像时，策略文件可能只写了一半，或缓存已切换，使同一进程对不同 layer 使用不同 trust policy；重启还可能加载失败配置并拒绝所有镜像。请让策略更新先在候选配置中完成解析、引用和签名机制校验，再让读取路径一次切换到新策略；解析、写盘或进程退出失败时继续使用最后一份有效策略，启动时清理未完成候选。并发拉取、策略缓存、Docker/OCI/SIF 引用以及未配置策略的默认行为保持一致，并能区分配置错误和签名拒绝。

**Assistant**

toolName: view_folder

status: success

/Users/ding/Documents/swe/0910061/project-06

This is the `containers/image` Go library. Let me explore the signature policy code, which is the core of this task.

toolName: file_search

status: success

toolName: file_search

status: success

toolName: view_files

status: success

filePath: /Users/ding/Documents/swe/0910061/project-06/signature/policy_config.go

toolName: view_files

status: success

filePath: /Users/ding/Documents/swe/0910061/project-06/signature/policy_types.go

toolName: view_files

status: success

filePath: /Users/ding/Documents/swe/0910061/project-06/signature/policy_eval.go

Now let me look at how policies are used across the codebase and check for caching mechanisms and tests.

toolName: search_by_regex

status: success

toolName: search_by_regex

status: success

toolName: view_files

status: success

filePath: /Users/ding/Documents/swe/0910061/project-06/signature/policy_paths_common.go

toolName: view_files

status: success

filePath: /Users/ding/Documents/swe/0910061/project-06/signature/policy_paths_freebsd.go

toolName: search_by_regex

status: success

toolName: search_by_regex

status: success

toolName: view_files

status: success

filePath: /Users/ding/Documents/swe/0910061/project-06/signature/policy_config_test.go

Let me check the git state and version of this repo to understand the baseline.

toolName: Shell

status: success

Let me check memory for any prior context on this task family, and look at related code.

toolName: view_folder

status: success

/Users/ding/.trae-cn/memory

toolName: file_search

status: success

toolName: Shell

status: success

toolName: Shell

status: success

The memory sessions are empty. Now let me study how the pull flow consumes the policy, and the sif/oci transports' scope validation.

toolName: search_by_regex

status: success

toolName: view_files

status: success

filePath: /Users/ding/Documents/swe/0910061/project-06/sif/load.go

I found something interesting at line 201-205 in policy_config.go. Let me examine the paranoid JSON unmarshaler semantics and the mechanism validation.

toolName: view_files

status: success

filePath: /Users/ding/Documents/swe/0910061/project-06/signature/internal/json.go

Let me read the signature mechanism code to understand how keys/certs are loaded, so candidate validation can exercise the same mechanisms.

toolName: view_files

status: success

filePath: /Users/ding/Documents/swe/0910061/project-06/signature/mechanism.go

toolName: view_files

status: success

filePath: /Users/ding/Documents/swe/0910061/project-06/signature/policy_eval_signedby.go

toolName: search_by_regex

status: success

toolName: view_files

status: success

filePath: /Users/ding/Documents/swe/0910061/project-06/signature/policy_config_sigstore.go

toolName: view_files

status: success

filePath: /Users/ding/Documents/swe/0910061/project-06/signature/policy_eval_sigstore.go

toolName: view_files

status: success

filePath: /Users/ding/Documents/swe/0910061/project-06/signature/policy_eval_sigstore.go

Let me check the transports registry and scope validation for docker/oci/sif.

toolName: view_files

status: success

filePath: /Users/ding/Documents/swe/0910061/project-06/transports/transports.go

toolName: search_by_regex

status: success

Let me check the build-tag variants for the GPG mechanism and whether `sif`/`oci` imports would create cycles.

toolName: Shell

status: success

toolName: view_files

status: success

filePath: /Users/ding/Documents/swe/0910061/project-06/signature/policy_eval_baselayer.go

toolName: Shell

status: success

The package builds. Per the workflow requirement, let me recall the closest prior experience before implementing.
That experience is about Windows Docker operations and isn't applicable to this Go library change. Let me set up my task plan and inspect usable key fixtures.

toolName: todo_write

status: success

Todos updated: 6 items

toolName: search_by_regex

status: success

toolName: search_by_regex

status: success

toolName: view_files

status: success

filePath: /Users/ding/Documents/swe/0910061/project-06/signature/policy_eval_sigstore_test.go

Fixtures confirmed. Let me look at the docker transport scope validator and the test helpers (`dirImageMock`) I can reuse.

toolName: view_files

status: success

filePath: /Users/ding/Documents/swe/0910061/project-06/docker/docker_transport.go

toolName: search_by_regex

status: success

toolName: search_by_regex

status: success

toolName: view_files

status: success

filePath: /Users/ding/Documents/swe/0910061/project-06/sif/transport.go

toolName: view_files

status: success

filePath: /Users/ding/Documents/swe/0910061/project-06/oci/layout/oci_transport.go

I have the full picture. Now let me implement the policy store with candidate validation, atomic commit, and snapshot cache.

toolName: Write

status: success

filePath: /Users/ding/Documents/swe/0910061/project-06/signature/policy_store.go

changes: undefined

toolName: Shell

status: success

The default gpgme CGO backend needs pkg-config (unavailable here); I'll validate with the pure-Go openpgp build tag.

toolName: Shell

status: success

toolName: Shell

status: success

toolName: search_by_regex

status: success

Now let me check OCI's scope validation to pick valid/invalid scope values for tests.

toolName: search_by_regex

status: success

toolName: search_by_regex

status: success

toolName: Write

status: success

filePath: /Users/ding/Documents/swe/0910061/project-06/signature/policy_store_test.go

changes: undefined

Let me fix the convoluted last test and make the concurrency marker reader goroutine-safe.

toolName: edit_file_search_replace

status: success

filePath: /Users/ding/Documents/swe/0910061/project-06/signature/policy_store_test.go

changes: undefined

toolName: edit_file_search_replace

status: success

filePath: /Users/ding/Documents/swe/0910061/project-06/signature/policy_store_test.go

changes: undefined

toolName: edit_file_search_replace

status: success

filePath: /Users/ding/Documents/swe/0910061/project-06/signature/policy_store_test.go

changes: undefined

toolName: edit_file_search_replace

status: success

filePath: /Users/ding/Documents/swe/0910061/project-06/signature/policy_store_test.go

changes: undefined

toolName: Shell

status: success

Two test bugs to investigate. Let me probe how these policy types actually marshal and check the existing baseline tests.

toolName: Shell

status: success

Found a real round-trip bug worth handling: `Transports: nil` marshals to `"transports":null`, which the strict parser rejects. The store must normalize before persisting. Let me fix the store and tests.

toolName: edit_file_search_replace

status: success

filePath: /Users/ding/Documents/swe/0910061/project-06/signature/policy_store.go

changes: undefined

toolName: edit_file_search_replace

status: success

filePath: /Users/ding/Documents/swe/0910061/project-06/signature/policy_store_test.go

changes: undefined

toolName: edit_file_search_replace

status: success

filePath: /Users/ding/Documents/swe/0910061/project-06/signature/policy_store_test.go

changes: undefined

toolName: edit_file_search_replace

status: success

filePath: /Users/ding/Documents/swe/0910061/project-06/signature/policy_store_test.go

changes: undefined

toolName: Shell

status: success

toolName: view_files

status: success

filePath: /Users/ding/Documents/swe/0910061/project-06/signature/policy_eval_test.go

toolName: edit_file_search_replace

status: success

filePath: /Users/ding/Documents/swe/0910061/project-06/signature/policy_store_test.go

changes: undefined

toolName: Shell

status: success

All targeted tests pass. Now run the race detector on store tests, then the whole signature package suite.

toolName: todo_write

status: success

Todos updated: 6 items

toolName: Shell

status: success

toolName: Shell

status: success

toolName: Shell

status: success

toolName: Shell

status: success

The only failure is pre-existing and environment-related (`gpgconf` binary missing), unrelated to my change. Let me confirm the rest of the package passes by excluding it.

toolName: Shell

status: success

toolName: Shell

status: success

toolName: Shell

status: success

toolName: Shell

status: success

toolName: Shell

status: success

Let me check the repo's lint config for line-length/style rules and verify my files comply.

toolName: Shell

status: success

toolName: Shell

status: success

toolName: Shell

status: success

toolName: Shell

status: success

toolName: search_by_regex

status: success

toolName: Shell

status: success

toolName: edit_file_search_replace

status: success

filePath: /Users/ding/Documents/swe/0910061/project-06/signature/policy_store_test.go

changes: undefined

toolName: edit_file_search_replace

status: success

filePath: /Users/ding/Documents/swe/0910061/project-06/signature/policy_store_test.go

changes: undefined

toolName: edit_file_search_replace

status: success

filePath: /Users/ding/Documents/swe/0910061/project-06/signature/policy_store_test.go

changes: undefined

toolName: Shell

status: success

toolName: Shell

status: success

My files are lint-clean; the single remaining issue is pre-existing in [mechanism_openpgp.go](file:///Users/ding/Documents/swe/0910061/project-06/signature/mechanism_openpgp.go#L153) (untouched). Final verification run:

toolName: Shell

status: success

Let me do a final review read of the complete implementation file.

toolName: view_files

status: success

filePath: /Users/ding/Documents/swe/0910061/project-06/signature/policy_store.go

Let me simplify two unused named return values.

toolName: edit_file_search_replace

status: success

filePath: /Users/ding/Documents/swe/0910061/project-06/signature/policy_store.go

changes: undefined

toolName: edit_file_search_replace

status: success

filePath: /Users/ding/Documents/swe/0910061/project-06/signature/policy_store.go

changes: undefined

toolName: Shell

status: success

toolName: Shell

status: success

toolName: todo_write

status: success

Todos updated: 6 items

已完成。改动全部落在 signature 包，新增两个文件，未修改任何既有文件，现有 API 与行为完全保持不变。

## 实现：[policy_store.go](file:///Users/ding/Documents/swe/0910061/project-06/signature/policy_store.go)

**候选先校验，再一次切换读取路径**
- `UpdateFromBytes`/`Update` 先在内存候选上完成三道关，全部通过才落盘：
  1. 解析：复用 `NewPolicyFromBytes` 的严格 JSON 解析（重复键、未知字段、空要求集、dockerReference 合法性等）；
  2. 引用校验：新增 [ValidatePolicy](file:///Users/ding/Documents/swe/0910061/project-06/signature/policy_store.go#L284-L333)，对已注册 transport 的每个 scope 调 `ValidatePolicyConfigurationScope`（Docker/OCI/SIF 全覆盖；SIF 拒绝相对路径和 `/`，OCI 要求绝对规范路径；未知 transport 沿用解析时的宽容语义）；
  3. 签名机制校验：signedBy 实际加载 keyPath/keyPaths/keyData 并建立 GPG 临时机制、要求至少导入一个公钥；sigstore 复用 `prepareTrustRoot()` 实例化公钥/Rekor/Fulcio/PKI 信任根；无实现的 keyType（X.509、signedByGPGKeys）直接判为配置错误（对应代码里原有的 "Reject this at policy parsing time" FIXME）。
- 提交顺序：写同目录临时候选 `.policy.json.candidate-*` → `Sync` → 原子 `rename` 覆盖主文件 → fsync 目录 → 最后在锁内做**一次**内存指针切换。拉取用的 `PolicyContext` 固定持有切换前的快照，整次拉取（manifest list/各 layer）绝不可能混用两套策略。

**失败保留最后有效策略**
- 解析/校验失败：不写盘、不切指针；写盘/重命名失败：删除临时文件、不切指针。并发更新由 `updateMu` 串行化，读取只在指针切换瞬间持锁，磁盘 I/O 不阻塞并发拉取。
- 进程写到一半退出：候选与主文件分离，主文件始终是上一份完整有效策略；[NewPolicyStore](file:///Users/ding/Documents/swe/0910061/project-06/signature/policy_store.go#L92-L109) 启动时先 glob 清理残留候选，再加载并全量校验主文件；重启不会加载半成品或 reject-all 配置。
- 未配置策略：[NewDefaultPolicyStore](file:///Users/ding/Documents/swe/0910061/project-06/signature/policy_store.go#L78-L84) 沿用 `DefaultPolicy` 的路径解析，缺文件返回同类错误，不伪造回退策略。
- 持久化用的 `marshalPolicy` 还修掉一个往返陷阱：nil `Transports` 会被编码成严格解析器拒绝的 `"transports":null`，落盘时规范化为 `{}`。

**区分两类错误**：[IsPolicyConfigurationError](file:///Users/ding/Documents/swe/0910061/project-06/signature/policy_store.go#L401-L410)（`InvalidPolicyFormatError`/严格解析 `JSONFormatError`，含坏引用、坏密钥、机制不可用）与 [IsPolicyRequirementError](file:///Users/ding/Documents/swe/0910061/project-06/signature/policy_store.go#L416-L421)（`PolicyRequirementError`：有效策略做出的签名拒绝）。

## 测试：[policy_store_test.go](file:///Users/ding/Documents/swe/0910061/project-06/signature/policy_store_test.go)

30+ 用例，使用仓库真实夹具（`public-key.gpg`、`cosign.pub`、`rekor.pub`、Fulcio/PKI 证书）：合法/非法机制、Docker/OCI/SIF 合法与非法 scope、坏 JSON/坏引用/缺密钥/坏 PEM 候选均回退且主文件字节不变、写盘失败保留旧策略、启动清理候选且不动无关文件、快照跨更新固定、8 reader + 200 次更新（含坏候选）的 `-race` 并发一致性、无策略默认行为、端到端验证 reject 属于签名拒绝而非配置错误。

验证：`go build ./...` 通过；signature 包测试全部通过（含 `-race`）、gofmt/govt/golangci-lint 对新文件零告警。本机无 gpgme/pkg-config，使用 `containers_image_openpgp` 标签测试；唯一失败的 `TestSignDockerManifestWithPassphrase` 是环境缺 `gpgconf` 导致的既有失败，与本次改动无关。
