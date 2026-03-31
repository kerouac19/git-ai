# Fork 与上游同步操作指南

本文档介绍如何 fork 本仓库、在 fork 中独立开发 server 功能，并保持与上游仓库同步。

## 1. Fork 并设置远程仓库

```bash
# 方式一：通过 GitHub CLI 创建 fork 并克隆
gh repo fork <原始仓库> --clone
cd git-ai

# 方式二：在 GitHub 上手动 fork 后克隆
git clone git@github.com:<你的用户名>/git-ai.git
cd git-ai
git remote add upstream git@github.com:<原始仓库>/git-ai.git

# 确认远程仓库配置
git remote -v
# origin    git@github.com:<你的用户名>/git-ai.git (fetch)
# upstream  git@github.com:<原始仓库>/git-ai.git (fetch)
```

## 2. 在独立分支上开发 server 功能

```bash
git checkout -b server-feature main

# 在此分支上进行 server 功能开发
# 建议将改动集中在 server/ 目录下，减少与上游的冲突
```

## 3. 同步上游更新

```bash
# 拉取上游最新代码
git fetch upstream

# 将上游更新合并到本地 main
git checkout main
git merge upstream/main

# 推送到你的 fork
git push origin main

# 将上游更新合并到功能分支
git checkout server-feature
git rebase main
# 如遇冲突，解决后执行 git rebase --continue
```

也可以使用 GitHub CLI 一键同步 fork 的 main 分支：

```bash
gh repo sync <你的用户名>/git-ai
```

## 注意事项

- **rebase vs merge**：单人开发功能分支时优先用 `rebase` 保持历史整洁；多人协作时用 `merge` 避免强推
- **减少冲突**：尽量将 server 功能放在独立目录（如 `server/`），避免大量修改上游已有文件
- **同步频率**：建议每周至少同步一次上游，避免分歧过大导致合并困难
- **自动化同步**：可通过 GitHub Actions 定期自动同步 upstream 到 fork 的 main 分支
