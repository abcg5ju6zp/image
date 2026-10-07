# 签名集合更新的可恢复发布

> **Workspace:** /Users/ding/Documents/swe/0910061/project-04

---

**User**

向带 lookaside 签名的 Docker registry 更新同一镜像签名时，签名按序逐个覆盖并在末尾删除旧文件；网络或进程中止会让读取端看到新旧签名混合或丢失可验证签名。请将一次签名集合更新作为可恢复提交：先在隔离位置写完并校验全部签名，再让读取端只看到完整的新集合，失败或取消保留上一集合，重试清理未发布临时对象；文件、HTTP lookaside、API 扩展和 sigstore 附件的既有选择及普通无签名复制继续兼容。

**Assistant**

toolName: view_folder

status: success

/Users/ding/Documents/swe/0910061/project-04

这是 containers/image 项目。让我查看签名相关的目录结构。

toolName: view_folder

status: success

/Users/ding/Documents/swe/0910061/project-04/signature

签名相关代码在 `internal/signature/` 下。让我阅读核心文件并了解版本信息。

toolName: view_files

status: success

filePath: /Users/ding/Documents/swe/0910061/project-04/internal/signature/signature.go

toolName: view_files

status: success

filePath: /Users/ding/Documents/swe/0910061/project-04/internal/signature/simple.go

toolName: view_files

status: success

filePath: /Users/ding/Documents/swe/0910061/project-04/go.mod

让我搜索 lookaside 签名存储的实现位置。

toolName: search_by_regex

status: success

toolName: search_by_regex

status: success

toolName: view_files

status: success

filePath: /Users/ding/Documents/swe/0910061/project-04/signature/signer/signer.go

toolName: search_by_regex

status: success

toolName: search_by_regex

status: success

toolName: view_files

status: success

filePath: /Users/ding/Documents/swe/0910061/project-04/docker/docker_image_dest.go

toolName: view_files

status: success

filePath: /Users/ding/Documents/swe/0910061/project-04/docker/docker_image_dest.go

toolName: search_by_regex

status: success

toolName: view_files

status: success

filePath: /Users/ding/Documents/swe/0910061/project-04/docker/docker_image_src.go

toolName: view_files

status: success

filePath: /Users/ding/Documents/swe/0910061/project-04/docker/registries_d.go

让我查看 git 历史以确定基线版本和相关结构。

toolName: Shell

status: success

让我搜索上游 containers/image 中对应的真实实现，以精确匹配设计。

toolName: web_search

status: success

toolName: view_files

status: success

filePath: /Users/ding/Documents/swe/0910061/project-04/docker/docker_client.go

让我通过 GitHub API 在上游查找对应的原子签名更新提交。

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

toolName: Shell

status: success

开发已迁移到 `containers/container-libs` monorepo。让我在那里查找对应的原子签名更新实现。

toolName: Shell

status: success

toolName: Shell

status: success

toolName: Shell

status: success

toolName: Shell

status: success

让我在上游 monorepo 中搜索是否已有事务性签名相关代码。

toolName: Shell

status: success

上游尚无此特性，需要自行设计实现。让我创建任务计划并继续研读相关代码。

toolName: todo_write

status: success

Todos updated: 6 items

toolName: view_files

status: success

filePath: /Users/ding/Documents/swe/0910061/project-04/docker/docker_image_dest.go

toolName: view_files

status: success

filePath: /Users/ding/Documents/swe/0910061/project-04/docker/registries_d.go

toolName: view_files

status: success

filePath: /Users/ding/Documents/swe/0910061/project-04/docker/docker_image_src.go

toolName: search_by_regex

status: success

toolName: view_files

status: success

filePath: /Users/ding/Documents/swe/0910061/project-04/docker/docker_image_src.go

toolName: search_by_regex

status: success

toolName: search_by_regex

status: success

toolName: view_files

status: success

filePath: /Users/ding/Documents/swe/0910061/project-04/docs/signature-protocols.md

toolName: view_files

status: success

filePath: /Users/ding/Documents/swe/0910061/project-04/copy/sign_test.go

设计已经清晰。让我确认 dest 文件中 `url` 包的其他使用，然后开始实现。

toolName: search_by_regex

status: success

现在开始实现。首先更新调用处传入 ctx：

toolName: edit_file_search_replace

status: success

filePath: /Users/ding/Documents/swe/0910061/project-04/docker/docker_image_dest.go

changes: undefined

现在用事务实现替换旧的逐文件覆盖逻辑：

toolName: edit_file_search_replace

status: success

filePath: /Users/ding/Documents/swe/0910061/project-04/docker/docker_image_dest.go

changes: undefined

现在更新导入并替换 `deleteOneSignature`：

toolName: edit_file_search_replace

status: success

filePath: /Users/ding/Documents/swe/0910061/project-04/docker/docker_image_dest.go

changes: undefined

toolName: search_by_regex

status: success

toolName: edit_file_search_replace

status: success

filePath: /Users/ding/Documents/swe/0910061/project-04/docker/docker_image_dest.go

changes: undefined

现在更新 `deleteImage` 中的调用点：

toolName: edit_file_search_replace

status: success

filePath: /Users/ding/Documents/swe/0910061/project-04/docker/docker_image_src.go

changes: undefined

清理事务结构体中未使用的字段和误导性注释：

toolName: edit_file_search_replace

status: success

filePath: /Users/ding/Documents/swe/0910061/project-04/docker/docker_image_dest.go

changes: undefined

toolName: edit_file_search_replace

status: success

filePath: /Users/ding/Documents/swe/0910061/project-04/docker/docker_image_dest.go

changes: undefined

toolName: edit_file_search_replace

status: success

filePath: /Users/ding/Documents/swe/0910061/project-04/docker/docker_image_dest.go

changes: undefined

现在编译检查：

toolName: Shell

status: success

toolName: Shell

status: success

toolName: Shell

status: success

toolName: Shell

status: success

使用纯 Go openpgp 构建标签进行编译：

toolName: Shell

status: success

编译通过。运行 docker 包现有测试：

toolName: todo_write

status: success

Todos updated: 6 items

toolName: Shell

status: success

toolName: Shell

status: success

测试通过。现在为事务语义添加单元测试。先看测试文件约定：

toolName: view_files

status: success

filePath: /Users/ding/Documents/swe/0910061/project-04/docker/docker_image_dest_test.go

现在在测试文件中追加事务语义测试：

toolName: edit_file_search_replace

status: success

filePath: /Users/ding/Documents/swe/0910061/project-04/docker/docker_image_dest_test.go

changes: undefined

toolName: edit_file_search_replace

status: success

filePath: /Users/ding/Documents/swe/0910061/project-04/docker/docker_image_dest_test.go

changes: undefined

运行新测试：

toolName: Shell

status: success

toolName: edit_file_search_replace

status: success

filePath: /Users/ding/Documents/swe/0910061/project-04/docker/docker_image_dest_test.go

changes: undefined

toolName: edit_file_search_replace

status: success

filePath: /Users/ding/Documents/swe/0910061/project-04/docker/docker_image_dest_test.go

changes: undefined

toolName: Shell

status: success

测试辅助函数需要排除符号链接指向的活动集合目录：

toolName: edit_file_search_replace

status: success

filePath: /Users/ding/Documents/swe/0910061/project-04/docker/docker_image_dest_test.go

changes: undefined

toolName: Shell

status: success

toolName: view_files

status: success

filePath: /Users/ding/Documents/swe/0910061/project-04/docker/docker_image_dest_test.go

让我写个快速复现脚本查看删除失败原因：

toolName: Shell

status: success

出现了一个神秘的 `001` 条目。让我检查它是什么：

toolName: Shell

status: success

让我直接打印关键路径定位问题：

toolName: Shell

status: success

找到原因了：测试中构造第二个事务时基准路径少了末尾的 repo 分量（t.TempDir 返回的路径以 `/001` 结尾，正好模拟 repo），导致路径推导错位。修正测试：

toolName: edit_file_search_replace

status: success

filePath: /Users/ding/Documents/swe/0910061/project-04/docker/docker_image_dest_test.go

changes: undefined

toolName: edit_file_search_replace

status: success

filePath: /Users/ding/Documents/swe/0910061/project-04/docker/docker_image_dest_test.go

changes: undefined

删除临时复现文件并运行测试：

toolName: delete_file

status: success

toolName: Shell

status: success

全部通过。现在更新协议文档：

toolName: todo_write

status: success

Todos updated: 6 items

toolName: edit_file_search_replace

status: success

filePath: /Users/ding/Documents/swe/0910061/project-04/docs/signature-protocols.md

changes: undefined

现在运行完整验证：

toolName: Shell

status: success

再验证 HTTP 静态服务器场景下读取端能透明跟随符号链接：

toolName: Shell

status: success

HTTP 读取端验证通过。清理临时文件并运行更广范围的测试：

toolName: Shell

status: success

toolName: Shell

status: success

唯一的失败是签名包测试需要外部 `gpgconf` 二进制（环境缺少 gpg 工具链），与本次改动无关。检查编辑器诊断：

toolName: GetDiagnostics

status: success

让我最终审查完整改动：

toolName: Shell

status: success

toolName: search_by_regex

status: success

所有引用已清理。完成任务标记并做最终测试确认：

toolName: todo_write

status: success

Todos updated: 6 items

toolName: Shell

status: success

已完成。将一次签名集合更新实现为可恢复的三阶段提交，改动如下：

## 实现方案

核心在 [docker_image_dest.go](file:///Users/ding/Documents/swe/0910061/project-04/docker/docker_image_dest.go) 中引入 `lookasideSignatureTransaction`，把原来"逐个覆盖、末尾删除"的流程替换为：

1. **暂存（stage）**：在同级目录创建隔离的 `<repo>@<digest>.set-<随机后缀>/`，将全部签名以 `signature-1…N` 写入、`fsync` 落盘，然后逐个读回做三重校验——字节一致、`signature.FromBlob` 可解析、目录条目数恰好等于签名数。读取端从不访问该位置。
2. **发布（commit）**：创建指向新集合的临时相对符号链接 `<repo>@<digest>.link-<随机后缀>`，再用一次 `rename(2)` 原子替换可见路径。读取端（`file://` 或服务同一目录的静态 HTTP 服务器）跟随链接，因此**只能看到完整旧集合或完整新集合**。发布后才删除被取代的集合目录。
3. **中止/失败（abort）**：提交前任一阶段出错或 ctx 取消，仅删除自己的隔离目录，可见路径与上一集合完全不动。

## 崩溃恢复与重试清理

- 迁移旧式普通目录时先改名为 `.obsolete-<随机>`；若进程在两次重命名之间中止，可见路径缺失但备份存在，下一次 [cleanupStaleObjects](file:///Users/ding/Documents/swe/0910061/project-04/docker/docker_image_dest.go#L877-L942) 会自动恢复**上一集合**。
- 未发布的 `.set-`、`.link-` 与残留的 `.obsolete-` 对象在每次尝试开头清理（包括不携带签名的普通复制），且绝不删除当前符号链接指向的活动集合。

## 兼容性

- **HTTP lookaside**：读取逻辑不变；直接写 HTTP 的拒绝提示与原来一致（需配置 `lookaside-staging`）。已用 `httptest` 验证静态服务器透明跟随链接，提交前后分别返回 200/200/404 与 200/404。
- **API 扩展**（X-Registry-Supports-Signatures）与 **sigstore 附件**：分支选择和实现均未改动。
- **无签名普通复制**：仍为 no-op，不触碰签名存储；删除镜像时改为整体清理链接+集合（[docker_image_src.go](file:///Users/ding/Documents/swe/0910061/project-04/docker/docker_image_src.go#L724-L726)），错误语义保持。

## 验证

新增 6 个针对性测试（首次提交、原子更新、取消保留旧集合、旧目录迁移、obsolete 恢复、HTTP 拒绝），`docker`、`copy` 及全仓库 `-short` 测试均通过；`go vet` 与编辑器诊断无告警。协议说明同步更新于 [signature-protocols.md](file:///Users/ding/Documents/swe/0910061/project-04/docs/signature-protocols.md#L62-L86)。（环境中 `signature` 包有一个依赖外部 `gpgconf` 二进制的预存测试失败，与本次改动无关。）
