# DepDash

使用 Go + Wails v2 构建的桌面端批量 Git 项目工具。

![示例图片](image.png)

## 功能

### 批量更新依赖
- 选择项目根目录，自动扫描所有 Git 子项目
- 多选项目、多选包，填写目标版本号
- 自动执行：stash → 切换分支 → pull → 修改 package.json → npm install → commit → push
- 失败自动回滚，完成后恢复原分支和本地修改
- 支持自动升版本号（可在设置中配置）

### Cherry-Pick
- 支持 Tab 切换「更新依赖」和「Cherry-Pick」两种模式
- 选择源分支和目标分支，自动拉取远程最新提交
- 获取最近提交列表（显示 hash、提交信息、作者、相对时间）
- 多项目按分组折叠展示，勾选指定 commit
- 逐条 cherry-pick，遇冲突自动 abort，跳过当前项目
- 自动 push 到目标分支

### 通用
- 实时终端输出，所有命令过程可见
- 亮色 / 暗色主题切换
- 启动时检查 GitHub Releases 新版本

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

构建产物位于 `build/bin/depdash.exe`。

## 配置

首次运行自动在 `%APPDATA%/depdash/config.json` 生成配置文件：

```json
{
    "registry": "https://registry.npmmirror.com",
    "branches": ["main", "develop"],
    "packages": [],
    "autoBumpProjects": [],
    "rootPath": "",
    "updateRepo": "ai-written/depdash",
    "commitCount": 5
}
```

| 字段 | 说明 |
|------|------|
| `registry` | npm 镜像源地址 |
| `branches` | 可选的分支列表 |
| `packages` | 常用包名（勾选后填版本号） |
| `autoBumpProjects` | 需要自动升版本号的项目目录名 |
| `rootPath` | 上次使用的项目根目录（自动记忆） |
| `updateRepo` | GitHub 仓库 `user/repo`，用于版本更新检查（留空禁用） |
| `commitCount` | Cherry-Pick 模式每次获取的提交数量（默认 5） |

## 工作流程

### 更新依赖模式
1. 点击 **设置** 配置镜像源、分支和常用包
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

## CI/CD

推送 `v*` 标签自动触发 GitHub Actions 构建和发布：

```powershell
git tag v1.0.0
git push origin v1.0.0
```

## License

[MIT](LICENSE)
