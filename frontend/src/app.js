import { EventsOn } from '/wailsjs/runtime/runtime.js';
import { WindowMinimise, WindowToggleMaximise, WindowIsMaximised, Quit } from '/wailsjs/runtime/runtime.js';
import { LoadConfig, SaveConfig, SelectDir, ListProjects, UpdatePackage, CheckUpdate, OpenURL, GetRecentCommits, GetRecentCommitsPage, CherryPickCommits, ForceCheckoutBranch, DeleteBackupBranches, RestoreBackupBranches, GetCommitFiles, GetCommitFileDiffSideBySide, SearchHistoryCommitsPage, ListProjectBranches, GetCommitDetail, OpenInExplorer, OpenInTerminal, OpenInVSCode, OpenInVSCodeDiff, GetBranchDiffStat } from '/wailsjs/go/main/App.js';

const $ = (sel) => document.querySelector(sel);
const $$ = (sel) => document.querySelectorAll(sel);

const terminal = $('#terminalContent');

const versionInputCache = new Map();

let currentMode = 'history';

let config = {
    registry: 'https://registry.npmmirror.com',
    packages: [],
    autoBumpProjects: [],
    rootPath: '',
    commitCount: 3,
    historyCommitCount: 50,
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
        $('#branch').innerHTML = '<option value="">— 请选择项目 —</option>';
        $('#rootPath').addEventListener('change', async () => {
            const val = $('#rootPath').value;
            if (val === '__browse__') {
                await browseDir();
                return;
            }
            if (val) {
                config.rootPath = val;
                await SaveConfig(config);
                await loadProjects(val);
            }
        });
        if (config.rootPath) {
            if ((config.rootPaths || []).includes(config.rootPath)) {
                $('#rootPath').value = config.rootPath;
            }
            await loadProjects(config.rootPath);
        }
    } catch (e) {
        logError('加载配置失败: ' + e);
    }
}

function renderRootPaths() {
    const sel = $('#rootPath');
    const cur = sel.value;
    sel.innerHTML = '<option value="">— 选择项目根目录 —</option>' +
        (config.rootPaths || []).map(p => `<option value="${escapeHtml(p)}">${escapeHtml(p)}</option>`).join('') +
        `<option value="__browse__">── 浏览新目录 ──</option>`;
    if (cur && (config.rootPaths || []).includes(cur)) {
        sel.value = cur;
    } else if (config.rootPath && (config.rootPaths || []).includes(config.rootPath)) {
        sel.value = config.rootPath;
    }
}

function applyConfigToUI() {
    renderRootPaths();
    renderPackageList();
}

function switchMode(mode) {
    currentMode = mode;

    $('.tab-bar').querySelectorAll('.tab').forEach(t => t.classList.toggle('tab-active', t.dataset.mode === mode));

    $('#updateSection').style.display = 'none';
    $('#cherrypickSection').style.display = 'none';
    $('#forcecheckoutSection').style.display = 'none';
    $('#historySection').style.display = 'none';
    $('#terminal').style.display = '';
    $('#historyPanel').style.display = 'none';

    if (mode === 'update') {
        $('#branchLabel').textContent = '目标分支';
        $('#updateSection').style.display = '';
    } else if (mode === 'cherrypick') {
        $('#branchLabel').textContent = '源分支';
        $('#cherrypickSection').style.display = '';
    } else if (mode === 'history') {
        $('#branchLabel').textContent = '分支';
        $('#historySection').style.display = '';
        $('#terminal').style.display = 'none';
        $('#historyPanel').style.display = 'flex';
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
            if (!(config.rootPaths || []).includes(dir)) {
                config.rootPaths = config.rootPaths || [];
                config.rootPaths.push(dir);
            }
            config.rootPath = dir;
            renderRootPaths();
            $('#rootPath').value = dir;
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
        list.querySelectorAll('.proj-checkbox').forEach(cb => {
            cb.addEventListener('change', () => refreshBranches());
        });
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
        // 获取分支列表填充目标分支下拉框
        try {
            const resp = await ListProjectBranches(selectedProjects);
            const sel = $('#cpDestBranch');
            const cur = sel.value;
            const branches = resp.allBranches || [];
            sel.innerHTML = '<option value="">— 请选择目标分支 —</option>' +
                branches.map(b => `<option value="${b}">${b}</option>`).join('');
            if (cur && branches.includes(cur)) {
                sel.value = cur;
            } else {
                const priority = ['dev', 'develop', 'main', 'master'];
                for (const name of priority) {
                    if (branches.includes(name)) {
                        sel.value = name;
                        break;
                    }
                }
            }
        } catch (e) {}
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
        const commits = pc.commits;
        const total = pc.commits.length;
        const commitsHtml = commits.map((c, ci) =>
            `<label class="commit-item">
                <input type="checkbox" class="commit-checkbox" data-project="${pc.project_path}" data-hash="${c.hash}" data-order="${ci}" ${ci === 0 ? 'checked' : ''} />
                <div class="commit-body">
                    <div class="commit-row">
                        <span class="commit-hash">${c.hash.slice(0, 8)}</span>
                        <span class="commit-msg" title="${escapeHtml(c.message)}">${escapeHtml(c.message)}</span>
                    </div>
                    <div class="commit-meta" title="${escapeHtml(c.dateISO)}">${escapeHtml(c.author)} &middot; ${escapeHtml(c.date)}</div>
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

function setHistoryMsg(text, type) {
    const msg = $('#historyPanelMsg');
    if (!text) { msg.style.display = 'none'; return; }
    const color = type === 'error' ? 'var(--error)' : type === 'warn' ? 'var(--warn)' : 'var(--success)';
    msg.style.display = 'block';
    msg.style.background = type === 'error' ? 'rgba(239,68,68,0.1)' : type === 'warn' ? 'rgba(245,158,11,0.1)' : 'rgba(34,197,94,0.1)';
    msg.style.color = color;
    msg.style.border = '1px solid ' + color;
    msg.textContent = text;
}

async function refreshBranches() {
    const selected = Array.from($$('.proj-checkbox:checked')).map(cb => cb.value);
    const branchSel = $('#branch');
    if (selected.length === 0) {
        branchSel.innerHTML = '<option value="">— 请选择项目 —</option>';
        return;
    }
    try {
        const resp = await ListProjectBranches(selected);
        const current = branchSel.value;
        const options = (resp.allBranches || []).map(b => {
            const count = (resp.branchCounts || {})[b] || 0;
            const label = count === resp.totalProjects ? b : `${b} (${count}/${resp.totalProjects})`;
            return `<option value="${b}">${label}</option>`;
        }).join('');
        if (options) {
            branchSel.innerHTML = options;
            if (current && resp.allBranches.includes(current)) {
                branchSel.value = current;
            } else {
                const priority = ['dev', 'develop', 'main', 'master'];
                for (const name of priority) {
                    if (resp.allBranches.includes(name)) {
                        branchSel.value = name;
                        break;
                    }
                }
            }
        }
    } catch (e) {}
}

async function onFetchHistory() {
    const selectedProjects = Array.from($$('.proj-checkbox:checked')).map(cb => cb.value);
    if (selectedProjects.length === 0) {
        logError('请至少选择一个项目');
        return;
    }

    const branch = $('#branch').value;
    if (!branch) {
        logError('请选择分支');
        return;
    }

    const btn = $('#btnFetchHistory');
    btn.disabled = true;
    btn.textContent = '获取中...';

    try {
        const pageSize = (config.historyCommitCount || 50);
        const searchPageSize = pageSize;
        const firstPage = await GetRecentCommitsPage(selectedProjects, branch, pageSize, 0);
        if (firstPage && firstPage.length > 0) {
            showHistoryPanel(selectedProjects, branch, pageSize, searchPageSize, firstPage);
            setHistoryMsg('');
        } else {
            setHistoryMsg('没有获取到提交记录', 'warn');
        }
    } catch (e) {
        setHistoryMsg('获取提交记录失败: ' + e, 'error');
    } finally {
        btn.disabled = false;
        btn.textContent = '获取提交';
    }
}

function showHistoryPanel(projectPaths, branch, pageSize, searchPageSize, firstPage) {
    const tabs = $('#historyPanelTabs');
    let content = $('#historyPanelContent');
    let searchInput = $('#historyPanelSearch');

    // 移除旧监听器：替换 content 与 searchInput 元素（否则 input 监听器会跨次调用累积泄漏）
    const newContent = content.cloneNode(false);
    content.parentNode.replaceChild(newContent, content);
    content = newContent;
    const newSearch = searchInput.cloneNode(false);
    searchInput.parentNode.replaceChild(newSearch, searchInput);
    searchInput = newSearch;
    searchInput.value = '';
    tabs.innerHTML = '';
    setHistoryMsg('');

    const s = {
        allCommits: new Map(),
        loading: false,
        currentTabIndex: 0,
        isSearchMode: false,
        searchResults: new Map(),
        lastSearchTerm: '',
    };

    let projectSkip = {};
    let projectAllLoaded = {};
    let projectSearchSkip = {};
    let projectSearchAllLoaded = {};

    function curPath() { return projectPaths[s.currentTabIndex]; }
    function normalizePath(p) { return p.replace(/\\/g, '/'); }
    function getSkip() { return projectSkip[normalizePath(curPath())] || 0; }
    function incSkip(d) { projectSkip[normalizePath(curPath())] = getSkip() + d; }
    function isAllLoaded() { return !!projectAllLoaded[normalizePath(curPath())]; }
    function setAllLoaded(v) { projectAllLoaded[normalizePath(curPath())] = v !== false; }
    function getSearchSkip() { return projectSearchSkip[normalizePath(curPath())] || 0; }
    function setSearchSkip(v) { projectSearchSkip[normalizePath(curPath())] = v; }
    function incSearchSkip(d) { projectSearchSkip[normalizePath(curPath())] = getSearchSkip() + d; }
    function isSearchAllLoaded() { return !!projectSearchAllLoaded[normalizePath(curPath())]; }
    function setSearchAllLoaded(v) { projectSearchAllLoaded[normalizePath(curPath())] = v !== false; }

    let savedScrollTops = {};

    firstPage.forEach(pc => {
        s.allCommits.set(pc.project_path, [...pc.commits]);
    });
    firstPage.forEach(pc => {
        if (pc.commits.length < pageSize) {
            projectAllLoaded[normalizePath(pc.project_path)] = true;
        }
    });

    window._historyState = s;
    window._historyCurPath = curPath;

    function updateTabCounts(primarySource) {
        tabs.querySelectorAll('.tab').forEach(tab => {
            if (tab.dataset.projectIndex === undefined) return;
            const idx = parseInt(tab.dataset.projectIndex);
            const path = projectPaths[idx];
            const count = primarySource.has(path)
                ? (primarySource.get(path) || []).length
                : (s.allCommits.get(path) || []).length;
            const name = firstPage.find(p => p.project_path === path)?.project_name || path.split('/').pop();
            tab.innerHTML = `${escapeHtml(name)} (<span class="tab-count">${count}</span>)`;
        });
    }

    const LANE_COLORS = ['#e74c3c','#3498db','#2ecc71','#f39c12','#9b59b6','#1abc9c','#e67e22','#34495e'];
    const GRAPH_DOT_Y = 9;
    const GRAPH_CURVE = 6;
    const LANE_W = 14;
    const GRAPH_X_OFFSET = 4;

    function nearestEmpty(lanes, from) {
        let best = -1, bestDist = Infinity;
        for (let i = 0; i < lanes.length; i++) {
            if (lanes[i] === null) {
                const d = Math.abs(i - from);
                if (d < bestDist) { bestDist = d; best = i; }
            }
        }
        return best;
    }

    function computeGraph(commits) {
        const lanes = [];
        return commits.map(c => {
            const hash = c.hash;
            const parents = c.parents || [];
            const incoming = [];
            for (let i = 0; i < lanes.length; i++) if (lanes[i] !== null) incoming.push(i);
            const arriving = [];
            for (let i = 0; i < lanes.length; i++) if (lanes[i] === hash) arriving.push(i);
            let dotCol;
            const mergeIn = [];
            if (arriving.length > 0) {
                dotCol = arriving[0];
                for (let j = 1; j < arriving.length; j++) mergeIn.push(arriving[j]);
                for (const m of mergeIn) lanes[m] = null;
            } else {
                dotCol = lanes.indexOf(null);
                if (dotCol === -1) { lanes.push(null); dotCol = lanes.length - 1; }
            }
            const spawnOut = [];
            if (parents.length === 0) {
                lanes[dotCol] = null;
            } else {
                lanes[dotCol] = parents[0];
                for (let k = 1; k < parents.length; k++) {
                    let slot = nearestEmpty(lanes, dotCol);
                    if (slot === -1) { lanes.push(parents[k]); slot = lanes.length - 1; }
                    else lanes[slot] = parents[k];
                    spawnOut.push(slot);
                }
            }
            const outgoing = [];
            for (let i = 0; i < lanes.length; i++) if (lanes[i] !== null) outgoing.push(i);
            return { dotCol, mergeIn, spawnOut, incoming, outgoing };
        });
    }

    function renderGraphCell(row, laneX, graphW) {
        const color = (col) => LANE_COLORS[col % LANE_COLORS.length];
        const dotX = laneX(row.dotCol);
        let paths = '';
        for (const col of row.incoming) {
            const fx = laneX(col);
            if (row.mergeIn.includes(col)) {
                paths += `<path d="M${fx} 0 C${fx} ${GRAPH_DOT_Y - GRAPH_CURVE} ${dotX} ${GRAPH_DOT_Y - GRAPH_CURVE} ${dotX} ${GRAPH_DOT_Y}" stroke="${color(col)}" stroke-width="2" fill="none"/>`;
            } else {
                paths += `<path d="M${fx} 0 L${fx} ${GRAPH_DOT_Y}" stroke="${color(col)}" stroke-width="2" fill="none"/>`;
            }
        }
        for (const col of row.outgoing) {
            const cx = laneX(col);
            if (row.spawnOut.includes(col)) {
                paths += `<path d="M${dotX} ${GRAPH_DOT_Y} C${dotX} ${GRAPH_DOT_Y + GRAPH_CURVE} ${cx} ${GRAPH_DOT_Y + GRAPH_CURVE} ${cx} ${GRAPH_DOT_Y + GRAPH_CURVE} L${cx} 10000" stroke="${color(col)}" stroke-width="2" fill="none"/>`;
            } else {
                paths += `<path d="M${cx} ${GRAPH_DOT_Y} L${cx} 10000" stroke="${color(col)}" stroke-width="2" fill="none"/>`;
            }
        }
        const dot = `<circle cx="${dotX}" cy="${GRAPH_DOT_Y}" r="5" fill="${color(row.dotCol)}" style="stroke:var(--bg);stroke-width:2"/>`;
        return `<div class="commit-graph" style="width:${graphW}px"><svg style="position:absolute;left:0;top:0;width:100%;height:100%;overflow:hidden;display:block">${paths}${dot}</svg></div>`;
    }

    function renderProjectCommits(commits, projectPath) {
        const isSearch = s.isSearchMode;
        const loaded = isSearch ? isSearchAllLoaded() : isAllLoaded();
        const hasMore = !loaded && !s.loading;

        let graphRows = null;
        if (!isSearch) {
            graphRows = computeGraph(commits);
        }
        const laneX = (col) => GRAPH_X_OFFSET + col * LANE_W + LANE_W / 2;
        const rowGraphW = (row) => {
            if (!row) return 20;
            let m = row.dotCol + 1;
            for (const c of row.incoming) if (c + 1 > m) m = c + 1;
            for (const c of row.outgoing) if (c + 1 > m) m = c + 1;
            return Math.max(20, GRAPH_X_OFFSET + (m - 1) * LANE_W + LANE_W / 2 + 6);
        };

        content.innerHTML = commits.map((c, i) => {
            const graph = isSearch ? '' : renderGraphCell(graphRows[i], laneX, rowGraphW(graphRows[i]));
            return `<div class="commit-item" data-hash="${c.hash}" data-project="${escapeHtml(projectPath)}" style="padding:4px 8px 4px 4px">
                ${graph}
                <div class="commit-body">
                    <div style="width:100%;display:flex;align-items:center;gap:4px;font-size:12px">
                        <span class="commit-toggle" style="flex-shrink:0;width:14px;text-align:center;color:var(--text-muted);font-size:10px">▶</span>
                        <span style="flex:1;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:var(--text)" title="${escapeHtml(c.message)}">${escapeHtml(c.message)}</span>
                        ${(c.tags || []).map(t => `<span style="display:inline-block;padding:0 5px;margin:0 2px;font-size:10px;border-radius:3px;background:rgba(124,58,237,0.15);color:var(--primary);white-space:nowrap">${escapeHtml(t)}</span>`).join('')}
                        <span style="flex-shrink:0;color:var(--text-muted);white-space:nowrap" title="${escapeHtml(c.dateISO)}">${escapeHtml(c.author)} &middot; ${escapeHtml(c.date)}</span>
                    </div>
                    <div class="commit-files" style="display:none;padding:2px 0 2px 18px;width:100%;font-size:11px;font-family:var(--font-mono);line-height:1.8"></div>
                </div>
            </div>`;
        }).join('') || '<div style="padding:8px;color:var(--text-muted);font-size:12px">没有提交记录</div>';

        if (hasMore) {
            const indicator = document.createElement('div');
            indicator.className = 'scroll-loading';
            indicator.style.cssText = 'padding:8px;text-align:center;color:var(--text-muted);font-size:12px';
            indicator.textContent = '加载更多...';
            content.appendChild(indicator);
        } else if (loaded && commits.length > 0) {
            const end = document.createElement('div');
            end.style.cssText = 'padding:8px;text-align:center;color:var(--text-muted);font-size:11px';
            end.textContent = isSearch ? '— 搜索结果已全部加载 —' : '— 已显示全部 —';
            content.appendChild(end);
        }

        content.querySelectorAll('.commit-item').forEach(row => {
            let clickTimer = null;
            row.addEventListener('click', (e) => {
                if (e.target.closest('.commit-files')) return;
                e.stopPropagation();
                const ctx = document.querySelector('.ctx-menu');
                if (ctx) ctx.remove();
                if (clickTimer) {
                    clearTimeout(clickTimer);
                    clickTimer = null;
                    return;
                }
                clickTimer = setTimeout(async () => {
                    clickTimer = null;
                    const item = row;
                    content.querySelectorAll('.commit-item.active').forEach(el => el.classList.remove('active'));
                    item.classList.add('active');
                    const filesDiv = item.querySelector('.commit-files');
                if (filesDiv.style.display === 'none') {
                    if (!filesDiv.dataset.loaded) {
                        filesDiv.innerHTML = '<div class="diff-spinner"></div>';
                        try {
                            const files = await GetCommitFiles(item.dataset.project, item.dataset.hash);
                            if (files && files.length > 0) {
                                filesDiv.innerHTML = files.map(f => {
                                    const color = f.status === 'A' ? 'var(--success)' : f.status === 'M' ? 'var(--primary)' : 'var(--error)';
                                    return `<div class="file-item" data-file="${escapeHtml(f.filePath)}">
                                        <span class="file-toggle" style="cursor:pointer">▶ </span><span style="color:${color};margin-right:8px">${f.status}</span><span class="file-path" style="cursor:pointer">${escapeHtml(f.filePath)}</span>
                                        <div class="file-diff" style="display:none;padding:4px 0;font-size:11px;line-height:1.6;white-space:pre;overflow-x:auto;user-select:text"></div>
                                    </div>`;
                                }).join('');
                                filesDiv.querySelectorAll('.file-item').forEach(el => {
                                    function createDiffRow(container, num, content, isDel, isAdd) {
                                        const row = document.createElement('div');
                                        row.className = 'diff-row';
                                        const n = document.createElement('span');
                                        n.className = 'diff-num';
                                        n.textContent = num > 0 ? String(num) : '';
                                        const c = document.createElement('span');
                                        c.className = 'diff-content' + (isDel ? ' diff-content-del' : '') + (isAdd ? ' diff-content-add' : '');
                                        c.textContent = content;
                                        row.appendChild(n);
                                        row.appendChild(c);
                                        container.appendChild(row);
                                    }

                                    function renderSideBySide(hunks) {
                                        const container = document.createElement('div');
                                        container.className = 'diff-container';
                                        let syncing = false;

                                        const header = document.createElement('div');
                                        header.className = 'diff-header';
                                        ['旧文件', '新文件'].forEach((text, i) => {
                                            const col = document.createElement('div');
                                            col.className = 'diff-header-col';
                                            col.textContent = text;
                                            header.appendChild(col);
                                        });
                                        container.appendChild(header);

                                        const scrollWrapper = document.createElement('div');
                                        scrollWrapper.className = 'diff-scrollwrap';

                                        const leftWrap = document.createElement('div');
                                        leftWrap.className = 'diff-col';
                                        const rightWrap = document.createElement('div');
                                        rightWrap.className = 'diff-col';

                                        const left = document.createElement('div');
                                        const right = document.createElement('div');
                                        leftWrap.appendChild(left);
                                        rightWrap.appendChild(right);

                                        let firstHunk = true;
                                        hunks.forEach(hunk => {
                                            if (!firstHunk) {
                                                const oldEnd = hunk.lines[0].oldNum + hunk.lines.filter(l => l.type !== 'add').length - 1;
                                                const newEnd = hunk.lines[0].newNum + hunk.lines.filter(l => l.type !== 'delete').length - 1;
                                                const sepL = document.createElement('div');
                                                sepL.className = 'diff-sep';
                                                sepL.textContent = '... ' + hunk.lines[0].oldNum + '-' + oldEnd;
                                                left.appendChild(sepL);
                                                const sepR = document.createElement('div');
                                                sepR.className = 'diff-sep';
                                                sepR.textContent = '... ' + hunk.lines[0].newNum + '-' + newEnd;
                                                right.appendChild(sepR);
                                            }
                                            firstHunk = false;

                                            hunk.lines.forEach(dl => {
                                                if (dl.type === 'context') {
                                                    createDiffRow(left, dl.oldNum, dl.oldLine);
                                                    createDiffRow(right, dl.newNum, dl.newLine);
                                                } else if (dl.type === 'delete') {
                                                    createDiffRow(left, dl.oldNum, dl.oldLine, true);
                                                    createDiffRow(right, 0, '', false, false);
                                                } else if (dl.type === 'add') {
                                                    createDiffRow(left, 0, '');
                                                    createDiffRow(right, dl.newNum, dl.newLine, false, true);
                                                }
                                            });
                                        });

                                        leftWrap.addEventListener('scroll', () => {
                                            if (syncing) return; syncing = true;
                                            rightWrap.scrollTop = leftWrap.scrollTop;
                                            syncing = false;
                                        });
                                        rightWrap.addEventListener('scroll', () => {
                                            if (syncing) return; syncing = true;
                                            leftWrap.scrollTop = rightWrap.scrollTop;
                                            syncing = false;
                                        });

                                        scrollWrapper.appendChild(leftWrap);
                                        scrollWrapper.appendChild(rightWrap);
                                        container.appendChild(scrollWrapper);
                                        return container;
                                    }

                                    async function toggleFileDiff(e) {
                                        const diffDiv = el.querySelector('.file-diff');
                                        const toggle = el.querySelector('.file-toggle');
                                        if (diffDiv.style.display === 'none') {
                                            if (!diffDiv.dataset.loaded) {
                                                diffDiv.innerHTML = '<div class="diff-spinner"></div>';
                                                try {
                                                    const resp = await GetCommitFileDiffSideBySide(item.dataset.project, item.dataset.hash, el.dataset.file);
                                                    if (resp.tooLarge) {
                                                        diffDiv.innerHTML = '<div style="padding:12px;text-align:center;color:var(--warn);font-size:12px">⚠ 文件过大，已跳过 diff 加载</div>';
                                                    } else if (resp.hunks && resp.hunks.length > 0) {
                                                        diffDiv.innerHTML = '';
                                                        diffDiv.appendChild(renderSideBySide(resp.hunks));
                                                    } else {
                                                        diffDiv.innerHTML = '<div style="padding:8px;text-align:center;color:var(--text-muted);font-size:11px">(无差异)</div>';
                                                    }
                                                    diffDiv.dataset.loaded = '1';
                                                } catch (e) {
                                                    diffDiv.innerHTML = '<div style="padding:8px;text-align:center;color:var(--error);font-size:11px">获取失败</div>';
                                                }
                                            }
                                            diffDiv.style.display = '';
                                            toggle.textContent = '▼ ';
                                        } else {
                                            diffDiv.style.display = 'none';
                                            toggle.textContent = '▶ ';
                                        }
                                    }
                                    el.addEventListener('click', (e) => {
                                        e.stopPropagation();
                                        const ctx = document.querySelector('.ctx-menu');
                                        if (ctx) ctx.remove();
                                        if (e.target.closest('.file-diff')) return;
                                        toggleFileDiff(e);
                                    });
                                    el.addEventListener('contextmenu', (e) => {
                                        e.stopPropagation();
                                        e.preventDefault();
                                        const existing = document.querySelector('.ctx-menu');
                                        if (existing) existing.remove();
                                        const menu = document.createElement('div');
                                        menu.className = 'ctx-menu';
                                        menu.style.cssText = 'position:fixed;z-index:200;background:var(--surface);border:1px solid var(--border);border-radius:var(--radius);padding:4px 0;box-shadow:0 4px 12px rgba(0,0,0,0.15);font-size:12px;min-width:160px';
                                        menu.style.left = e.clientX + 'px';
                                        menu.style.top = e.clientY + 'px';
                                        const fullPath = item.dataset.project + '/' + el.dataset.file;
                                        [
                                            ['📝 使用 VSCode 打开文件', () => OpenInVSCode(fullPath)],
                                            ['📊 使用 VSCode 对比更改', () => OpenInVSCodeDiff(item.dataset.project, item.dataset.hash, el.dataset.file)],
                                            ['📁 在文件管理器中显示', () => OpenInExplorer(fullPath)],
                                        ].forEach(([text, fn]) => {
                                            const itemEl = document.createElement('div');
                                            itemEl.style.cssText = 'padding:6px 14px;cursor:pointer;color:var(--text);transition:background 0.1s';
                                            itemEl.textContent = text;
                                            itemEl.addEventListener('mouseenter', () => itemEl.style.background = 'color-mix(in srgb, var(--primary) 10%, transparent)');
                                            itemEl.addEventListener('mouseleave', () => itemEl.style.background = '');
                                            itemEl.addEventListener('click', () => { fn(); menu.remove(); });
                                            menu.appendChild(itemEl);
                                        });
                                        document.body.appendChild(menu);
                                    });
                                });
                            } else {
                                filesDiv.innerHTML = '<span style="color:var(--text-muted)">(无文件变更)</span>';
                            }
                            filesDiv.dataset.loaded = '1';
                        } catch (e) {
                            filesDiv.innerHTML = '<span style="color:var(--error)">获取失败</span>';
                        }
                    }
                    filesDiv.style.display = '';
                    row.querySelector('.commit-toggle').textContent = '▼';
                } else {
                    filesDiv.style.display = 'none';
                    row.querySelector('.commit-toggle').textContent = '▶';
                }
            });
        });
    });
    }

    async function loadMore() {
        if (s.loading) return;
        const cp = () => projectPaths[s.currentTabIndex];
        if (s.isSearchMode && isSearchAllLoaded()) {
            renderProjectCommits(s.searchResults.get(cp()) || [], cp());
            updateTabCounts(s.searchResults);
            return;
        }
        if (!s.isSearchMode && isAllLoaded()) {
            renderProjectCommits(s.allCommits.get(cp()) || [], cp());
            return;
        }

        s.loading = true;

        if (s.isSearchMode) {
            const myGen = ++searchGen;
            const myPath = projectPaths[s.currentTabIndex];
            incSearchSkip(searchPageSize);
            try {
                const searchResp = await SearchHistoryCommitsPage([myPath], branch, s.lastSearchTerm, searchPageSize, getSearchSkip());
                if (myGen !== searchGen) return;
                projectSearchAllLoaded[normalizePath(myPath)] = !searchResp.hasMore;
                (searchResp.results || []).forEach(pc => {
                    const existing = s.searchResults.get(pc.project_path) || [];
                    s.searchResults.set(pc.project_path, existing.concat(pc.commits));
                });
                if (myPath !== projectPaths[s.currentTabIndex]) return;
                renderProjectCommits(s.searchResults.get(myPath) || [], myPath);
                updateTabCounts(s.searchResults);
            } catch (e) {
                if (myGen === searchGen) {
                    projectSearchAllLoaded[normalizePath(myPath)] = true;
                }
            }
        } else {
            incSkip(pageSize);
            try {
                const currentPath = projectPaths[s.currentTabIndex];
                const nextPage = await GetRecentCommitsPage([currentPath], branch, pageSize, getSkip());
                if (!nextPage || nextPage.length === 0 || nextPage.every(pc => pc.commits.length === 0)) {
                    setAllLoaded(true);
                    renderProjectCommits(s.allCommits.get(currentPath) || [], currentPath);
                } else {
                    nextPage.forEach(pc => {
                        const existing = s.allCommits.get(pc.project_path) || [];
                        s.allCommits.set(pc.project_path, existing.concat(pc.commits));
                        if (pc.commits.length < pageSize) {
                            projectAllLoaded[normalizePath(pc.project_path)] = true;
                        }
                    });
                    renderProjectCommits(s.allCommits.get(currentPath) || [], currentPath);
                    updateTabCounts(s.allCommits);
                }
            } catch (e) {
                setAllLoaded(true);
            }
        }
        s.loading = false;
    }

    async function fillContent() {
        if (s.loading) return;
        if (isAllLoaded()) return;
        if (content.scrollHeight <= content.clientHeight + 50) {
            await loadMore();
        }
    }

    content.addEventListener('scroll', () => {
        if (content.scrollTop + content.clientHeight >= content.scrollHeight - 150) {
            loadMore();
        }
    });

    let searchTimeout;
    let searchGen = 0;

    async function triggerSearch(term) {
        if (!term) return;
        s.isSearchMode = true;
        setSearchSkip(0);
        setSearchAllLoaded(false);
        s.lastSearchTerm = term;
        const myGen = ++searchGen;
        const myPath = projectPaths[s.currentTabIndex];
        content.innerHTML = '<div class="diff-spinner"></div><div style="padding:4px;text-align:center;color:var(--text-muted);font-size:12px">搜索中...</div>';
        try {
            const searchResp = await SearchHistoryCommitsPage([myPath], branch, term, searchPageSize, 0);
            if (myGen !== searchGen) return;
            projectSearchAllLoaded[normalizePath(myPath)] = !searchResp.hasMore;
            (searchResp.results || []).forEach(pc => {
                s.searchResults.set(pc.project_path, pc.commits);
            });
            if (myPath !== projectPaths[s.currentTabIndex]) return;
            renderProjectCommits(s.searchResults.get(myPath) || [], myPath);
            updateTabCounts(s.searchResults);
        } catch (e) {
            if (myGen === searchGen && myPath === projectPaths[s.currentTabIndex]) {
                content.innerHTML = '<div style="padding:8px;text-align:center;color:var(--error);font-size:12px">搜索失败</div>';
            }
        }
    }

    searchInput.addEventListener('input', () => {
        clearTimeout(searchTimeout);
        ++searchGen;
        const term = searchInput.value.trim();
        if (term) {
            searchTimeout = setTimeout(() => triggerSearch(term), 300);
        } else {
            s.isSearchMode = false;
            setSearchSkip(0);
            setSearchAllLoaded(false);
            s.searchResults = new Map();
            s.lastSearchTerm = '';
            setHistoryMsg('');
            renderProjectCommits(s.allCommits.get(projectPaths[s.currentTabIndex]) || [], projectPaths[s.currentTabIndex]);
            updateTabCounts(s.allCommits);
        }
    });

    renderProjectCommits(s.allCommits.get(projectPaths[0]) || [], projectPaths[0]);
    setTimeout(() => fillContent(), 200);

    tabs.innerHTML = projectPaths.map((path, i) => {
        const pc = firstPage.find(p => p.project_path === path);
        return `<button type="button" class="tab ${i === 0 ? 'tab-active' : ''}" data-project-index="${i}">${escapeHtml(pc ? pc.project_name : path.split('/').pop())} (${(s.allCommits.get(path) || []).length})</button>`;
    }).join('');

    tabs.querySelectorAll('.tab').forEach(tab => {
        tab.addEventListener('click', () => {
            ++searchGen;
            savedScrollTops[curPath()] = content.scrollTop;

            tabs.querySelectorAll('.tab').forEach(t => t.classList.remove('tab-active'));
            tab.classList.add('tab-active');
            s.currentTabIndex = parseInt(tab.dataset.projectIndex);

            const term = searchInput.value.trim();
            setHistoryMsg('');

            if (term) {
                if (s.searchResults.has(curPath())) {
                    renderProjectCommits(s.searchResults.get(curPath()) || [], curPath());
                } else {
                    triggerSearch(term);
                }
            } else {
                s.isSearchMode = false;
                setSearchSkip(0);
                setSearchAllLoaded(false);
                s.searchResults = new Map();
                s.lastSearchTerm = '';
                renderProjectCommits(s.allCommits.get(curPath()) || [], curPath());
                updateTabCounts(s.allCommits);
            }

            setTimeout(() => {
                if (!term) fillContent();
                const saved = savedScrollTops[curPath()];
                if (saved !== undefined) content.scrollTop = saved;
            }, 200);
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

    const destBranch = $('#cpDestBranch').value;
    if (!destBranch) {
        logError('请选择目标分支');
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
    $('#cfgRootPaths').value = (config.rootPaths || []).join(', ');
    const pkgVersions = config.packageVersions || {};
    $('#cfgPackages').value = (config.packages || []).map(p => pkgVersions[p] ? `${p}@${pkgVersions[p]}` : p).join(', ');
    $('#cfgAutoBump').value = (config.autoBumpProjects || []).join(', ');
    $('#cfgCommitCount').value = config.commitCount || 3;
    $('#cfgHistoryCount').value = config.historyCommitCount || 50;
    $('#configModal').style.display = 'flex';
}

function closeConfig() {
    $('#configModal').style.display = 'none';
}

async function saveConfig() {
    config.registry = $('#cfgRegistry').value.trim();
    config.rootPaths = $('#cfgRootPaths').value.split(',').map(s => s.trim()).filter(Boolean);
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
    config.commitCount = parseInt($('#cfgCommitCount').value) || 3;
    config.historyCommitCount = parseInt($('#cfgHistoryCount').value) || 50;

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
$('#btnFetchHistory').addEventListener('click', onFetchHistory);
$('#btnBranchCompare').addEventListener('click', async () => {
    const selected = Array.from($$('.proj-checkbox:checked')).map(cb => cb.value);
    const branch = $('#branch').value;
    if (selected.length === 0 || !branch) {
        logError('请先选择项目和分支');
        return;
    }
    try {
        const resp = await ListProjectBranches(selected);
        const branches = resp.allBranches || [];
        const overlay = document.createElement('div');
        overlay.style.cssText = 'position:fixed;inset:0;background:rgba(0,0,0,0.5);display:flex;align-items:center;justify-content:center;z-index:200';
        const box = document.createElement('div');
        box.style.cssText = 'background:var(--surface);border:1px solid var(--border);border-radius:var(--radius-lg);width:480px;max-height:80vh;overflow-y:auto;box-shadow:0 20px 60px rgba(0,0,0,0.5)';
        box.innerHTML =
            '<div style="padding:14px 18px;border-bottom:1px solid var(--border);font-size:14px;font-weight:600;color:var(--text)">🔀 分支对比</div>' +
            '<div style="padding:14px 18px">' +
            '<div style="display:flex;gap:8px;margin-bottom:12px">' +
            '<select id="bcBranchA" class="input">' + branches.map(b => `<option value="${b}" ${b === branch ? 'selected' : ''}>${b}</option>`).join('') + '</select>' +
            '<span style="line-height:32px;color:var(--text-muted)">vs</span>' +
            '<select id="bcBranchB" class="input">' + branches.map(b => `<option value="${b}" ${b !== branch ? 'selected' : ''}>${b}</option>`).join('') + '</select>' +
            '</div>' +
            '<button class="btn btn-primary" id="bcDoCompare" style="width:100%">对比</button>' +
            '<div id="bcResult" style="margin-top:12px"></div>' +
            '</div>' +
            '<div style="padding:10px 18px;border-top:1px solid var(--border);display:flex;justify-content:flex-end">' +
            '<button class="btn btn-secondary" style="padding:6px 18px;font-size:12px">关闭</button></div>';
        box.querySelector('#bcDoCompare').addEventListener('click', async () => {
            const a = box.querySelector('#bcBranchA').value;
            const b = box.querySelector('#bcBranchB').value;
            if (!a || !b) return;
            const result = box.querySelector('#bcResult');
            result.innerHTML = '<div style="padding:8px;text-align:center;color:var(--text-muted);font-size:12px">对比中...</div>';
            try {
                // Compare for each selected project
                let stat = null;
                for (const p of selected) {
                    const s = await GetBranchDiffStat(p, a, b);
                    if (s) { stat = s; break; }
                }
                if (!stat) { result.innerHTML = '<div style="padding:8px;text-align:center;color:var(--error);font-size:12px">对比失败</div>'; return; }
                function ic(n) { return '<span style="display:inline-block;width:24px;text-align:center;flex-shrink:0">' + n + '</span>'; }
                result.innerHTML =
                    '<div style="font-size:12px;line-height:1.8">' +
                    `<div>${ic('🔺')} <strong>${escapeHtml(a)}</strong> 领先 <strong>${escapeHtml(b)}</strong>: <span style="color:var(--error)">+${stat.commitsAhead}</span> 提交</div>` +
                    `<div>${ic('🔻')} <strong>${escapeHtml(b)}</strong> 领先 <strong>${escapeHtml(a)}</strong>: <span style="color:var(--success)">+${stat.commitsBehind}</span> 提交</div>` +
                    '<div style="border-top:1px solid var(--border);margin:6px 0;padding-top:6px">' +
                    `<div>${ic('📄')} 新增文件: ${stat.filesAdded}</div>` +
                    `<div>${ic('📝')} 修改文件: ${stat.filesModified}</div>` +
                    `<div>${ic('🗑')} 删除文件: ${stat.filesDeleted}</div>` +
                    `<div style="margin-top:4px">${ic('➕')} 新增行: <span style="color:var(--success)">+${stat.linesAdded}</span></div>` +
                    `<div>${ic('➖')} 删除行: <span style="color:var(--error)">-${stat.linesDeleted}</span></div>` +
                    '</div></div>';
            } catch (e) { result.innerHTML = '<div style="padding:8px;text-align:center;color:var(--error);font-size:12px">对比失败</div>'; }
        });
        box.querySelectorAll('.btn-secondary').forEach(b => b.addEventListener('click', () => overlay.remove()));
        overlay.addEventListener('click', (e) => { if (e.target === overlay) overlay.remove(); });
        overlay.appendChild(box);
        document.body.appendChild(overlay);
    } catch (e) { logError('获取分支列表失败: ' + e); }
});

$('#btnStats').addEventListener('click', async () => {
    const panels = $('#historyPanel');
    if (panels.style.display === 'none') return;
    const state = window._historyState;
    if (!state) {
        logError('没有提交数据，请先获取提交');
        return;
    }
    const path = window._historyCurPath ? window._historyCurPath() : '';
    const commits = state.allCommits.get(path) || [];
    if (commits.length === 0) {
        logError('没有提交数据，请先获取提交');
        return;
    }
    const stats = {};
    commits.forEach(c => {
        if (!stats[c.author]) stats[c.author] = { author: c.author, count: 0 };
        stats[c.author].count++;
    });
    const sorted = Object.values(stats).sort((a, b) => b.count - a.count);
    const totalCommits = sorted.reduce((s, a) => s + a.count, 0);

    const overlay = document.createElement('div');
    overlay.style.cssText = 'position:fixed;inset:0;background:rgba(0,0,0,0.5);display:flex;align-items:center;justify-content:center;z-index:200';
    const box = document.createElement('div');
    box.style.cssText = 'background:var(--surface);border:1px solid var(--border);border-radius:var(--radius-lg);width:400px;max-height:80vh;overflow-y:auto;box-shadow:0 20px 60px rgba(0,0,0,0.5)';
    box.innerHTML =
        '<div style="padding:14px 18px;border-bottom:1px solid var(--border);font-size:14px;font-weight:600;color:var(--text)">📊 提交统计</div>' +
        '<div style="padding:12px 18px">' +
        '<table style="width:100%;border-collapse:collapse;font-size:12px">' +
        '<thead><tr style="border-bottom:1px solid var(--border);color:var(--text-muted)">' +
        '<th style="text-align:left;padding:6px 8px;font-weight:600">作者</th>' +
        '<th style="text-align:right;padding:6px 8px;font-weight:600">提交数</th>' +
        '<th style="text-align:right;padding:6px 8px;font-weight:600">占比</th>' +
        '</tr></thead><tbody>' +
        sorted.map(a =>
            `<tr style="border-bottom:1px solid var(--border)">
                <td style="padding:6px 8px;color:var(--text)">${escapeHtml(a.author)}</td>
                <td style="padding:6px 8px;text-align:right;color:var(--text)">${a.count}</td>
                <td style="padding:6px 8px;text-align:right;color:var(--text-muted)">${(a.count / totalCommits * 100).toFixed(1)}%</td>
            </tr>`
        ).join('') +
        '</tbody></table>' +
        `<div style="padding:6px 8px;margin-top:4px;font-size:12px;color:var(--text-muted);text-align:right">合计: ${totalCommits} 条提交</div>` +
        '</div>' +
        '<div style="padding:10px 18px;border-top:1px solid var(--border);display:flex;justify-content:flex-end">' +
        '<button class="btn btn-secondary" style="padding:6px 18px;font-size:12px">关闭</button></div>';
    box.querySelector('.btn').addEventListener('click', () => overlay.remove());
    overlay.addEventListener('click', (e) => { if (e.target === overlay) overlay.remove(); });
    overlay.appendChild(box);
    document.body.appendChild(overlay);
});

$('#historyPanel').addEventListener('contextmenu', (e) => {
    const item = e.target.closest('.commit-item');
    if (!item || !item.dataset.hash) return;
    const content = $('#historyPanelContent');
    content.querySelectorAll('.commit-item.active').forEach(el => el.classList.remove('active'));
    item.classList.add('active');
    e.preventDefault();
    const existing = document.querySelector('.ctx-menu');
    if (existing) existing.remove();
    const menu = document.createElement('div');
    menu.className = 'ctx-menu';
    menu.style.cssText = 'position:fixed;z-index:200;background:var(--surface);border:1px solid var(--border);border-radius:var(--radius);padding:4px 0;box-shadow:0 4px 12px rgba(0,0,0,0.15);font-size:12px;min-width:150px';
    menu.style.left = e.clientX + 'px';
    menu.style.top = e.clientY + 'px';
    function makeMenuEl(text, fn) {
        const el = document.createElement('div');
        el.textContent = text;
        el.style.cssText = 'padding:6px 14px;cursor:pointer;color:var(--text);transition:background 0.1s';
        el.addEventListener('mouseenter', () => el.style.background = 'color-mix(in srgb, var(--primary) 10%, transparent)');
        el.addEventListener('mouseleave', () => el.style.background = '');
        el.addEventListener('click', () => { fn(); menu.remove(); });
        return el;
    }
    menu.appendChild(makeMenuEl('Copy SHA', () => {
        navigator.clipboard.writeText(item.dataset.hash);
        item.style.outline = '2px solid var(--primary)';
        setTimeout(() => item.style.outline = '', 600);
    }));
    menu.appendChild(makeMenuEl('查看提交信息', () => {
        GetCommitDetail(item.dataset.project, item.dataset.hash).then(detail => {
            if (!detail) return;
            const overlay = document.createElement('div');
            overlay.style.cssText = 'position:fixed;inset:0;background:rgba(0,0,0,0.5);display:flex;align-items:center;justify-content:center;z-index:200';
            const box = document.createElement('div');
            box.style.cssText = 'background:var(--surface);border:1px solid var(--border);border-radius:var(--radius-lg);width:520px;max-height:80vh;overflow-y:auto;box-shadow:0 20px 60px rgba(0,0,0,0.5)';
            const rows = [
                ['提交', detail.hash],
                ['提交者', `${detail.author} <${detail.authorEmail}>`],
                ['创作时间', detail.authorDate],
                ['提交人', `${detail.committer} <${detail.committerEmail}>`],
                ['提交日期', detail.committerDate],
                ['仓库路径', item.dataset.project],
                ...((detail.tags || []).length ? [['标签', detail.tags.join(', ')]] : []),
                ['提交信息', detail.message],
            ];
            const parts = [
                '<div style="padding:14px 18px;border-bottom:1px solid var(--border);font-size:14px;font-weight:600;color:var(--text)">提交信息</div>',
                '<div style="padding:12px 18px">',
                rows.map(([label, val]) =>
                    `<div style="display:flex;gap:12px;margin-bottom:8px;font-size:12px">
                        <span style="width:70px;flex-shrink:0;color:var(--text-muted)">${label}</span>
                        <span style="flex:1;color:var(--text);word-break:break-all;user-select:text">${escapeHtml(val)}</span>
                    </div>`
                ).join(''),
                '</div>',
                '<div style="padding:10px 18px;border-top:1px solid var(--border);display:flex;justify-content:flex-end">',
                '<button class="btn btn-secondary" style="padding:6px 18px;font-size:12px">关闭</button>',
                '</div>',
            ].join('');
            box.innerHTML = parts;
            box.querySelector('.btn').addEventListener('click', () => overlay.remove());
            overlay.addEventListener('click', (ev) => { if (ev.target === overlay) overlay.remove(); });
            overlay.appendChild(box);
            document.body.appendChild(overlay);
        });
    }));
    document.body.appendChild(menu);
});
document.addEventListener('click', () => {
    const menu = document.querySelector('.ctx-menu');
    if (menu) menu.remove();
    const c = $('#historyPanelContent');
    if (c) c.querySelectorAll('.commit-item.active').forEach(el => el.classList.remove('active'));
});

$('#projectList').addEventListener('contextmenu', (e) => {
    const label = e.target.closest('.checkbox-item');
    if (!label) return;
    const cb = label.querySelector('.proj-checkbox');
    if (!cb) return;
    e.preventDefault();
    const existing = document.querySelector('.ctx-menu');
    if (existing) existing.remove();
    const path = cb.value;
    const name = cb.dataset.name;
    const menu = document.createElement('div');
    menu.className = 'ctx-menu';
    menu.style.cssText = 'position:fixed;z-index:200;background:var(--surface);border:1px solid var(--border);border-radius:var(--radius);padding:4px 0;box-shadow:0 4px 12px rgba(0,0,0,0.15);font-size:12px;min-width:160px';
    menu.style.left = e.clientX + 'px';
    menu.style.top = e.clientY + 'px';
    function makeItem(icon, text, fn) {
        const el = document.createElement('div');
        el.style.cssText = 'padding:6px 14px;cursor:pointer;color:var(--text);transition:background 0.1s;display:flex;align-items:center;gap:6px';
        const iconSpan = document.createElement('span');
        iconSpan.style.cssText = 'width:20px;text-align:center;flex-shrink:0';
        iconSpan.textContent = icon;
        const textSpan = document.createElement('span');
        textSpan.textContent = text;
        el.appendChild(iconSpan);
        el.appendChild(textSpan);
        el.addEventListener('mouseenter', () => el.style.background = 'color-mix(in srgb, var(--primary) 10%, transparent)');
        el.addEventListener('mouseleave', () => el.style.background = '');
        el.addEventListener('click', () => { fn(); menu.remove(); });
        return el;
    }
    menu.appendChild(makeItem('📁', '在文件管理器中显示', () => OpenInExplorer(path)));
    menu.appendChild(makeItem('🖥', '打开终端', () => OpenInTerminal(path)));
    menu.appendChild(makeItem('📝', '使用 VSCode 打开', () => OpenInVSCode(path)));
    document.body.appendChild(menu);
});

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
switchMode(currentMode);
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
