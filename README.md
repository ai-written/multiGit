# MultiGit

使用 Go + Wails v2 构建的桌面端批量 Git 项目工具。

![alt text](image.png)

## 功能

支持 Tab 切换「历史记录」「更新依赖」「Cherry-Pick」「强制检出」四种模式

### 历史记录
- 按项目分组展示任意分支的提交历史，支持滚动分页加载
- 提交列表左侧渲染类似 VSCode 的 git graph 分支图（多车道彩色线条、merge 汇入/分叉曲线、提交圆点），展开文件变更时线条跟随延伸
- 按提交信息、作者名或 commit-id 搜索全量历史
- 每个 commit 可展开查看文件变更列表（A/M/D 状态）
- 文件级侧边对比 diff（左右两栏同步滚动）
- 自动跳过压缩 JS 等大文件的 diff 渲染
- 右键菜单：复制 SHA、查看完整提交信息、VSCode 打开文件、VSCode 对比更改
- 支持显示 git tag 标签
- 分支列表从已选项目中动态获取（本地 + 远程），无需手动配置
- 搜索词全局保留，切换项目自动搜索或复用缓存
- 自动分页加载，加载完成后正确显示结束标记

### 批量更新依赖
- 选择项目根目录，自动扫描所有 Git 子项目
- 多选项目、多选包，填写目标版本号
- 自动执行：stash → 切换分支 → pull → 修改 package.json → npm install → commit → push
- 失败自动回滚，完成后恢复原分支和本地修改
- 支持自动升版本号（可在设置中配置）
- 包版本号支持配置默认值和持久化缓存

### Cherry-Pick
- 选择源分支和目标分支，自动拉取远程最新提交
- 获取最近提交列表（显示 hash、提交信息、作者、相对时间）
- 多项目按分组折叠展示，勾选指定 commit
- 逐条 cherry-pick，遇冲突自动 abort，跳过当前项目
- 自动 push 到目标分支

### 强制检出
- 选择源分支，批量将目标分支强制重置为源分支的最新状态
- 自动 stash，切换源分支并 pull 到最新
- 目标分支存在时自动创建 `xxx-backup` 备份分支
- 强制推送覆盖远程目标分支
- 完成后自动恢复原分支和本地修改

### 备份管理
- 「还原 -backup 备份分支」：将备份分支的状态强制还原回原分支
- 「清理 -backup 备份分支」：一键删除本地和远程所有备份分支

### 通用
- 实时终端输出，所有命令过程可见
- 亮色 / 暗色主题切换
- 启动时检查 GitHub Releases 新版本
- 项目根目录支持下拉切换已保存的路径列表，设置中可编辑
- 项目列表右键菜单：在文件管理器中显示、打开终端、使用 VSCode 打开
- 支持 WSL 路径（`\\wsl.localhost\...`），自动使用 WSL 内部 git/npm/node
- WSL 环境使用 login shell，正确加载 nvm 等用户配置

### 分支对比
- 选择任意两个分支对比 commit 领先/落后数量
- 显示文件级差异统计（新增/修改/删除文件数，新增/删除行数）
- 自动 fallback 到远程跟踪分支，无需本地分支存在

## 技术栈

| 层 | 技术 |
|---|------|
| 桌面框架 | Wails v2 |
| 后端 | Go |
| 前端 | HTML + CSS + vanilla JS |
| UI 风格 | 亮色/暗色双主题，自定义无边框标题栏 |

## 开发

```powershell
# 安装依赖
go mod tidy

# 开发模式（热重载）
wails dev

# 生产构建（含自定义图标 + 版本号）
build.bat
```

构建产物位于 `build/bin/MultiGit.exe`。

## 配置

首次运行自动在 `%APPDATA%/MultiGit/config.json` 生成配置文件：

```json
{
    "registry": "https://registry.npmmirror.com",
    "packages": [],
    "autoBumpProjects": [],
    "rootPath": "",
    "rootPaths": [],
    "updateRepo": "ai-written/multiGit",
    "commitCount": 3,
    "historyCommitCount": 50
}
```

| 字段 | 说明 |
|------|------|
| `registry` | npm 镜像源地址 |
| `packages` | 常用包名，支持 `pkg@version` 配置默认版本号 |
| `packageVersions` | 包的默认版本号映射，如 `{"lodash": "4.0.0"}` |
| `autoBumpProjects` | 需要自动升版本号的项目目录名 |
| `rootPath` | 上次使用的项目根目录（自动记忆） |
| `rootPaths` | 项目根目录列表，可在下拉框中快速切换（逗号分隔） |
| `updateRepo` | GitHub 仓库 `user/repo`，用于版本更新检查（留空禁用） |
| `commitCount` | Cherry-Pick 每次获取的提交数量（默认 3） |
| `historyCommitCount` | 历史记录每次获取的提交数量（默认 50） |

## 工作流程

### 历史记录模式
1. 点击 Tab 切换到 **历史记录**（默认页面）
2. 勾选要查看的项目，分支列表自动从选中项目获取
3. 选择分支，点击 **获取提交**
4. 按项目分组展示 commit，滚动自动加载下一页
5. 搜索框输入关键词实时搜索（搜索提交信息、作者、commit-id）
6. 点击 commit 行展开文件变更列表
7. 点击文件名展开侧边对比 diff
8. 双击 commit 复制完整 hash

### 更新依赖模式
1. 点击 **设置** 配置镜像源和常用包
2. 点击 **浏览** 选择项目根目录
3. 选择目标分支
4. 勾选要更新的项目
5. 勾选要更新的包，填写目标版本号
6. 点击 **执行更新**
7. 右侧终端实时显示执行过程

### Cherry-Pick 模式
1. 点击 Tab 切换到 **Cherry-Pick**
2. 选择源分支
3. 勾选要处理的项目
4. 输入目标分支名（分支必须已存在于本地或远程）
5. 点击 **获取最近提交**，按项目分组显示 commit 列表
6. 勾选需要 cherry-pick 的 commit（支持折叠项目组）
7. 点击 **执行 Cherry-Pick**
8. 自动处理：stash → 切源分支 → 切目标分支 → cherry-pick → push → 恢复

### 强制检出模式
1. 点击 Tab 切换到 **强制检出**
2. 选择源分支
3. 勾选要处理的项目
4. 输入目标分支名（将重置为该分支）
5. 点击 **执行强制检出**
6. 自动处理：stash → 切源分支并 pull → 备份目标分支（若存在）→ 强制检出并推送 → 恢复
7. 如需还原，点击 **还原 -backup 备份分支** 将备份状态写回原分支
8. 备份确认无误后，点击 **清理 -backup 备份分支** 删除备份分支

## CI/CD

推送 `v*` 标签自动触发 GitHub Actions 构建和发布：

```powershell
git tag v1.0.0
git push origin v1.0.0
```

## License

[MIT](LICENSE)
