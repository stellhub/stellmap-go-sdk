# Release Guide

## 当前版本

- `VERSION`: `v0.1.0`

## 发布前检查

```bash
gofmt -w ./...
go test ./...
```

## 建议发布步骤

1. 提交当前改动

```bash
git add .
git commit -m "feat: initialize stellmap go sdk"
```

2. 创建版本标签

```bash
git tag v0.1.0
```

3. 推送分支和标签

```bash
git push origin main
git push origin v0.1.0
```

4. 在代码托管平台创建 Release，并附带：

- 版本号：`v0.1.0`
- 发布说明：`CHANGELOG.md` 中的 `v0.1.0`
- 接入文档：`README.md`

## 说明

当前仓库还没有任何 git commit，所以现在不适合直接创建正式 tag。
更合理的顺序是先提交本次 SDK 初始化代码，再打 `v0.1.0`。
