# 短写与重试下的单一 blob 内容

> **Workspace:** /Users/ding/Documents/swe/0910061/project-05

---

**User**

镜像写入端收到短于声明长度的流或遇到网络重试时，目标文件可能保留旧尾部，digest 却已登记成功，后续复制因复用错误 blob 才暴露 manifest 校验失败。请把 BlobWriter 的写入、关闭、digest 校验和持久化登记视为一次可恢复提交：短写、尾随内容、断连和 fsync 失败都不能产生可复用候选；重试隔离不同 writer，已确认 blob 不被失败 writer 删除。重新打开目标时清理未确认临时文件，压缩、加密、分块和 seek 语义继续兼容。

**Assistant**

我先了解一下项目结构和 BlobWriter 的现有实现。

toolName: view_folder

status: success

/Users/ding/Documents/swe/0910061/project-05

toolName: todo_write

status: success

Todos updated: 6 items

toolName: search_by_regex

status: success

toolName: search_by_regex

status: success

toolName: view_files

status: success

filePath: /Users/ding/Documents/swe/0910061/project-05/pkg/blobcache/blobcache.go

toolName: view_files

status: success

filePath: /Users/ding/Documents/swe/0910061/project-05/directory/directory_dest.go

toolName: Shell

status: success

toolName: search_by_regex

status: success

toolName: view_folder

status: success

/Users/ding/Documents/swe/0910061/project-05/storage

toolName: search_by_regex

status: success

toolName: view_files

status: success

filePath: /Users/ding/Documents/swe/0910061/project-05/pkg/blobcache/dest.go

toolName: Shell

status: success

toolName: search_by_regex

status: success

toolName: view_folder

status: success

/Users/ding/Documents/swe/0910061/project-05/types

toolName: view_folder

status: success

/Users/ding/Documents/swe/0910061/project-05/transports

toolName: view_folder

status: success

/Users/ding/Documents/swe/0910061/project-05/tarball

toolName: view_files

status: success

filePath: /Users/ding/Documents/swe/0910061/project-05/storage/storage_dest.go

让我查看 sif 包和 signature 目录，以及搜索包含 Seek 语义的写入器。

toolName: view_folder

status: success

/Users/ding/Documents/swe/0910061/project-05/sif

toolName: search_by_regex

status: success

让我检查一下项目记忆，看之前是否有相关约定。

toolName: Shell

status: success

toolName: Shell

status: success

toolName: Shell

status: success

我来读一下 OCI layout 目标端的实现，判断 BlobWriter 最合适的落点。

toolName: view_files

status: success

filePath: /Users/ding/Documents/swe/0910061/project-05/oci/layout/oci_dest.go

现在查看目录/OCI 的路径解析和 putblobdigest 辅助函数。

toolName: view_files

status: success

filePath: /Users/ding/Documents/swe/0910061/project-05/internal/putblobdigest/put_blob_digest.go

toolName: view_files

status: success

filePath: /Users/ding/Documents/swe/0910061/project-05/oci/layout/reader.go

toolName: search_by_regex

status: success

toolName: search_by_regex

status: success

toolName: search_by_regex

status: success

toolName: view_files

status: success

filePath: /Users/ding/Documents/swe/0910061/project-05/oci/layout/oci_transport.go

toolName: view_files

status: success

filePath: /Users/ding/Documents/swe/0910061/project-05/go.mod

设计基本成型。我先检查现有测试是否会严格枚举目标目录内容（锁文件可能造成影响）。

toolName: search_by_regex

status: success

toolName: search_by_regex

status: success

实现方案已定。先检查现有测试是否依赖旧的错误信息文本。

toolName: todo_write

status: success

Todos updated: 6 items

toolName: search_by_regex

status: success

现在创建 `internal/blobwriter` 包。

toolName: Write

status: success

filePath: /Users/ding/Documents/swe/0910061/project-05/internal/blobwriter/blobwriter.go

changes: undefined

toolName: Write

status: success

filePath: /Users/ding/Documents/swe/0910061/project-05/internal/blobwriter/locking_unix.go

changes: undefined

toolName: Write

status: success

filePath: /Users/ding/Documents/swe/0910061/project-05/internal/blobwriter/locking_windows.go

changes: undefined

包核心完成。现在写全面的单元测试。

toolName: Write

status: success

filePath: /Users/ding/Documents/swe/0910061/project-05/internal/blobwriter/blobwriter_test.go

changes: undefined

修正非规范 digest 测试中残留的占位代码。

toolName: edit_file_search_replace

status: success

filePath: /Users/ding/Documents/swe/0910061/project-05/internal/blobwriter/blobwriter_test.go

changes: undefined

先运行新包的测试确认基础实现正确。

toolName: Shell

status: success

toolName: view_files

status: success

filePath: /Users/ding/Documents/swe/0910061/project-05/internal/blobwriter/blobwriter_test.go

toolName: edit_file_search_replace

status: success

filePath: /Users/ding/Documents/swe/0910061/project-05/internal/blobwriter/blobwriter_test.go

changes: undefined

toolName: edit_file_search_replace

status: success

filePath: /Users/ding/Documents/swe/0910061/project-05/internal/blobwriter/blobwriter_test.go

changes: undefined

toolName: Shell

status: success

需要在测试中注册 sha512 算法。

toolName: edit_file_search_replace

status: success

filePath: /Users/ding/Documents/swe/0910061/project-05/internal/blobwriter/blobwriter_test.go

changes: undefined

顺便修正 TestCommitShortWrite 中 glob 无法匹配点开头文件的问题。

toolName: edit_file_search_replace

status: success

filePath: /Users/ding/Documents/swe/0910061/project-05/internal/blobwriter/blobwriter_test.go

changes: undefined

toolName: Shell

status: success

19 个测试全部通过。现在把 BlobWriter 接入 directory 目标端。

toolName: todo_write

status: success

Todos updated: 6 items

toolName: edit_file_search_replace

status: success

filePath: /Users/ding/Documents/swe/0910061/project-05/directory/directory_dest.go

changes: undefined

toolName: edit_file_search_replace

status: success

filePath: /Users/ding/Documents/swe/0910061/project-05/directory/directory_dest.go

changes: undefined

toolName: edit_file_search_replace

status: success

filePath: /Users/ding/Documents/swe/0910061/project-05/directory/directory_dest.go

changes: undefined

现在接入 OCI layout 目标端。

toolName: edit_file_search_replace

status: success

filePath: /Users/ding/Documents/swe/0910061/project-05/oci/layout/oci_dest.go

changes: undefined

toolName: edit_file_search_replace

status: failed

filePath: /Users/ding/Documents/swe/0910061/project-05/oci/layout/oci_dest.go

changes: undefined

toolName: view_files

status: success

filePath: /Users/ding/Documents/swe/0910061/project-05/oci/layout/oci_dest.go

toolName: edit_file_search_replace

status: success

filePath: /Users/ding/Documents/swe/0910061/project-05/oci/layout/oci_dest.go

changes: undefined

toolName: edit_file_search_replace

status: success

filePath: /Users/ding/Documents/swe/0910061/project-05/oci/layout/oci_dest.go

changes: undefined

两端接入完成。现在编译并运行相关包的完整测试。

toolName: Shell

status: success

全量构建涉及 cgo（gpgme），改为只构建测试受影响的包。

toolName: Shell

status: success

OCI 全部通过。目录测试有一个用例失败，查看测试代码。

toolName: view_files

status: success

filePath: /Users/ding/Documents/swe/0910061/project-05/directory/directory_test.go

失败的根因很关键：[directory_test.go](file:///Users/ding/Documents/swe/0910061/project-05/directory/directory_test.go#L67-L80) 旧测试故意写入与声明 digest 不符的内容并期望被接受——这正是本任务要消除的“digest 登记了错误内容”行为。新契约下 PutBlob 文档也声明“digest MUST match stream contents”，将测试改为使用内容真实 digest（保留“调用方提供 digest”路径的测试意图）。

toolName: edit_file_search_replace

status: success

filePath: /Users/ding/Documents/swe/0910061/project-05/directory/directory_test.go

changes: undefined

toolName: Shell

status: success

补充两个传输层的“重开清理”集成测试。先看 OCI 测试的辅助函数。

toolName: view_files

status: success

filePath: /Users/ding/Documents/swe/0910061/project-05/oci/layout/oci_transport_test.go

toolName: view_files

status: success

filePath: /Users/ding/Documents/swe/0910061/project-05/oci/layout/oci_dest_test.go

toolName: search_by_regex

status: success

现在分别为两个传输端追加“重开清理”集成测试。

toolName: view_files

status: success

filePath: /Users/ding/Documents/swe/0910061/project-05/oci/layout/oci_dest_test.go

toolName: edit_file_search_replace

status: success

filePath: /Users/ding/Documents/swe/0910061/project-05/oci/layout/oci_dest_test.go

changes: undefined

toolName: view_files

status: success

filePath: /Users/ding/Documents/swe/0910061/project-05/directory/directory_test.go

toolName: edit_file_search_replace

status: success

filePath: /Users/ding/Documents/swe/0910061/project-05/directory/directory_test.go

changes: undefined

toolName: Shell

status: success

全部通过。现在运行 vet 并交叉编译 Windows 验证构建标签拆分。

toolName: Shell

status: success

再用 race 检测器跑一遍并发敏感的新包和集成包。

toolName: Shell

status: success

跑一下 copy 包测试确认无连带影响。

toolName: Shell

status: success

这是环境中缺少 pkg-config（gpgme cgo）导致，与本次改动无关。尝试禁用 cgo 运行。

toolName: Shell

status: success

copy 包测试因本机缺少 gpgme 的 pkg-config 无法构建（签名相关 cgo，与本次改动无关，改动未触及这些文件）。最后检查整体差异与诊断。

toolName: todo_write

status: success

Todos updated: 6 items

toolName: Shell

status: success

toolName: GetDiagnostics

status: success

完成。无诊断问题，所有受影响包测试通过（含 `-race`），Windows/FreeBSD 交叉编译通过。

## 改动说明

**新增事务性组件** [internal/blobwriter](file:///Users/ding/Documents/swe/0910061/project-05/internal/blobwriter/blobwriter.go)

`BlobWriter` 把「写入 → 关闭 → digest 校验 → 持久化登记」收敛为一次可恢复提交：

- **未确认临时文件**：所有字节先写入 staging 目录中的 `.blobwriter-ingest-<随机>` 文件，发布前任何读者都无法按 digest 找到它，因此短写不可能残留旧尾部（每次都是全新的 `O_EXCL` 临时文件）。
- **Commit 的完整顺序**（[Commit](file:///Users/ding/Documents/swe/0910061/project-05/internal/blobwriter/blobwriter.go#L204-L265)）：实际大小校验（少一字节即 `ErrSizeMismatch`）→ canonical digest 校验（不符即 `ErrDigestMismatch`）→ `Sync()` 落盘 → chmod 0644 → 关闭 → `os.Link` 硬链接原子发布到最终 digest 路径 → 父目录 fsync。
- **四类失败均不产生候选**：
  - 短写 → 大小校验失败；
  - 尾随内容 → `Write` 在超过声明大小时立即拒绝（`ErrTrailingData`），Commit 再兜底；
  - 断连 → 调用方不 Commit，`Close` 只删除自己的临时文件；
  - fsync 失败 → 发布前即中止（`syncFile` 可注入，已测试）。
- **重试隔离**：每次 New 生成独立临时文件；失败 writer 的 [abort](file:///Users/ding/Documents/swe/0910061/project-05/internal/blobwriter/blobwriter.go#L319-L325) 只删自己的 staging 文件。
- **已确认 blob 保护**：发布用硬链接而非覆盖；最终路径已存在时，只做「大小 + 全量 digest」校验，校验通过即幂等成功，校验失败返回错误但**绝不删除/替换**该文件（含冲突异文件场景，已测试）。
- **崩溃恢复**：[CleanupStaging](file:///Users/ding/Documents/swe/0910061/project-05/internal/blobwriter/blobwriter.go#L354-L386) 清理带 staging 前缀的文件；通过 [flock](file:///Users/ding/Documents/swe/0910061/project-05/internal/blobwriter/locking_unix.go) 协议——活动 writer 持共享锁、清理持排他锁——保证跨进程活动 writer 的文件不会被误删（Windows 无 flock，降级为尽力而为）。
- **语义兼容**：压缩/加密仍在 copy 管道上层作用于字节流；分块上传仍走 `PutBlobPartial` 独立路径，且 [WriteAt](file:///Users/ding/Documents/swe/0910061/project-05/internal/blobwriter/blobwriter.go#L166-L184) 支持乱序拼装 chunk；[Seek](file:///Users/ding/Documents/swe/0910061/project-05/internal/blobwriter/blobwriter.go#L187-L198) 后 Commit 改为对磁盘上完整文件重新哈希，保证登记 digest 与实际字节一致。

**接入两个本地文件目标端**

- [directory_dest.go](file:///Users/ding/Documents/swe/0910061/project-05/directory/directory_dest.go#L143-L168)：PutBlob 改用 BlobWriter；[newImageDestination](file:///Users/ding/Documents/swe/0910061/project-05/directory/directory_dest.go#L102-L106) 重开时清理未确认文件。
- [oci_dest.go](file:///Users/ding/Documents/swe/0910061/project-05/oci/layout/oci_dest.go#L124-L156)：同样接入，FinalPath 回调兼容 blobs 目录与 shared blob dir；[重开时清理](file:///Users/ding/Documents/swe/0910061/project-05/oci/layout/oci_dest.go#L97-L102)。`PutBlobFromLocalFile` 的 reflink 路径保持不变。
- 旧测试 [TestGetPutBlob](file:///Users/ding/Documents/swe/0910061/project-05/directory/directory_test.go#L64-L69) 原本写入与声明 digest 不符的内容并期望接受——正是本次要消除的行为，改为使用内容真实 digest（保留「调用方提供 digest」路径的覆盖意图）。

新增 19 个 blobwriter 单测及 2 个传输层重开清理测试，`go test -race` 全绿。`copy` 包测试因本机缺少 gpgme 的 pkg-config 无法构建，属既有环境限制，未触及该包代码。
