'use strict';

document.getElementById('login-form').addEventListener('submit', function(e) {
    e.preventDefault();
});

function showLoginError(msg) {
    var el = document.getElementById('login-error');
    el.textContent = msg || '';
    el.style.display = msg ? 'block' : 'none';
}

// getArgon2Worker/hashPassword now live in argon2-client.js (shared with signup.js).

document.getElementById('submit').addEventListener('click', function (e) {
    e.preventDefault();
    var submitBtn = document.getElementById('submit');
    if (submitBtn.disabled) {
        return;
    }

    logTs = performance.now();
    showLoginError('');
    submitBtn.disabled = true;
    submitBtn.classList.add('is-loading');

    hashPassword(getArg())
        .then(function (hash) {
            log('Hashed in ' + Math.round(performance.now() - logTs) + 'ms');
            login(hash);
        })
        .catch(function (err) {
            log('Error: ' + err);
            showLoginError(polyglot.t('LoginError'));
        })
        .finally(function () {
            submitBtn.disabled = false;
            submitBtn.classList.remove('is-loading');
        });
});

function loadScript(src, onload, onerror) {
    var el = document.createElement('script');
    el.src = src;
    el.onload = onload;
    el.onerror = onerror;
    document.body.appendChild(el);
}

function uuidv4() {
    return '00-0-4-1-000'.replace(/[^-]/g,
            s => ((Math.random() + ~~s) * 0x10000 >> s).toString(16).padStart(4, '0')
    );
}

function login(hashedPwd) {
    var auth = btoa($('#username').val() + ":" + hashedPwd + ":web" + uuidv4())
    $.ajaxSetup({
        headers: {
            "Authorization": "Basic " + auth
        }
    });
    $.ajax({
        url: CONFIG.getLoginUrl()
    })
    .done(function (json) {
        log( "Login succeed! save token " + json.Token);
        sessionStorage.setItem("userid", json.Userid);
        sessionStorage.setItem("token", json.Token);
        sessionStorage.setItem("username", $('#username').val());
        document.location.href = '/gallery'
    })
    .fail(function( xhr, status, errorThrown ) {
        showLoginError( polyglot.t("LoginError") );
        log( "Error: " + errorThrown );
        log( "Status: " + status );
        console.dir( xhr );
    });
}

function getArg() {
    return {
        pass: $('#password').val(),
        salt: $('#username').val() + '@lomorage.lomoware',
        time: 3,
        mem: 4096,
        hashLen: 32,
        parallelism: 1,
        type: 2 //Argon2id
    };
}

var logTs = 0;

function log(msg) {
    if (!msg) {
        return;
    }

    var elapsedMs = Math.round(performance.now() - logTs);
    var elapsedSec = (elapsedMs / 1000).toFixed(3);
    var elapsed = leftPad(elapsedSec, 6);
    console.log('[' + elapsed + '] ' + msg)
}

function leftPad(str, len) {
    str = str.toString();
    while (str.length < len) {
        str = '0' + str;
    }
    return str;
}

function clearLog() {
}
