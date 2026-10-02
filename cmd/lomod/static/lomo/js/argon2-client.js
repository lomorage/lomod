'use strict';

// Shared by login.js and signup.js: both need to Argon2id-hash a password
// client-side before it ever goes over the wire, using the exact same
// salt/params the server expects (see common/security.EncryptPassword and
// lomo-mobile's AuthService.js, which must all agree on this scheme).

var argon2Worker;

function getArgon2Worker() {
    if (!argon2Worker) {
        argon2Worker = new Worker('static/lomo/dist/argon2-worker.js');
    }
    return argon2Worker;
}

// Runs the Argon2id hash in a Web Worker so the tab doesn't freeze while it computes.
function hashPassword(arg) {
    return new Promise(function (resolve, reject) {
        var worker = getArgon2Worker();
        worker.onmessage = function (e) {
            if (e.data.ok) {
                resolve(e.data.hash);
            } else {
                reject(new Error(e.data.error));
            }
        };
        worker.onerror = function (err) {
            reject(err);
        };
        worker.postMessage(arg);
    });
}

// hashPasswordForUser hashes with the salt/params every Lomorage client (web,
// mobile) agrees on, so a password hashed here can be used to both create an
// account and log into it afterwards.
function hashPasswordForUser(username, password) {
    return hashPassword({
        pass: password,
        salt: username + '@lomorage.lomoware',
        time: 3,
        mem: 4096,
        hashLen: 32,
        parallelism: 1,
        type: 2 // Argon2id
    });
}
