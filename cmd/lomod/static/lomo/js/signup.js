'use strict';

var usernameRegex = /^[a-zA-Z][a-zA-Z0-9_-]{0,31}$/;

function showSignupError(msg) {
    var el = document.getElementById('signup-error');
    el.textContent = msg || '';
    el.style.display = msg ? 'block' : 'none';
}

function formatMB(mb) {
    if (mb >= 1024) {
        return (mb / 1024).toFixed(1) + ' GB';
    }
    return mb + ' MB';
}

function loadDisks() {
    var select = document.getElementById('signup-disk');
    // dataType: 'json' is required -- /mount's response isn't served with a
    // application/json Content-Type, so jQuery would otherwise hand back the
    // raw response text instead of a parsed array.
    $.get({ url: CONFIG.getMountUrl(), dataType: 'json' })
        .done(function (disks) {
            disks = (disks || []).filter(function (d) { return !d.Error; });
            if (disks.length === 0) {
                select.innerHTML = '<option value="">' + polyglot.t('NoDisksFound') + '</option>';
                return;
            }
            // Recommend the disk with the most free space, mirroring the mobile app.
            disks.sort(function (a, b) { return b.FreeSize - a.FreeSize; });
            select.innerHTML = disks.map(function (d) {
                return '<option value="' + d.Dir + '">' + d.Dir + ' (' + formatMB(d.FreeSize) + ' ' + polyglot.t('Free') + ')</option>';
            }).join('');
        })
        .fail(function () {
            select.innerHTML = '<option value="">' + polyglot.t('NoDisksFound') + '</option>';
        });
}

function extractErrorText(jqXHR) {
    try {
        var body = JSON.parse(jqXHR.responseText);
        if (body && body.text) {
            return body.text;
        }
    } catch (e) {}
    return polyglot.t('SignupError');
}

// Mirrors login.js's login(), so a freshly created account signs straight
// in the same way a returning user does, without a second Argon2 hash.
function uuidv4() {
    return '00-0-4-1-000'.replace(/[^-]/g,
            s => ((Math.random() + ~~s) * 0x10000 >> s).toString(16).padStart(4, '0')
    );
}

function signInAfterSignup(username, hashedPwd) {
    var auth = btoa(username + ":" + hashedPwd + ":web" + uuidv4());
    $.ajaxSetup({
        headers: {
            "Authorization": "Basic " + auth
        }
    });
    return $.ajax({ url: CONFIG.getLoginUrl() }).done(function (json) {
        sessionStorage.setItem("userid", json.Userid);
        sessionStorage.setItem("token", json.Token);
        sessionStorage.setItem("username", username);
        document.location.href = '/gallery';
    });
}

document.addEventListener('DOMContentLoaded', loadDisks);

document.getElementById('signup-form').addEventListener('submit', function (e) {
    e.preventDefault();
    showSignupError('');

    var username = document.getElementById('signup-username').value.trim();
    var password = document.getElementById('signup-password').value;
    var confirmPassword = document.getElementById('signup-confirm-password').value;
    var disk = document.getElementById('signup-disk').value;

    if (!usernameRegex.test(username)) {
        showSignupError(polyglot.t('InvalidUsername'));
        return;
    }
    if (password.length < 6) {
        showSignupError(polyglot.t('PasswordTooShort'));
        return;
    }
    if (password !== confirmPassword) {
        showSignupError(polyglot.t('PasswordMismatch'));
        return;
    }
    if (!disk) {
        showSignupError(polyglot.t('NoDiskSelected'));
        return;
    }

    var submitBtn = document.getElementById('signup-submit');
    submitBtn.disabled = true;

    hashPasswordForUser(username, password)
        .then(function (hashedPwd) {
            return $.ajax({
                url: CONFIG.getUsersUrl(),
                method: 'POST',
                contentType: 'application/json',
                data: JSON.stringify({ Name: username, Password: hashedPwd, HomeDir: disk, BotUser: false })
            }).then(function () {
                return signInAfterSignup(username, hashedPwd);
            }, function (jqXHR) {
                throw new Error(extractErrorText(jqXHR));
            });
        })
        .catch(function (err) {
            showSignupError((err && err.message) || polyglot.t('SignupError'));
            submitBtn.disabled = false;
        });
});
