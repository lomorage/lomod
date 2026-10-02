'use strict';

// Bit flag mirrors common/scan.File's unexported dirFlag constant -- there's
// no separate JSON field for it, only the raw int.
var DIR_FLAG = 1 << 0;

// Folders with more files than this start collapsed, so a big scan (a whole
// SD card) doesn't render as one endless list.
var COLLAPSE_OVER_FILES = 50;
var POLL_INTERVAL_MS = 1000;

function isDirNode(node) {
    return (node.Flag & DIR_FLAG) !== 0;
}

// Nodes the user has excluded from import. Only ever holds file (non-dir)
// nodes -- a folder checkbox is just a bulk cascade over its descendant
// files, not something separately recorded, since only leaf files are
// actually imported (see pruneTree).
var deselected = new WeakSet();
// node -> its checkbox <input>, kept out of the node objects themselves so
// the fetched scan tree stays exactly JSON-serializable for re-posting.
var checkboxes = new WeakMap();
var scanTree = null;
var pollTimer = null;
// Bumped by scanPath/startImport so a poll chain from a previous scan or
// import (still in flight when a new one starts) can tell its own response
// is stale and no-op instead of overwriting the current one's UI state.
var requestGeneration = 0;
// The signed-in user's own folder, to pick a sensible default import mode.
var homeDir = '';
// The folder scanTree came from -- not re-read from the path box at import
// time, since that may have been edited after the scan.
var scannedPath = '';

function $id(id) {
    return document.getElementById(id);
}

function showStatus(el, msg, kind) {
    el.textContent = msg;
    el.className = 'localimport-status' + (kind ? ' is-' + kind : '');
    el.hidden = !msg;
}

// Fills a .localimport-progress block; a total of 0 shows an indeterminate bar.
function showProgress(el, done, total, text) {
    el.hidden = false;
    var fill = el.querySelector('.localimport-progress-fill');
    var track = el.querySelector('.localimport-progress-track');
    if (total > 0) {
        track.classList.remove('is-indeterminate');
        fill.style.width = Math.min(100, Math.round(done * 100 / total)) + '%';
    } else {
        track.classList.add('is-indeterminate');
        fill.style.width = '';
    }
    el.querySelector('.localimport-progress-text').textContent = text;
}

function hideProgress(el) {
    el.hidden = true;
}

function walkFiles(node, fn) {
    if (isDirNode(node)) {
        (node.Children || []).forEach(function (c) { walkFiles(c, fn); });
    } else {
        fn(node);
    }
}

function walkAll(node, fn) {
    fn(node);
    if (isDirNode(node)) {
        (node.Children || []).forEach(function (c) { walkAll(c, fn); });
    }
}

function countFiles(node) {
    var n = 0;
    walkFiles(node, function () { n++; });
    return n;
}

function setSelected(node, selected) {
    // Only leaf (file) nodes are tracked in `deselected` -- that's what
    // actually gets pruned at import time -- but every checkbox under this
    // node, folders included, needs to visually reflect the cascade too, or
    // an intermediate folder can be left showing "checked" while every file
    // inside it just got excluded.
    walkAll(node, function (n) {
        if (!isDirNode(n)) {
            if (selected) {
                deselected.delete(n);
            } else {
                deselected.add(n);
            }
        }
        var cb = checkboxes.get(n);
        if (cb) {
            cb.checked = selected;
            cb.indeterminate = false;
        }
    });
    updateSelectedCount();
}

function selectedCount() {
    var selected = 0;
    walkFiles(scanTree, function (f) {
        if (!deselected.has(f)) {
            selected++;
        }
    });
    return selected;
}

function updateSelectedCount() {
    var total = countFiles(scanTree);
    var selected = selectedCount();
    $id('localimport-selected-count').textContent =
        polyglot.t('LocalImportSelectedCount', { selected: selected, total: total });
    var btn = $id('localimport-import-btn');
    btn.textContent = polyglot.t('LocalImportImportN', { smart_count: selected });
    btn.disabled = selected === 0;
}

function formatCreateTime(node) {
    // Show the calendar date as written in the timestamp, which is the day
    // the server files the asset under -- converting to the browser's zone
    // first (new Date(...).toLocaleDateString()) can shift it by a day.
    var m = /^(\d{4})-(\d{2})-(\d{2})/.exec(node.CreateTime || '');
    if (!m || parseInt(m[1], 10) <= 1) {
        return '';
    }
    return new Date(parseInt(m[1], 10), parseInt(m[2], 10) - 1, parseInt(m[3], 10)).toLocaleDateString();
}

function baseName(path) {
    var parts = path.split(/[\\/]/).filter(function (p) { return p !== ''; });
    return parts.length ? parts[parts.length - 1] : path;
}

function renderLabel(node, metaText, displayName) {
    var label = document.createElement('span');
    label.className = 'localimport-node-label';
    var cb = document.createElement('input');
    cb.type = 'checkbox';
    cb.checked = true;
    checkboxes.set(node, cb);
    cb.addEventListener('change', function () {
        setSelected(node, cb.checked);
    });
    var name = document.createElement('span');
    name.className = 'localimport-node-name';
    name.textContent = displayName || node.Name;
    name.title = displayName ? node.Name : '';
    var meta = document.createElement('span');
    meta.className = 'localimport-node-meta';
    meta.textContent = metaText;

    label.appendChild(cb);
    label.appendChild(name);
    label.appendChild(meta);
    return { label: label, cb: cb };
}

function renderNode(node, depth) {
    var li = document.createElement('li');

    if (!isDirNode(node)) {
        li.appendChild(renderLabel(node, formatCreateTime(node)).label);
        return li;
    }

    var fileCount = countFiles(node);
    if (fileCount === 0) {
        return null; // empty directory, nothing to show or import
    }
    var details = document.createElement('details');
    details.open = depth === 0 || fileCount <= COLLAPSE_OVER_FILES;
    var summary = document.createElement('summary');
    // the root's Name is the scanned folder's full path; its last part is
    // what the user recognizes (hover shows the rest)
    var rendered = renderLabel(node, polyglot.t('LocalImportFolderMeta', { smart_count: fileCount }),
        depth === 0 ? baseName(node.Name) : '');
    rendered.cb.addEventListener('click', function (e) {
        e.stopPropagation(); // don't toggle the <details> open state
    });
    summary.appendChild(rendered.label);
    details.appendChild(summary);

    var ul = document.createElement('ul');
    (node.Children || []).forEach(function (child) {
        var childLi = renderNode(child, depth + 1);
        if (childLi) {
            ul.appendChild(childLi);
        }
    });
    details.appendChild(ul);
    li.appendChild(details);
    return li;
}

function renderTree(root) {
    var container = $id('localimport-tree');
    container.innerHTML = '';
    var ul = document.createElement('ul');
    var rootLi = renderNode(root, 0);
    if (rootLi) {
        ul.appendChild(rootLi);
    }
    container.appendChild(ul);
    updateSelectedCount();
}

function pruneTree(node) {
    if (!isDirNode(node)) {
        return deselected.has(node) ? null : node;
    }
    var kept = (node.Children || []).map(pruneTree).filter(Boolean);
    if (kept.length === 0) {
        return null;
    }
    var copy = Object.assign({}, node);
    copy.Children = kept;
    return copy;
}

function stopPolling() {
    if (pollTimer) {
        clearTimeout(pollTimer);
        pollTimer = null;
    }
}

function normalizePath(p) {
    var n = p.replace(/[\\/]+$/, '');
    // Windows paths are case-insensitive
    return /^[A-Za-z]:/.test(n) ? n.toLowerCase().replace(/\//g, '\\') : n;
}

function isInsideHome(path) {
    if (!homeDir) {
        return false;
    }
    var p = normalizePath(path), h = normalizePath(homeDir);
    return p === h || p.indexOf(h + (/^[a-z]:/.test(h) ? '\\' : '/')) === 0;
}

function setDefaultMode(path) {
    // Files already in the user's own folder don't need a second copy;
    // anything else (USB stick, SD card, another folder) is copied so the
    // library never depends on it staying plugged in.
    var mode = isInsideHome(path) ? 'link' : 'copy';
    document.querySelector('input[name="localimport-mode"][value="' + mode + '"]').checked = true;
}

function selectedMode() {
    var checked = document.querySelector('input[name="localimport-mode"]:checked');
    return checked ? checked.value : 'copy';
}

function setBusy(busy) {
    $id('localimport-scan-btn').disabled = busy;
    $id('localimport-browse-btn').disabled = busy;
    document.querySelectorAll('input[name="localimport-mode"]').forEach(function (r) {
        r.disabled = busy;
    });
}

function resetResults() {
    deselected = new WeakSet();
    checkboxes = new WeakMap();
    scanTree = null;
    $id('localimport-results').hidden = true;
    showStatus($id('localimport-scan-status'), '');
    showStatus($id('localimport-import-status'), '');
    hideProgress($id('localimport-scan-progress'));
    hideProgress($id('localimport-import-progress'));
    $id('localimport-failures').hidden = true;
    $id('localimport-done').hidden = true;
    $id('localimport-log-wrap').hidden = true;
    $id('localimport-log-wrap').open = false;
}

function extractErrorText(jqXHR) {
    if (jqXHR.status === 503) {
        return polyglot.t('LocalImportBusy');
    }
    try {
        var body = JSON.parse(jqXHR.responseText);
        if (body && body.text === 'Device is not mounted yet') {
            return polyglot.t('LocalImportFolderMissing');
        }
    } catch (e) {}
    return polyglot.t('LocalImportError');
}

function getStatus() {
    return $.ajax({ url: CONFIG.getScanStatusUrl(), method: 'GET', dataType: 'json' });
}

function scanPath(path) {
    stopPolling();
    var gen = ++requestGeneration; // invalidates any poll chain still in flight from before
    resetResults();
    setBusy(true);
    var progressEl = $id('localimport-scan-progress');
    showProgress(progressEl, 0, 0, polyglot.t('LocalImportScanning'));

    $.ajax({ url: CONFIG.getScanUrl(path, 'scan-video=1&exif-time=1'), method: 'POST' })
        .done(function () { pollScan(gen, path); })
        .fail(function (xhr) {
            if (gen !== requestGeneration) { return; }
            setBusy(false);
            hideProgress(progressEl);
            showStatus($id('localimport-scan-status'), extractErrorText(xhr), 'error');
        });
}

function pollScan(gen, path) {
    var progressEl = $id('localimport-scan-progress');
    getStatus()
        .done(function (st) {
            if (gen !== requestGeneration) { return; }
            var s = st.Scan;
            if (s.Running) {
                if (s.Counting) {
                    showProgress(progressEl, 0, 0, polyglot.t('LocalImportCounting', { smart_count: s.Total }));
                } else {
                    showProgress(progressEl, s.Checked, s.Total,
                        polyglot.t('LocalImportChecking', { checked: s.Checked, total: s.Total, found: s.New }));
                }
                pollTimer = setTimeout(function () { pollScan(gen, path); }, POLL_INTERVAL_MS);
                return;
            }
            fetchScanResult(gen, path, s);
        })
        .fail(function (xhr) { scanFailed(gen, xhr); });
}

function fetchScanResult(gen, path, scanStats) {
    $.ajax({ url: CONFIG.getScanUrl(path), method: 'GET', dataType: 'json' })
        .done(function (tree) {
            if (gen !== requestGeneration) { return; }
            setBusy(false);
            hideProgress($id('localimport-scan-progress'));
            showScanResult(tree, path, scanStats);
        })
        .fail(function (xhr) {
            if (gen !== requestGeneration) { return; }
            if (xhr.status === 503) {
                pollTimer = setTimeout(function () { pollScan(gen, path); }, POLL_INTERVAL_MS);
                return;
            }
            scanFailed(gen, xhr);
        });
}

function scanFailed(gen, xhr) {
    if (gen !== requestGeneration) { return; }
    setBusy(false);
    hideProgress($id('localimport-scan-progress'));
    showStatus($id('localimport-scan-status'), extractErrorText(xhr), 'error');
}

function showScanResult(tree, path, scanStats) {
    scanTree = tree;
    scannedPath = path;
    var found = countFiles(tree);
    var existing = scanStats.Existing || 0;

    if (found === 0) {
        showStatus($id('localimport-scan-status'), existing > 0
            ? polyglot.t('LocalImportNothingNewExisting', { smart_count: existing })
            : polyglot.t('LocalImportNothingFound'), 'success');
        return;
    }

    var summary = polyglot.t('LocalImportFound', { smart_count: found });
    if (existing > 0) {
        summary += ' ' + polyglot.t('LocalImportAlreadyIn', { smart_count: existing });
    }
    $id('localimport-summary').textContent = summary;
    setDefaultMode(path);
    renderTree(tree);
    $id('localimport-results').hidden = false;
    $id('localimport-import-btn').hidden = false;
    $id('localimport-mode').hidden = false;
    $id('localimport-select-all').hidden = false;
    $id('localimport-select-none').hidden = false;
    $id('localimport-results').scrollIntoView({ behavior: 'smooth', block: 'start' });
}

function startImport() {
    var path = scannedPath;
    var pruned = pruneTree(scanTree);
    if (!pruned) {
        return;
    }
    stopPolling();
    var gen = ++requestGeneration; // invalidates any poll chain still in flight from before
    var progressEl = $id('localimport-import-progress');
    showStatus($id('localimport-import-status'), '');
    $id('localimport-failures').hidden = true;
    $id('localimport-done').hidden = true;
    $id('localimport-import-btn').disabled = true;
    setBusy(true);
    var total = countFiles(pruned);
    showProgress(progressEl, 0, total, polyglot.t('LocalImportImporting', { done: 0, total: total }));

    var params = ['scan-video=1'];
    if (selectedMode() === 'copy') {
        params.push('copy=1');
    }
    $.ajax({
        // scan-video=1 matters here specifically -- handler/scan.go's scanImport
        // reads it fresh off this request (not from the original scan), and
        // defaults to false, silently skipping every video file on import
        // (no error, just a no-op per file) if it's left off.
        url: CONFIG.getScanImportUrl('localimport-' + Date.now()) + '?' + params.join('&'),
        method: 'POST',
        contentType: 'application/json',
        data: JSON.stringify(pruned)
    })
        .done(function () { pollImport(gen, path); })
        .fail(function (xhr) { importFailed(gen, xhr); });
}

function pollImport(gen, path) {
    var progressEl = $id('localimport-import-progress');
    getStatus()
        .done(function (st) {
            if (gen !== requestGeneration) { return; }
            var im = st.Import;
            if (im.Running) {
                showProgress(progressEl, im.Done, im.Total,
                    polyglot.t('LocalImportImporting', { done: im.Done, total: im.Total }));
                pollTimer = setTimeout(function () { pollImport(gen, path); }, POLL_INTERVAL_MS);
                return;
            }
            showProgress(progressEl, im.Total, im.Total,
                polyglot.t('LocalImportImporting', { done: im.Total, total: im.Total }));
            showImportResult(im, path);
        })
        .fail(function (xhr) { importFailed(gen, xhr); });
}

function importFailed(gen, xhr) {
    if (gen !== requestGeneration) { return; }
    setBusy(false);
    $id('localimport-import-btn').disabled = false;
    hideProgress($id('localimport-import-progress'));
    showStatus($id('localimport-import-status'), extractErrorText(xhr), 'error');
}

// Plain-language version of an import failure reason for common causes; the
// raw error (an OS message on the server) stays available as a tooltip.
function friendlyReason(reason) {
    if (/cannot find the (file|path)|no such file|not exist/i.test(reason)) {
        return polyglot.t('LocalImportReasonMissing');
    }
    if (/access is denied|permission denied|being used by another process/i.test(reason)) {
        return polyglot.t('LocalImportReasonNoAccess');
    }
    if (/no space|not enough space|disk (is )?full/i.test(reason)) {
        return polyglot.t('LocalImportReasonNoSpace');
    }
    return reason;
}

function showImportResult(im, path) {
    setBusy(false);
    var failures = im.Failures || [];
    var parts = [polyglot.t('LocalImportDoneImported', { smart_count: im.Imported })];
    if (im.Skipped > 0) {
        parts.push(polyglot.t('LocalImportDoneSkipped', { smart_count: im.Skipped }));
    }
    if (failures.length > 0) {
        parts.push(polyglot.t('LocalImportDoneFailed', { smart_count: failures.length }));
    }
    var kind = failures.length > 0 && im.Imported === 0 ? 'error' : (failures.length > 0 ? 'warning' : 'success');
    showStatus($id('localimport-import-status'), parts.join(polyglot.t('LocalImportListSep')), kind);

    var failuresEl = $id('localimport-failures');
    if (failures.length > 0) {
        failuresEl.querySelector('summary').textContent =
            polyglot.t('LocalImportFailedList', { smart_count: failures.length });
        var ul = failuresEl.querySelector('ul');
        ul.innerHTML = '';
        failures.forEach(function (f) {
            var li = document.createElement('li');
            var name = document.createElement('span');
            name.className = 'localimport-failure-path';
            name.textContent = f.Path;
            var reason = document.createElement('span');
            reason.className = 'localimport-failure-reason';
            reason.textContent = friendlyReason(f.Reason);
            reason.title = f.Reason;
            li.appendChild(name);
            li.appendChild(reason);
            ul.appendChild(li);
        });
        failuresEl.hidden = false;
        failuresEl.open = true;
    }

    // what's left to import is gone now; offer the next step instead
    $id('localimport-import-btn').hidden = true;
    $id('localimport-selected-count').textContent = '';
    $id('localimport-mode').hidden = true;
    hideProgress($id('localimport-import-progress'));
    $id('localimport-tree').querySelectorAll('input').forEach(function (cb) { cb.disabled = true; });
    $id('localimport-select-all').hidden = true;
    $id('localimport-select-none').hidden = true;
    $id('localimport-done').hidden = false;
    var logWrap = $id('localimport-log-wrap');
    logWrap.hidden = false;
    logWrap.dataset.path = path;
}

function loadLog(path) {
    $.ajax({ url: CONFIG.getScanLogUrl(path), method: 'GET', dataType: 'text' })
        .done(function (text) {
            $id('localimport-log').textContent = text;
        })
        .fail(function () {
            $id('localimport-log').textContent = polyglot.t('LocalImportError');
        });
}

// Folder-picker modal: browses the server's own filesystem via
// /assets/scan/browse (a cheap single-level directory listing), so picking a
// path doesn't require typing one by hand. `browsePath` tracks the modal's
// current location; confirming writes it into the main path input.
var browsePath = null;

var FOLDER_ICON_SVG = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7z"/></svg>';
var CHEVRON_ICON_SVG = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="9 18 15 12 9 6"/></svg>';

function pathSeparator(path) {
    // Match whichever separator the server's own path already uses
    // (backslash on Windows, forward slash on macOS/Linux) rather than
    // hardcoding one.
    return path.indexOf('/') !== -1 && path.indexOf('\\') === -1 ? '/' : '\\';
}

function openBrowser(startPath) {
    $id('localimport-browse-modal').hidden = false;
    loadBrowseDir(startPath);
}

function closeBrowser() {
    $id('localimport-browse-modal').hidden = true;
}

function renderBreadcrumb(path) {
    var el = $id('localimport-breadcrumb');
    el.innerHTML = '';
    var sep = pathSeparator(path);
    var parts = path.split(sep).filter(function (p) { return p !== ''; });
    var accum = sep === '/' ? '/' : '';
    parts.forEach(function (part, i) {
        accum = (i === 0 && /^[A-Za-z]:$/.test(part)) ? part + sep : accum + part + sep;
        var isLast = i === parts.length - 1;
        var crumb = document.createElement(isLast ? 'span' : 'button');
        if (!isLast) {
            crumb.type = 'button';
        }
        crumb.className = isLast ? 'localimport-crumb-current' : 'localimport-crumb';
        crumb.textContent = part;
        if (!isLast) {
            var target = accum;
            crumb.addEventListener('click', function () { loadBrowseDir(target); });
        }
        el.appendChild(crumb);
        if (!isLast) {
            var sepEl = document.createElement('span');
            sepEl.className = 'localimport-crumb-sep';
            sepEl.textContent = '/';
            sepEl.setAttribute('aria-hidden', 'true');
            el.appendChild(sepEl);
        }
    });
}

function renderRoots(roots, current) {
    var el = $id('localimport-roots');
    el.innerHTML = '';
    (roots || []).forEach(function (root) {
        var btn = document.createElement('button');
        btn.type = 'button';
        btn.className = 'localimport-root' +
            (normalizePath(current).indexOf(normalizePath(root)) === 0 && root.length > 1 ? ' is-current' : '');
        btn.textContent = root;
        btn.addEventListener('click', function () { loadBrowseDir(root); });
        el.appendChild(btn);
    });
    el.hidden = !roots || roots.length < 2;
}

function loadBrowseDir(path) {
    var list = $id('localimport-browse-list');
    list.innerHTML = '';
    $.ajax({ url: CONFIG.getScanBrowseUrl(path), method: 'GET', dataType: 'json' })
        .done(function (result) {
            browsePath = result.Path;
            renderBreadcrumb(result.Path);
            renderRoots(result.Roots, result.Path);

            if (!result.Dirs || result.Dirs.length === 0) {
                var empty = document.createElement('div');
                empty.className = 'localimport-browse-empty';
                empty.innerHTML = FOLDER_ICON_SVG;
                var emptyText = document.createElement('span');
                emptyText.textContent = polyglot.t('NoSubfolders');
                empty.appendChild(emptyText);
                list.appendChild(empty);
                return;
            }
            result.Dirs.forEach(function (name) {
                var item = document.createElement('div');
                item.className = 'localimport-browse-item';

                var icon = document.createElement('span');
                icon.className = 'localimport-browse-icon';
                icon.innerHTML = FOLDER_ICON_SVG;

                var label = document.createElement('span');
                label.className = 'localimport-browse-name';
                label.textContent = name;

                var chevron = document.createElement('span');
                chevron.className = 'localimport-browse-chevron';
                chevron.innerHTML = CHEVRON_ICON_SVG;

                item.appendChild(icon);
                item.appendChild(label);
                item.appendChild(chevron);
                item.addEventListener('click', function () {
                    var sep = pathSeparator(result.Path);
                    var base = result.Path.replace(/[\\/]+$/, '');
                    loadBrowseDir(base + sep + name);
                });
                list.appendChild(item);
            });
        })
        .fail(function (xhr) {
            var empty = document.createElement('div');
            empty.className = 'localimport-browse-empty';
            empty.textContent = extractErrorText(xhr);
            list.appendChild(empty);
        });
}

$(function () {
    if (sessionStorage.getItem('token') === null) {
        document.location.href = '/';
    }

    $('a#logout').text(polyglot.t('Logout') + sessionStorage.getItem('username'));
    $('a#logout').click(function () {
        sessionStorage.removeItem('token');
        sessionStorage.removeItem('userid');
        sessionStorage.removeItem('username');
        document.location.href = '/';
    });

    $.ajaxSetup({
        headers: { 'Authorization': 'token=' + sessionStorage.getItem('token') }
    });

    var pathInput = $id('localimport-path');
    pathInput.placeholder = polyglot.t('LocalImportPathPlaceholder');

    // Prefill with the current user's own home dir -- the motivating case is
    // recovering files that already sit in your own folder (e.g. survived a
    // reset but never made it into a fresh catalog). Any other path can be
    // typed in or browsed to instead.
    $.get({ url: CONFIG.getUsersUrl(), dataType: 'json' }).done(function (resp) {
        var users = (resp && resp.Users) || [];
        var myID = parseInt(sessionStorage.getItem('userid'), 10);
        var me = users.filter(function (u) { return u.ID === myID; })[0];
        if (me && me.HomeDir) {
            homeDir = me.HomeDir;
            if (!pathInput.value) {
                pathInput.value = me.HomeDir;
            }
        }
    });

    function scanCurrentPath() {
        var path = pathInput.value.trim();
        if (path) {
            scanPath(path);
        } else {
            pathInput.focus();
        }
    }

    $id('localimport-scan-btn').addEventListener('click', scanCurrentPath);
    pathInput.addEventListener('keydown', function (e) {
        if (e.key === 'Enter') {
            scanCurrentPath();
        }
    });

    $id('localimport-browse-btn').addEventListener('click', function () {
        openBrowser(pathInput.value.trim());
    });
    $id('localimport-browse-cancel').addEventListener('click', closeBrowser);
    $id('localimport-browse-select').addEventListener('click', function () {
        pathInput.value = browsePath;
        closeBrowser();
        // picking a folder is the whole point of opening the picker --
        // go straight on to finding photos in it
        scanPath(browsePath);
    });

    $id('localimport-select-all').addEventListener('click', function () {
        if (scanTree) {
            setSelected(scanTree, true);
        }
    });
    $id('localimport-select-none').addEventListener('click', function () {
        if (scanTree) {
            setSelected(scanTree, false);
        }
    });

    $id('localimport-import-btn').addEventListener('click', function () {
        startImport();
    });

    $id('localimport-again').addEventListener('click', function () {
        stopPolling();
        requestGeneration++;
        resetResults();
        setBusy(false);
        window.scrollTo({ top: 0, behavior: 'smooth' });
        openBrowser(pathInput.value.trim());
    });

    $id('localimport-log-wrap').addEventListener('toggle', function () {
        if (this.open) {
            loadLog(this.dataset.path);
        }
    });
});
