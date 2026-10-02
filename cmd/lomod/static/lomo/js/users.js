'use strict';

var usernameRegex = /^[a-zA-Z][a-zA-Z0-9_-]{0,31}$/;

// Icons for the account-row chips, in the same stroke style as the nav bar's.
var ICON_LOCK = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="11" width="18" height="11" rx="2"/><path d="M7 11V7a5 5 0 0110 0v4"/></svg>';
var ICON_QRCODE = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="7" height="7"/><rect x="14" y="3" width="7" height="7"/><rect x="3" y="14" width="7" height="7"/><rect x="14" y="14" width="3" height="3"/><rect x="18" y="18" width="3" height="3"/><rect x="14" y="18" width="3" height="0.01"/><rect x="18" y="14" width="0.01" height="3"/></svg>';
var ICON_TRASH = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 01-2 2H7a2 2 0 01-2-2V6m3 0V4a2 2 0 012-2h4a2 2 0 012 2v2"/><line x1="10" y1="11" x2="10" y2="17"/><line x1="14" y1="11" x2="14" y2="17"/></svg>';
var ICON_FOLDER = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M22 19a2 2 0 01-2 2H4a2 2 0 01-2-2V5a2 2 0 012-2h5l2 3h9a2 2 0 012 2z"/></svg>';
var ICON_CLOCK = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg>';

function metaItem(icon, value, title) {
    var el = document.createElement('span');
    el.className = 'users-meta-item';
    var valueSpan = document.createElement('span');
    valueSpan.textContent = value;
    el.innerHTML = icon;
    el.appendChild(valueSpan);
    if (title) {
        el.title = title;
    }
    return el;
}

function chipButton(className, icon, label) {
    var btn = document.createElement('button');
    btn.type = 'button';
    btn.className = 'users-chip ' + className;
    btn.innerHTML = icon + '<span>' + label + '</span>';
    return btn;
}

function formatMB(mb) {
    if (mb >= 1024) {
        return (mb / 1024).toFixed(1) + ' GB';
    }
    return mb + ' MB';
}

function loadDisks() {
    var select = document.getElementById('users-new-disk');
    $.get({ url: CONFIG.getMountUrl(), dataType: 'json' })
        .done(function (disks) {
            disks = (disks || []).filter(function (d) { return !d.Error; });
            if (disks.length === 0) {
                select.innerHTML = '<option value="">' + polyglot.t('NoDisksFound') + '</option>';
                return;
            }
            // Recommend the disk with the most free space, mirroring signup.
            disks.sort(function (a, b) { return b.FreeSize - a.FreeSize; });
            select.innerHTML = disks.map(function (d) {
                return '<option value="' + d.Dir + '">' + d.Dir + ' (' + formatMB(d.FreeSize) + ' ' + polyglot.t('Free') + ')</option>';
            }).join('');
        })
        .fail(function () {
            select.innerHTML = '<option value="">' + polyglot.t('NoDisksFound') + '</option>';
        });
}

function extractErrorText(jqXHR, fallbackKey) {
    try {
        var body = JSON.parse(jqXHR.responseText);
        if (body && body.text) {
            return body.text;
        }
    } catch (e) {}
    return polyglot.t(fallbackKey || 'SignupError');
}

function showCreateError(msg) {
    var el = document.getElementById('users-create-error');
    el.textContent = msg || '';
    el.hidden = !msg;
}

// The server address to embed in a pairing QR (matching lomo-mobile's
// PAIRING_QR_TYPE contract: {type, server, username, password}) has to be
// the LAN address a phone can actually reach, not whatever host this
// browser tab happens to be on (could be "localhost"). /welcome/info
// already computes that correctly for the first-run setup QR and has no
// auth/account-existence gate, so it works here unchanged.
var cachedServerAddress = null;
function getServerAddress() {
    if (cachedServerAddress) {
        return Promise.resolve(cachedServerAddress);
    }
    return fetch('/welcome/info').then(function (r) { return r.json(); }).then(function (info) {
        cachedServerAddress = info.Server;
        return cachedServerAddress;
    });
}

var qrInstance = null;
function showPairingQR(username, password) {
    getServerAddress().then(function (server) {
        var payload = { type: 'lomorage-pairing-v1', server: server, username: username, password: password };
        var container = document.getElementById('users-qr-canvas');
        if (qrInstance) {
            qrInstance.clear();
            qrInstance.makeCode(JSON.stringify(payload));
        } else {
            container.innerHTML = '';
            qrInstance = new QRCode(container, { text: JSON.stringify(payload), width: 220, height: 220 });
        }
        document.getElementById('users-qr-result').hidden = false;
    });
}

// shortenHomeDir mirrors the "<root>\username" naming the Storage Location
// dropdown already uses, by stripping the trailing username segment --
// e.g. "C:\Users\jeromy\Pictures\Lomorage\alice" -> "...\Lomorage".
function shortenHomeDir(homeDir, username) {
    var sep = homeDir.indexOf('\\') !== -1 ? '\\' : '/';
    var suffix = sep + username;
    if (homeDir.slice(-suffix.length) === suffix) {
        return homeDir.slice(0, -suffix.length) || homeDir;
    }
    return homeDir;
}

function closeInlinePanel(row) {
    var existing = row.querySelector('.users-inline-form, .users-inline-confirm');
    if (existing) {
        existing.remove();
    }
}

function openChangePasswordPanel(row, username) {
    closeInlinePanel(row);

    var form = document.createElement('form');
    form.className = 'users-inline-form';

    var pwdInput = document.createElement('input');
    pwdInput.type = 'password';
    pwdInput.className = 'users-input';
    pwdInput.placeholder = polyglot.t('NewPassword');
    pwdInput.autocomplete = 'new-password';

    var confirmInput = document.createElement('input');
    confirmInput.type = 'password';
    confirmInput.className = 'users-input';
    confirmInput.placeholder = polyglot.t('ConfirmNewPassword');
    confirmInput.autocomplete = 'new-password';

    var saveBtn = document.createElement('button');
    saveBtn.type = 'submit';
    saveBtn.className = 'btn btn-primary users-btn-sm';
    saveBtn.textContent = polyglot.t('SavePassword');

    var cancelBtn = document.createElement('button');
    cancelBtn.type = 'button';
    cancelBtn.className = 'users-btn-link';
    cancelBtn.textContent = polyglot.t('Cancel');
    cancelBtn.addEventListener('click', function () {
        form.remove();
    });

    var errorEl = document.createElement('div');
    errorEl.className = 'users-status is-error';
    errorEl.style.flexBasis = '100%';
    errorEl.hidden = true;

    form.appendChild(pwdInput);
    form.appendChild(confirmInput);
    form.appendChild(saveBtn);
    form.appendChild(cancelBtn);
    form.appendChild(errorEl);

    form.addEventListener('submit', function (e) {
        e.preventDefault();
        errorEl.hidden = true;

        var newPassword = pwdInput.value;
        if (newPassword.length < 6) {
            errorEl.textContent = polyglot.t('PasswordTooShort');
            errorEl.hidden = false;
            return;
        }
        if (newPassword !== confirmInput.value) {
            errorEl.textContent = polyglot.t('PasswordMismatch');
            errorEl.hidden = false;
            return;
        }

        saveBtn.disabled = true;
        hashPasswordForUser(username, newPassword)
            .then(function (hashedPwd) {
                return $.ajax({
                    url: CONFIG.getUsersUrl(),
                    method: 'PUT',
                    contentType: 'application/json',
                    data: JSON.stringify({ Name: username, Password: hashedPwd })
                });
            })
            .then(function () {
                // Changing a password invalidates that account's sessions
                // server-side (see updateUser) -- if that's the account
                // currently logged into this tab, its token is now dead,
                // so send it back to the login page instead of leaving a
                // stale session around to fail on the next action.
                if (username === sessionStorage.getItem('username')) {
                    sessionStorage.removeItem('token');
                    sessionStorage.removeItem('userid');
                    sessionStorage.removeItem('username');
                    document.location.href = '/';
                    return;
                }
                var successEl = document.createElement('div');
                successEl.className = 'users-status is-success';
                successEl.textContent = polyglot.t('PasswordUpdated');
                row.appendChild(successEl);
                setTimeout(function () { successEl.remove(); }, 3000);
                form.remove();
            })
            .catch(function (jqXHR) {
                errorEl.textContent = extractErrorText(jqXHR, 'ActionFailed');
                errorEl.hidden = false;
                saveBtn.disabled = false;
            });
    });

    row.appendChild(form);
    pwdInput.focus();
}

function openDeleteConfirm(row, username) {
    closeInlinePanel(row);

    var confirmBox = document.createElement('div');
    confirmBox.className = 'users-inline-confirm';

    var text = document.createElement('span');
    text.textContent = polyglot.t('ConfirmDeleteAccount').replace('%{username}', username);

    var confirmBtn = document.createElement('button');
    confirmBtn.type = 'button';
    confirmBtn.className = 'btn btn-danger users-btn-sm';
    confirmBtn.textContent = polyglot.t('ConfirmDeleteButton');
    confirmBtn.addEventListener('click', function () {
        confirmBtn.disabled = true;
        $.ajax({
            url: CONFIG.getUsersUrl() + '/' + encodeURIComponent(username),
            method: 'DELETE'
        })
            .then(function () {
                row.remove();
                loadDisks();
            })
            .catch(function (jqXHR) {
                confirmBtn.disabled = false;
                text.textContent = extractErrorText(jqXHR, 'ActionFailed');
            });
    });

    var cancelBtn = document.createElement('button');
    cancelBtn.type = 'button';
    cancelBtn.className = 'users-btn-link';
    cancelBtn.textContent = polyglot.t('Cancel');
    cancelBtn.addEventListener('click', function () {
        confirmBox.remove();
    });

    confirmBox.appendChild(text);
    confirmBox.appendChild(confirmBtn);
    confirmBox.appendChild(cancelBtn);
    row.appendChild(confirmBox);
}

// Deterministic per-username hue so each account gets its own avatar color
// without any color storage -- same math every time it's rendered.
function usernameHue(name) {
    var hash = 0;
    for (var i = 0; i < name.length; i++) {
        hash = (hash * 31 + name.charCodeAt(i)) >>> 0;
    }
    return hash % 360;
}

// Stable account colors drawn from the homepage's forest and memory palette.
// These darker shades keep the white initials readable.
function buildAvatar(username, isCurrent) {
    var hue = usernameHue(username);
    var colors = ['#2f654e', '#526c55', '#805f4e', '#776284', '#9a5141'];
    var el = document.createElement('div');
    el.className = 'users-avatar' + (isCurrent ? ' is-current' : '');
    el.style.background = colors[hue % colors.length];
    el.textContent = username.charAt(0).toUpperCase();
    return el;
}

function buildAccountRow(u, currentUsername) {
    var row = document.createElement('div');
    row.className = 'users-account-row';
    row.dataset.username = u.Name;

    var header = document.createElement('div');
    header.className = 'users-account-header';

    var isCurrent = u.Name === currentUsername;

    var identity = document.createElement('div');
    identity.className = 'users-account-identity';
    identity.appendChild(buildAvatar(u.Name, isCurrent));

    var info = document.createElement('div');
    info.className = 'users-account-info';

    var nameEl = document.createElement('div');
    nameEl.className = 'users-account-name';
    nameEl.textContent = u.Name;
    if (isCurrent) {
        var badge = document.createElement('span');
        badge.className = 'users-account-badge';
        badge.textContent = polyglot.t('CurrentAccount');
        nameEl.appendChild(badge);
    }
    info.appendChild(nameEl);

    var meta = document.createElement('div');
    meta.className = 'users-account-meta';
    var storage = shortenHomeDir(u.HomeDir || '', u.Name);
    var lastLogin = polyglot.t('NeverLoggedIn');
    if (u.LastLogin) {
        var parsed = new Date(u.LastLogin);
        lastLogin = isNaN(parsed.getTime()) ? u.LastLogin : parsed.toLocaleString();
    }
    meta.appendChild(metaItem(ICON_FOLDER, storage, u.HomeDir || ''));
    meta.appendChild(metaItem(ICON_CLOCK, lastLogin));
    info.appendChild(meta);

    identity.appendChild(info);
    header.appendChild(identity);

    var actions = document.createElement('div');
    actions.className = 'users-account-actions';

    actions.appendChild(chipButton('users-btn-changepwd', ICON_LOCK, polyglot.t('ChangePassword')));
    actions.appendChild(chipButton('users-btn-share-qr', ICON_QRCODE, polyglot.t('ShareQRCode')));

    if (!isCurrent) {
        actions.appendChild(chipButton('is-danger users-btn-delete', ICON_TRASH, polyglot.t('Delete')));
    }

    header.appendChild(actions);
    row.appendChild(header);

    return row;
}

function fetchAccounts() {
    var list = document.getElementById('users-list');
    return $.get({ url: CONFIG.getUsersUrl(), dataType: 'json' })
        .done(function (resp) {
            var users = ((resp && resp.Users) || []).filter(function (u) { return !u.BotUser; });
            users.sort(function (a, b) { return a.Name.localeCompare(b.Name); });
            var currentUsername = sessionStorage.getItem('username');
            list.innerHTML = '';
            users.forEach(function (u) {
                list.appendChild(buildAccountRow(u, currentUsername));
            });
        })
        .fail(function (jqXHR) {
            list.textContent = extractErrorText(jqXHR, 'ActionFailed');
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

    loadDisks();
    fetchAccounts();

    document.getElementById('users-list').addEventListener('click', function (e) {
        var row = e.target.closest('.users-account-row');
        if (!row) {
            return;
        }
        var username = row.dataset.username;

        if (e.target.classList.contains('users-btn-changepwd')) {
            openChangePasswordPanel(row, username);
        } else if (e.target.classList.contains('users-btn-share-qr')) {
            closeInlinePanel(row);
            document.getElementById('users-qr-username').value = username;
            document.getElementById('users-qr-password').value = '';
            document.getElementById('users-qr-result').hidden = true;
            document.getElementById('users-qr-form').scrollIntoView({ behavior: 'smooth', block: 'center' });
            document.getElementById('users-qr-password').focus();
        } else if (e.target.classList.contains('users-btn-delete')) {
            openDeleteConfirm(row, username);
        }
    });

    document.getElementById('users-create-form').addEventListener('submit', function (e) {
        e.preventDefault();
        showCreateError('');

        var username = document.getElementById('users-new-username').value.trim();
        var password = document.getElementById('users-new-password').value;
        var confirmPassword = document.getElementById('users-new-confirm-password').value;
        var disk = document.getElementById('users-new-disk').value;

        if (!usernameRegex.test(username)) {
            showCreateError(polyglot.t('InvalidUsername'));
            return;
        }
        if (password.length < 6) {
            showCreateError(polyglot.t('PasswordTooShort'));
            return;
        }
        if (password !== confirmPassword) {
            showCreateError(polyglot.t('PasswordMismatch'));
            return;
        }
        if (!disk) {
            showCreateError(polyglot.t('NoDiskSelected'));
            return;
        }

        var submitBtn = document.getElementById('users-create-form').querySelector('button[type="submit"]');
        submitBtn.disabled = true;

        hashPasswordForUser(username, password)
            .then(function (hashedPwd) {
                return $.ajax({
                    url: CONFIG.getUsersUrl(),
                    method: 'POST',
                    contentType: 'application/json',
                    data: JSON.stringify({ Name: username, Password: hashedPwd, HomeDir: disk, BotUser: false })
                });
            })
            .then(function () {
                document.getElementById('users-create-form').reset();
                loadDisks();
                fetchAccounts();
                // Immediately offer the pairing QR for the account just
                // created -- the plaintext password only ever exists in this
                // tab's memory, right up until this point.
                document.getElementById('users-qr-username').value = username;
                document.getElementById('users-qr-password').value = password;
                showPairingQR(username, password);
            })
            .catch(function (jqXHR) {
                showCreateError(extractErrorText(jqXHR));
            })
            .finally(function () {
                submitBtn.disabled = false;
            });
    });

    document.getElementById('users-qr-form').addEventListener('submit', function (e) {
        e.preventDefault();
        var username = document.getElementById('users-qr-username').value.trim();
        var password = document.getElementById('users-qr-password').value;
        if (!username || !password) {
            return;
        }
        showPairingQR(username, password);
    });
});
