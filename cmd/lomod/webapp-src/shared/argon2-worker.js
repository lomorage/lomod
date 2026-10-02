// Runs the Argon2id password hash off the main thread so the login tab stays
// responsive. Ports the same emscripten glue (static/argon2/argon2.js) that
// used to run inline via static/argon2/calc.js — same params, same hex output
// format, just moved into a Worker (argon2.js already detects
// `typeof importScripts === 'function'` and avoids all DOM access in that mode).

self.onmessage = function (e) {
  runArgon2(e.data).then(
    function (hash) {
      self.postMessage({ ok: true, hash: hash });
    },
    function (err) {
      self.postMessage({ ok: false, error: String((err && err.message) || err) });
    }
  );
};

function runArgon2(arg) {
  return new Promise(function (resolve, reject) {
    var KB = 1024 * 1024;
    var MB = 1024 * KB;
    var GB = 1024 * MB;
    var WASM_PAGE_SIZE = 64 * 1024;
    var totalMemory = (2 * GB - 64 * KB) / 1024 / WASM_PAGE_SIZE;
    var initialMemory = Math.min(
      Math.max(Math.ceil((arg.mem * 1024) / WASM_PAGE_SIZE), 256) + 256,
      totalMemory
    );
    var wasmMemory = new WebAssembly.Memory({ initial: initialMemory, maximum: totalMemory });

    self.Module = {
      wasmBinary: null,
      wasmJSMethod: 'native-wasm',
      wasmBinaryFile: '/static/argon2/argon2.wasm',
      wasmMemory: wasmMemory,
      buffer: wasmMemory.buffer,
      TOTAL_MEMORY: initialMemory * WASM_PAGE_SIZE,
      postRun: function () {
        try {
          resolve(computeHash(arg));
        } catch (err) {
          reject(err);
        }
      },
    };

    fetch(self.Module.wasmBinaryFile)
      .then(function (res) {
        return res.arrayBuffer();
      })
      .then(function (buf) {
        self.Module.wasmBinary = buf;
        importScripts('/static/argon2/argon2.js');
      })
      .catch(reject);
  });
}

function computeHash(arg) {
  var Module = self.Module;
  var pwd = allocateArray(arg.pass);
  var salt = allocateArray(arg.salt);
  var hash = Module.allocate(new Array(arg.hashLen), 'i8', Module.ALLOC_NORMAL);
  var encoded = Module.allocate(new Array(512), 'i8', Module.ALLOC_NORMAL);

  var res = Module._argon2_hash(
    arg.time,
    arg.mem,
    arg.parallelism,
    pwd,
    arg.pass.length,
    salt,
    arg.salt.length,
    hash,
    arg.hashLen,
    encoded,
    512,
    arg.type,
    0x13
  );

  if (res !== 0) {
    var err;
    try {
      err = Module.UTF8ToString(Module._argon2_error_message(res));
    } catch (e) {}
    try {
      Module._free(pwd);
      Module._free(salt);
      Module._free(hash);
      Module._free(encoded);
    } catch (e) {}
    throw new Error('argon2 error ' + res + (err ? ': ' + err : ''));
  }

  // Walk the raw encoded-hash bytes exactly like the original calc.js did,
  // rather than re-encoding the decoded string, to keep the hex output identical.
  var encodedStrLen = Module.UTF8ToString(encoded).length;
  var hashArr = [];
  for (var i = encoded; i < encoded + encodedStrLen; i++) {
    hashArr.push(Module.HEAP8[i]);
  }
  var encodedHash =
    hashArr
      .map(function (b) {
        return ('0' + (0xff & b).toString(16)).slice(-2);
      })
      .join('') + '00';

  try {
    Module._free(pwd);
    Module._free(salt);
    Module._free(hash);
    Module._free(encoded);
  } catch (e) {}

  return encodedHash;
}

function allocateArray(strOrArr) {
  var arr =
    strOrArr instanceof Uint8Array || strOrArr instanceof Array
      ? strOrArr
      : new TextEncoder().encode(strOrArr);
  return self.Module.allocate(arr, 'i8', self.Module.ALLOC_NORMAL);
}
