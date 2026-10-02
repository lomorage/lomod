// Apple Live Photo (HEIC still + MOV video, zipped) playback.
// This is the one genuinely heavy path in the gallery (zip.js + heic2any + the
// laphs live-photo player, ~2.6MB together), so it's its own dynamically-imported
// module — nothing here loads until a live-photo asset is actually opened.
// Decode logic ported from static/blueimp-gallery/js/blueimp-gallery-livephoto.js,
// which this replaces for the lightbox's purposes.

let vendorPromise = null;

function loadScriptOnce(src) {
  return new Promise((resolve, reject) => {
    if (document.querySelector(`script[src="${src}"]`)) {
      resolve();
      return;
    }
    const el = document.createElement('script');
    el.src = src;
    el.onload = () => resolve();
    el.onerror = () => reject(new Error('failed to load ' + src));
    document.body.appendChild(el);
  });
}

function loadVendorScripts() {
  if (!vendorPromise) {
    // zip.js's built-in default `workerScriptsPath: 'static/zip/'` (resolved against
    // the page URL, which is always served from "/") already finds z-worker.js —
    // no extra configuration needed here, and setting `zip.workerScripts` alongside
    // the default `workerScriptsPath` would actually throw ("may be set, not both").
    vendorPromise = Promise.all([
      loadScriptOnce('static/zip/zip.js'),
      loadScriptOnce('static/zip/zip-ext.js'),
      loadScriptOnce('static/heic2any/heic2any.min.js'),
      loadScriptOnce('static/laphs/laphs.min.js'),
    ]);
  }
  return vendorPromise;
}

function getZipEntries(url) {
  return new Promise((resolve, reject) => {
    zip.createReader(
      new zip.HttpReader(url, true),
      (zipReader) => zipReader.getEntries(resolve),
      reject
    );
  });
}

function getEntryBlobURL(entry) {
  return new Promise((resolve) => {
    const writer = new zip.BlobWriter();
    entry.getData(writer, (blob) => resolve(URL.createObjectURL(blob)));
  });
}

async function extractLivePhoto(url) {
  const entries = await getZipEntries(url);
  const img = document.createElement('img');

  await Promise.all(
    entries.map(async (entry) => {
      const ext = entry.filename.split('.').pop().toLowerCase();
      if (ext === 'jpg') {
        img.src = await getEntryBlobURL(entry);
      } else if (ext === 'mov') {
        img.setAttribute('data-live-photo', await getEntryBlobURL(entry));
      } else if (ext === 'heic') {
        const blobURL = await getEntryBlobURL(entry);
        const blob = await (await fetch(blobURL)).blob();
        const converted = await heic2any({ blob, toType: 'image/jpeg', quality: 0.3, multiple: true });
        img.src = URL.createObjectURL(converted[0]);
      }
    })
  );

  return img;
}

// Loads whatever's needed, decodes the zip at `url`, and returns a container
// element (already initialized as a Live Photo, hover/touch to play) ready to
// insert into the lightbox stage.
export async function renderLivePhoto(url) {
  await loadVendorScripts();
  const img = await extractLivePhoto(url);
  const [instance] = LivePhotos.initialize(img);
  return instance.container;
}
