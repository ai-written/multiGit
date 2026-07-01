import { EventsOn } from '/wailsjs/runtime/runtime.js';
import { WindowMinimise, WindowToggleMaximise, WindowIsMaximised, Quit } from '/wailsjs/runtime/runtime.js';
import { LoadConfig, SaveConfig, SelectDir, ListProjects, UpdatePackage } from '/wailsjs/go/main/App.js';

const $ = (sel) => document.querySelector(sel);
const $$ = (sel) => document.querySelectorAll(sel);

const terminal = $('#terminalContent');
const terminalContainer = $('#terminal');

let config = {
    registry: 'https://registry.npmmirror.com',
    branches: ['main', 'develop'],
    packages: [],
    autoBumpProjects: [],
    rootPath: ''
};

function log(text) {
    const div = document.createElement('div');
    div.textContent = text;
    terminal.appendChild(div);
    terminalContainer.scrollTop = terminalContainer.scrollHeight;
}

function logError(text) {
    const div = document.createElement('div');
    div.textContent = text;
    div.style.color = '#ef4444';
    terminal.appendChild(div);
    terminalContainer.scrollTop = terminalContainer.scrollHeight;
}

EventsOn('log', (text) => {
    log(text);
});

async function loadAndApplyConfig() {
    try {
        config = await LoadConfig();
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
    container.style.display = 'flex';
    container.innerHTML = '<label class="form-label">输入版本号</label>' +
        Array.from(checked).map(cb =>
            `<div class="version-row">
                <span class="pkg-name">${cb.value}</span>
                <input type="text" class="input pkg-version" data-pkg="${cb.value}" placeholder="输入版本号" />
            </div>`
        ).join('');
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
            log(result.message);
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
}

function openConfig() {
    $('#cfgRegistry').value = config.registry || '';
    $('#cfgBranches').value = (config.branches || []).join(', ');
    $('#cfgPackages').value = (config.packages || []).join(', ');
    $('#cfgAutoBump').value = (config.autoBumpProjects || []).join(', ');
    $('#configModal').style.display = 'flex';
}

function closeConfig() {
    $('#configModal').style.display = 'none';
}

async function saveConfig() {
    config.registry = $('#cfgRegistry').value.trim();
    config.branches = $('#cfgBranches').value.split(',').map(s => s.trim()).filter(Boolean);
    config.packages = $('#cfgPackages').value.split(',').map(s => s.trim()).filter(Boolean);
    config.autoBumpProjects = $('#cfgAutoBump').value.split(',').map(s => s.trim()).filter(Boolean);

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
    localStorage.setItem('gitdesk-theme', next ? 'dark' : 'light');
});

const saved = localStorage.getItem('gitdesk-theme');
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
