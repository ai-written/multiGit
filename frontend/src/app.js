import { EventsOn } from '/wailsjs/runtime/runtime.js';
import { WindowMinimise, WindowToggleMaximise, WindowIsMaximised, Quit } from '/wailsjs/runtime/runtime.js';
import { LoadConfig, SaveConfig, SelectDir, ListProjects, UpdatePackage, CheckUpdate, OpenURL, GetRecentCommits, CherryPickCommits, ForceCheckoutBranch, DeleteBackupBranches, RestoreBackupBranches } from '/wailsjs/go/main/App.js';

const $ = (sel) => document.querySelector(sel);
const $$ = (sel) => document.querySelectorAll(sel);

const terminal = $('#terminalContent');

const versionInputCache = new Map();

let currentMode = 'update';

let config = {
    registry: 'https://registry.npmmirror.com',
    branches: ['main', 'develop'],
    packages: [],
    autoBumpProjects: [],
    rootPath: '',
    commitCount: 5
};

function log(text) {
    const div = document.createElement('div');
    div.textContent = text;
    terminal.appendChild(div);
    terminal.scrollTop = terminal.scrollHeight;
}

function logError(text) {
    const div = document.createElement('div');
    div.textContent = text;
    div.style.color = '#ef4444';
    terminal.appendChild(div);
    terminal.scrollTop = terminal.scrollHeight;
}

function showConfirm({ title, message, confirmText, cancelText, variant }) {
    return new Promise((resolve) => {
        const overlay = $('#confirmModal');
        const box = $('#confirmModalBox');
        const titleEl = $('#confirmTitle');
        const msgEl = $('#confirmMessage');
        const okBtn = $('#confirmOk');
        const cancelBtn = $('#confirmCancel');

        titleEl.textContent = title || '确认操作';
        msgEl.innerHTML = message;
        okBtn.textContent = confirmText || '确认';
        cancelBtn.textContent = cancelText || '取消';

        okBtn.className = 'btn';
        if (variant === 'danger') okBtn.classList.add('btn-danger');
        else if (variant === 'warning') okBtn.classList.add('btn-warning');
        else if (variant === 'success') okBtn.classList.add('btn-success');
        else okBtn.classList.add('btn-primary');

        const cleanup = () => {
            overlay.style.display = 'none';
            okBtn.removeEventListener('click', onOk);
            cancelBtn.removeEventListener('click', onCancel);
            overlay.removeEventListener('click', onOverlay);
        };

        const onOk = () => { cleanup(); resolve(true); };
        const onCancel = () => { cleanup(); resolve(false); };
        const onOverlay = (e) => { if (e.target === overlay) { cleanup(); resolve(false); } };

        okBtn.addEventListener('click', onOk);
        cancelBtn.addEventListener('click', onCancel);
        overlay.addEventListener('click', onOverlay);

        overlay.style.display = 'flex';
    });
}

EventsOn('log', (text) => {
    log(text);
});

async function loadAndApplyConfig() {
    try {
        config = await LoadConfig();
        if (config.packageVersionCache) {
            Object.entries(config.packageVersionCache).forEach(([k, v]) => versionInputCache.set(k, v));
        }
        applyConfigToUI();
        if (config.rootPath) {
            $('#rootPath').value = config.rootPath;
            await loadProjects(config.rootPath);
        }
    } catch (e) {
        logError('加载配置失败: ' + e);
    }
}

function applyConfigToUI() {
    const branchSel = $('#branch');
    branchSel.innerHTML = config.branches.map(b => `<option value="${b}">${b}</option>`).join('');

    renderPackageList();
}

function switchMode(mode) {
    currentMode = mode;

    $$('.tab').forEach(t => t.classList.toggle('tab-active', t.dataset.mode === mode));

    $('#updateSection').style.display = 'none';
    $('#cherrypickSection').style.display = 'none';
    $('#forcecheckoutSection').style.display = 'none';

    if (mode === 'update') {
        $('#branchLabel').textContent = '目标分支';
        $('#updateSection').style.display = '';
    } else if (mode === 'cherrypick') {
        $('#branchLabel').textContent = '源分支';
        $('#cherrypickSection').style.display = '';
    } else {
        $('#branchLabel').textContent = '源分支';
        $('#forcecheckoutSection').style.display = '';
    }
}

function renderPackageList() {
    const list = $('#packageList');
    list.innerHTML = config.packages.map(pkg =>
        `<label class="checkbox-item">
            <input type="checkbox" value="${pkg}" class="pkg-checkbox" />
            <span>${pkg}</span>
        </label>`
    ).join('');

    $$('.pkg-checkbox').forEach(cb => {
        cb.addEventListener('change', onPackageChange);
    });
}

function onPackageChange() {
    const container = $('#packageVersions');
    const checked = $$('.pkg-checkbox:checked');

    if (checked.length === 0) {
        container.style.display = 'none';
        container.innerHTML = '';
        return;
    }

    $$('.pkg-version').forEach(input => {
        versionInputCache.set(input.dataset.pkg, input.value);
    });

    container.style.display = 'flex';
    container.innerHTML = '<label class="form-label">输入版本号</label>' +
        Array.from(checked).map(cb =>
            `<div class="version-row">
                <span class="pkg-name">${cb.value}</span>
                <input type="text" class="input pkg-version" data-pkg="${cb.value}" placeholder="输入版本号" />
            </div>`
        ).join('');

    const pkgVersions = config.packageVersions || {};
    $$('.pkg-version').forEach(input => {
        if (versionInputCache.has(input.dataset.pkg)) {
            input.value = versionInputCache.get(input.dataset.pkg);
        } else if (pkgVersions[input.dataset.pkg]) {
            input.value = pkgVersions[input.dataset.pkg];
        }
    });
}

async function browseDir() {
    try {
        const dir = await SelectDir();
        if (dir) {
            $('#rootPath').value = dir;
            config.rootPath = dir;
            await SaveConfig(config);
            await loadProjects(dir);
        }
    } catch (e) {
        logError('选择目录失败: ' + e);
    }
}

async function loadProjects(rootPath) {
    try {
        const projects = await ListProjects(rootPath);
        const list = $('#projectList');
        list.innerHTML = projects.map(p =>
            `<label class="checkbox-item">
                <input type="checkbox" value="${p.path}" data-name="${p.name}" class="proj-checkbox" />
                <span>${p.name}</span>
            </label>`
        ).join('');
    } catch (e) {
        logError('项目列表加载失败: ' + e);
    }
}

async function onSubmit(e) {
    e.preventDefault();

    const selectedProjects = Array.from($$('.proj-checkbox:checked')).map(cb => cb.value);
    if (selectedProjects.length === 0) {
        logError('请至少选择一个项目');
        return;
    }

    const branch = $('#branch').value;
    if (!branch) {
        logError('请选择目标分支');
        return;
    }

    const packages = [];
    let hasEmptyVersion = false;
    $$('.pkg-checkbox:checked').forEach(cb => {
        const versionInput = $(`.pkg-version[data-pkg="${cb.value}"]`);
        const version = versionInput ? versionInput.value.trim() : '';
        if (!version) {
            logError(`请输入 ${cb.value} 的版本号`);
            hasEmptyVersion = true;
            return;
        }
        packages.push(`${cb.value}@${version}`);
    });

    if (hasEmptyVersion) return;
    if (packages.length === 0) {
        logError('请至少选择一个包并填写版本号');
        return;
    }

    terminal.innerHTML = '<span class="terminal-prompt">$ _</span>';

    const btnRun = $('#btnRun');
    btnRun.disabled = true;
    btnRun.textContent = '更新中...';

    try {
        const result = await UpdatePackage(selectedProjects, packages, branch);
        if (result.ok) {
            config.packageVersionCache = Object.fromEntries(versionInputCache);
            await SaveConfig(config);
        } else {
            logError(result.message);
        }
    } catch (e) {
        logError('执行出错: ' + e);
    } finally {
        btnRun.disabled = false;
        btnRun.textContent = '执行更新';
    }
}

function onReset() {
    $$('.proj-checkbox, .pkg-checkbox').forEach(cb => cb.checked = false);
    $('#packageVersions').style.display = 'none';
    $('#packageVersions').innerHTML = '';
    terminal.innerHTML = '<span class="terminal-prompt">$ _</span>';
    versionInputCache.clear();
}

async function onFetchCommits() {
    const selectedProjects = Array.from($$('.proj-checkbox:checked')).map(cb => cb.value);
    if (selectedProjects.length === 0) {
        logError('请至少选择一个项目');
        return;
    }

    const branch = $('#branch').value;
    if (!branch) {
        logError('请选择源分支');
        return;
    }

    const btn = $('#btnFetchCommits');
    btn.disabled = true;
    btn.textContent = '获取中...';

    try {
        const result = await GetRecentCommits(selectedProjects, branch, config.commitCount);
        renderCommitList(result);
    } catch (e) {
        logError('获取提交记录失败: ' + e);
    } finally {
        btn.disabled = false;
        btn.textContent = '获取最近提交';
    }
}

function renderCommitList(projectCommits) {
    const container = $('#commitList');
    if (!projectCommits || projectCommits.length === 0) {
        container.innerHTML = '<div style="padding:8px;color:var(--text-muted);font-size:12px">没有获取到提交记录</div>';
        return;
    }

    container.innerHTML = projectCommits.map((pc, pi) => {
        const commits = [...pc.commits].reverse();
        const total = pc.commits.length;
        const commitsHtml = commits.map((c, ci) =>
            `<label class="commit-item">
                <input type="checkbox" class="commit-checkbox" data-project="${pc.project_path}" data-hash="${c.hash}" data-order="${total - 1 - ci}" />
                <div class="commit-body">
                    <div class="commit-row">
                        <span class="commit-hash">${c.hash.slice(0, 8)}</span>
                        <span class="commit-msg" title="${escapeHtml(c.message)}">${escapeHtml(c.message)}</span>
                    </div>
                    <div class="commit-meta">${escapeHtml(c.author)} &middot; ${escapeHtml(c.date)}</div>
                </div>
            </label>`
        ).join('');

        return `<div class="commit-project">
            <div class="commit-project-header" data-index="${pi}">
                <span class="commit-project-toggle">▼</span>
                <span>${escapeHtml(pc.project_name)} (${pc.commits.length} commits)</span>
            </div>
            <div class="commit-project-body">${commitsHtml}</div>
        </div>`;
    }).join('');

    container.querySelectorAll('.commit-project-header').forEach(h => {
        h.addEventListener('click', () => {
            const body = h.nextElementSibling;
            const toggle = h.querySelector('.commit-project-toggle');
            body.classList.toggle('hidden');
            toggle.classList.toggle('collapsed');
        });
    });
}

async function onForceCheckout() {
    const selectedProjects = Array.from($$('.proj-checkbox:checked')).map(cb => cb.value);
    if (selectedProjects.length === 0) {
        logError('请至少选择一个项目');
        return;
    }

    const sourceBranch = $('#branch').value;
    if (!sourceBranch) {
        logError('请选择源分支');
        return;
    }

    const targetBranch = $('#fcTargetBranch').value.trim();
    if (!targetBranch) {
        logError('请输入目标分支名');
        return;
    }

    const projectNames = Array.from($$('.proj-checkbox:checked')).map(cb => cb.dataset.name);
    const projectList = projectNames.map(n => `&nbsp;&nbsp;• ${escapeHtml(n)}`).join('<br>');

    const ok = await showConfirm({
        title: '强制检出',
        message: `确定要将以下 <strong>${projectNames.length}</strong> 个项目的分支 <strong>${escapeHtml(targetBranch)}</strong> 强制重置为 <strong>${escapeHtml(sourceBranch)}</strong> 的最新状态吗？<br><br>${projectList}<br><br>目标分支存在时将自动备份为 <strong>${escapeHtml(targetBranch)}-backup</strong>，可通过「还原 -backup 备份分支」恢复。`,
        confirmText: '执行强制检出',
        variant: 'danger',
    });
    if (!ok) return;

    terminal.innerHTML = '<span class="terminal-prompt">$ _</span>';

    const btn = $('#btnForceCheckout');
    btn.disabled = true;
    btn.textContent = '执行中...';

    try {
        const result = await ForceCheckoutBranch(selectedProjects, sourceBranch, targetBranch);
        if (!result.ok) {
            logError(result.message);
        }
    } catch (e) {
        logError('执行出错: ' + e);
    } finally {
        btn.disabled = false;
        btn.textContent = '执行强制检出';
    }
}

async function onDeleteBackupBranches() {
    const selectedProjects = Array.from($$('.proj-checkbox:checked')).map(cb => cb.value);
    if (selectedProjects.length === 0) {
        logError('请至少选择一个项目');
        return;
    }

    const projectNames = Array.from($$('.proj-checkbox:checked')).map(cb => cb.dataset.name);
    const projectList = projectNames.map(n => `&nbsp;&nbsp;• ${escapeHtml(n)}`).join('<br>');

    const ok = await showConfirm({
        title: '清理备份分支',
        message: `确定要删除以下项目中所有 <strong>-backup</strong> 结尾的备份分支吗？<br><br>${projectList}<br><br><span style="color:var(--warn)">本地和远程将同时删除</span>`,
        confirmText: '确认删除',
        variant: 'warning',
    });
    if (!ok) return;

    terminal.innerHTML = '<span class="terminal-prompt">$ _</span>';

    const btn = $('#btnDeleteBackup');
    btn.disabled = true;
    btn.textContent = '清理中...';

    try {
        const result = await DeleteBackupBranches(selectedProjects);
        if (result.ok) {
            log(result.message);
        } else {
            logError(result.message);
        }
    } catch (e) {
        logError('执行出错: ' + e);
    } finally {
        btn.disabled = false;
        btn.textContent = '清理 -backup 备份分支';
    }
}

async function onRestoreBackupBranches() {
    const selectedProjects = Array.from($$('.proj-checkbox:checked')).map(cb => cb.value);
    if (selectedProjects.length === 0) {
        logError('请至少选择一个项目');
        return;
    }

    const projectNames = Array.from($$('.proj-checkbox:checked')).map(cb => cb.dataset.name);
    const projectList = projectNames.map(n => `&nbsp;&nbsp;• ${escapeHtml(n)}`).join('<br>');

    const ok = await showConfirm({
        title: '还原备份分支',
        message: `确定要将已选项目中的 <strong>-backup</strong> 备份分支还原到原分支吗？<br><br>${projectList}<br><br><span style="color:var(--warn)">将强制重置原分支为备份分支状态并强制推送！</span>`,
        confirmText: '确认还原',
        variant: 'success',
    });
    if (!ok) return;

    terminal.innerHTML = '<span class="terminal-prompt">$ _</span>';

    const btn = $('#btnRestoreBackup');
    btn.disabled = true;
    btn.textContent = '还原中...';

    try {
        const result = await RestoreBackupBranches(selectedProjects);
        if (result.ok) {
            log(result.message);
        } else {
            logError(result.message);
        }
    } catch (e) {
        logError('执行出错: ' + e);
    } finally {
        btn.disabled = false;
        btn.textContent = '还原 -backup 备份分支';
    }
}

function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

async function onCherryPick() {
    const selectedProjects = Array.from($$('.proj-checkbox:checked')).map(cb => cb.value);
    if (selectedProjects.length === 0) {
        logError('请至少选择一个项目');
        return;
    }

    const sourceBranch = $('#branch').value;
    if (!sourceBranch) {
        logError('请选择源分支');
        return;
    }

    const destBranch = $('#cpDestBranch').value.trim();
    if (!destBranch) {
        logError('请输入目标分支名');
        return;
    }

    const selectedCommits = {};
    let totalChecked = 0;
    $$('.commit-checkbox:checked').forEach(cb => {
        const project = cb.dataset.project;
        const hash = cb.dataset.hash;
        const order = parseInt(cb.dataset.order);
        if (!selectedCommits[project]) {
            selectedCommits[project] = [];
        }
        selectedCommits[project].push({ hash, order });
        totalChecked++;
    });

    if (totalChecked === 0) {
        logError('请至少选择一个 commit');
        return;
    }

    for (const project of Object.keys(selectedCommits)) {
        selectedCommits[project].sort((a, b) => a.order - b.order);
        selectedCommits[project] = selectedCommits[project].map(c => c.hash);
    }

    terminal.innerHTML = '<span class="terminal-prompt">$ _</span>';

    const btn = $('#btnCherryPick');
    btn.disabled = true;
    btn.textContent = '执行中...';

    try {
        const result = await CherryPickCommits(selectedProjects, sourceBranch, selectedCommits, destBranch);
        if (!result.ok) {
            logError(result.message);
        }
    } catch (e) {
        logError('执行出错: ' + e);
    } finally {
        btn.disabled = false;
        btn.textContent = '执行 Cherry-Pick';
    }
}

function openConfig() {
    $('#cfgRegistry').value = config.registry || '';
    $('#cfgBranches').value = (config.branches || []).join(', ');
    const pkgVersions = config.packageVersions || {};
    $('#cfgPackages').value = (config.packages || []).map(p => pkgVersions[p] ? `${p}@${pkgVersions[p]}` : p).join(', ');
    $('#cfgAutoBump').value = (config.autoBumpProjects || []).join(', ');
    $('#cfgCommitCount').value = config.commitCount || 5;
    $('#configModal').style.display = 'flex';
}

function closeConfig() {
    $('#configModal').style.display = 'none';
}

async function saveConfig() {
    config.registry = $('#cfgRegistry').value.trim();
    config.branches = $('#cfgBranches').value.split(',').map(s => s.trim()).filter(Boolean);
    const parsedPkgs = [];
    const parsedVersions = {};
    $('#cfgPackages').value.split(',').map(s => s.trim()).filter(Boolean).forEach(token => {
        const idx = token.indexOf('@');
        if (idx > 0) {
            const name = token.slice(0, idx).trim();
            const version = token.slice(idx + 1).trim();
            if (name) {
                parsedPkgs.push(name);
                if (version) parsedVersions[name] = version;
            }
        } else if (token) {
            parsedPkgs.push(token);
        }
    });
    config.packages = parsedPkgs;
    config.packageVersions = parsedVersions;
    config.autoBumpProjects = $('#cfgAutoBump').value.split(',').map(s => s.trim()).filter(Boolean);
    config.commitCount = parseInt($('#cfgCommitCount').value) || 5;

    try {
        await SaveConfig(config);
        applyConfigToUI();
        closeConfig();
        log('配置已保存');
    } catch (e) {
        logError('保存配置失败: ' + e);
    }
}

$('#btnBrowse').addEventListener('click', browseDir);
$('#btnConfig').addEventListener('click', openConfig);
$('#btnCloseConfig').addEventListener('click', closeConfig);
$('#btnCancelConfig').addEventListener('click', closeConfig);
$('#btnSaveConfig').addEventListener('click', saveConfig);
$('#updateForm').addEventListener('submit', onSubmit);
$('#btnReset').addEventListener('click', onReset);

$$('.tab').forEach(tab => {
    tab.addEventListener('click', () => switchMode(tab.dataset.mode));
});
$('#btnFetchCommits').addEventListener('click', onFetchCommits);
$('#btnCherryPick').addEventListener('click', onCherryPick);
$('#btnForceCheckout').addEventListener('click', onForceCheckout);
$('#btnRestoreBackup').addEventListener('click', onRestoreBackupBranches);
$('#btnDeleteBackup').addEventListener('click', onDeleteBackupBranches);

$('#btnMinimize').addEventListener('click', () => WindowMinimise());
$('#btnMaximize').addEventListener('click', () => {
    WindowToggleMaximise();
    updateMaximizeIcon();
});
$('#btnClose').addEventListener('click', () => Quit());

const isDark = () => document.documentElement.classList.contains('dark');
const themeBtn = $('#btnTheme');

function applyTheme(dark) {
    document.documentElement.classList.toggle('dark', dark);
    themeBtn.innerHTML = dark ? '&#9789;' : '&#9788;';
}

themeBtn.addEventListener('click', () => {
    const next = !isDark();
    applyTheme(next);
    localStorage.setItem('multigit-theme', next ? 'dark' : 'light');
});

const saved = localStorage.getItem('multigit-theme');
applyTheme(saved === 'dark' || (saved === null && window.matchMedia('(prefers-color-scheme: dark)').matches));

window.addEventListener('resize', updateMaximizeIcon);

async function updateMaximizeIcon() {
    try {
        const maximised = await WindowIsMaximised();
        $('#btnMaximize').textContent = maximised ? '\u2750' : '\u25A1';
    } catch {}
}

updateMaximizeIcon();
loadAndApplyConfig();

async function checkUpdate() {
    try {
        const info = await CheckUpdate();
        if (info && info.has_update) {
            const bar = $('#updateBar');
            const text = $('#updateBarText');
            text.textContent = `New version ${info.latest} (current ${info.current}) - click to download`;
            bar.style.display = 'flex';
            bar.onclick = () => { if (info.download_url) OpenURL(info.download_url); };
            $('#updateBarClose').onclick = (e) => { e.stopPropagation(); bar.style.display = 'none'; };
        }
    } catch {}
}
checkUpdate();
