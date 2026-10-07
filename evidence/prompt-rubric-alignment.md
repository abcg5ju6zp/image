# Prompt-Rubric 对齐

Prompt-Rubric 对齐表：P1 BlobWriter 将写入、关闭、digest 校验和持久化登记作为提交 → R1-R2；P2 短写、尾随、断连、fsync 失败清理候选 → R3；P3 重试隔离 writer 且保护确认 blob → R4、R6；P4 重开清理未确认临时文件并保留索引 → R5；P5 压缩、加密、分块、seek、普通 PutBlob 和镜像读取兼容 → R7-R8。无额外验收要求。
