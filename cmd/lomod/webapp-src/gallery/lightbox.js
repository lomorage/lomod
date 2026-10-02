// A small hand-built full-screen viewer, replacing blueimp-gallery's ~8-file
// stack for the common image/video case. Live Photos are handled by
// gallery/livephoto.js, loaded on demand only when one is actually opened.

let overlay, stage, deleteBtn;
let items = [];
let index = -1;

function ensureOverlay() {
  if (overlay) return;

  overlay = document.createElement('div');
  overlay.className = 'lomo-lightbox';
  overlay.innerHTML = `
    <button type="button" class="lomo-lightbox-btn lomo-lightbox-close" aria-label="Close">&times;</button>
    <button type="button" class="lomo-lightbox-btn lomo-lightbox-prev" aria-label="Previous">&lsaquo;</button>
    <button type="button" class="lomo-lightbox-btn lomo-lightbox-next" aria-label="Next">&rsaquo;</button>
    <button type="button" class="lomo-lightbox-btn lomo-lightbox-delete" aria-label="Delete">&#128465;</button>
    <div class="lomo-lightbox-stage"></div>
  `;
  document.body.appendChild(overlay);

  stage = overlay.querySelector('.lomo-lightbox-stage');
  deleteBtn = overlay.querySelector('.lomo-lightbox-delete');

  overlay.querySelector('.lomo-lightbox-close').addEventListener('click', close);
  overlay.querySelector('.lomo-lightbox-prev').addEventListener('click', () => step(-1));
  overlay.querySelector('.lomo-lightbox-next').addEventListener('click', () => step(1));
  deleteBtn.addEventListener('click', confirmDelete);

  overlay.addEventListener('click', (e) => {
    if (e.target === overlay) close();
  });

  document.addEventListener('keydown', (e) => {
    if (!overlay.classList.contains('is-open')) return;
    if (e.key === 'Escape') close();
    else if (e.key === 'ArrowLeft') step(-1);
    else if (e.key === 'ArrowRight') step(1);
  });

  // Swipe left/right to move between photos — the buttons overlap the image
  // edges on a phone-width screen (little room beside it), so swipe is the
  // primary way to navigate on touch, not just a bonus.
  let touchStartX = null;
  let touchStartY = null;
  overlay.addEventListener(
    'touchstart',
    (e) => {
      if (e.touches.length !== 1) return;
      touchStartX = e.touches[0].clientX;
      touchStartY = e.touches[0].clientY;
    },
    { passive: true }
  );
  overlay.addEventListener(
    'touchend',
    (e) => {
      if (touchStartX === null) return;
      const touch = e.changedTouches[0];
      const dx = touch.clientX - touchStartX;
      const dy = touch.clientY - touchStartY;
      touchStartX = null;
      // Ignore short or mostly-vertical drags so it doesn't fight normal
      // touch scrolling/tapping.
      if (Math.abs(dx) < 40 || Math.abs(dx) < Math.abs(dy) * 1.5) return;
      step(dx < 0 ? 1 : -1);
    },
    { passive: true }
  );
}

function currentItems() {
  return Array.from(document.querySelectorAll('#links [data-gallery]'));
}

export function open(triggerEl) {
  ensureOverlay();
  items = currentItems();
  index = items.indexOf(triggerEl);
  if (index === -1) return;
  overlay.classList.add('is-open');
  lockScroll();
  render();
}

function close() {
  overlay.classList.remove('is-open');
  unlockScroll();
  stage.replaceChildren();
  stopVideo();
}

// Locking scroll with `overflow: hidden` removes the scrollbar, which widens
// the page by its width and reflows the grid underneath — on the gallery page
// that reflow was enough to trip the IntersectionObserver load/unload
// boundary for many months at once, right as a photo was opened, causing a
// burst of unnecessary reloads. `scrollbar-gutter: stable` doesn't help here
// (it only applies once overflow is already non-visible, i.e. after the
// scrollbar's already gone) — compensating with equivalent padding is the
// reliable fix, so the content box width never actually changes.
function lockScroll() {
  const scrollbarWidth = window.innerWidth - document.documentElement.clientWidth;
  if (scrollbarWidth > 0) {
    document.body.style.paddingRight = scrollbarWidth + 'px';
  }
  document.body.classList.add('lomo-no-scroll');
}

function unlockScroll() {
  document.body.classList.remove('lomo-no-scroll');
  document.body.style.paddingRight = '';
}

function step(delta) {
  if (items.length === 0) return;
  index = (index + delta + items.length) % items.length;
  render();
}

function stopVideo() {
  const video = stage.querySelector('video');
  if (video) video.pause();
}

async function render() {
  stopVideo();
  const link = items[index];
  if (!link) {
    close();
    return;
  }

  const mime = link.dataset.mime || '';
  const type = mime.split('/')[0];
  const frag = document.createDocumentFragment();

  if (mime === 'livephoto/zip') {
    const loading = document.createElement('div');
    loading.className = 'lomo-lightbox-loading';
    loading.textContent = 'Loading live photo…';
    stage.replaceChildren(loading);
    try {
      const { renderLivePhoto } = await import('./livephoto.js');
      const container = await renderLivePhoto(link.href);
      if (items[index] !== link) return; // user navigated away while this loaded
      stage.replaceChildren(container);
    } catch (err) {
      stage.replaceChildren(errorNode('Could not load live photo: ' + err.message));
    }
    return;
  }

  if (type === 'video') {
    const video = document.createElement('video');
    video.src = link.href;
    video.controls = true;
    video.autoplay = true;
    video.playsInline = true;
    frag.appendChild(video);
    stage.replaceChildren(frag);
    return;
  }

  // Show a loading state immediately — without it, a slow load (e.g. a large
  // photo over a slow connection) just looks like a blank, broken overlay.
  const loading = document.createElement('div');
  loading.className = 'lomo-lightbox-loading';
  loading.textContent = 'Loading…';
  stage.replaceChildren(loading);

  const img = document.createElement('img');
  img.alt = link.title || '';
  img.onload = () => {
    if (items[index] !== link) return; // navigated away while this was loading
    stage.replaceChildren(img);
  };
  img.onerror = () => {
    if (items[index] !== link) return;
    stage.replaceChildren(errorNode('Could not load image'));
  };
  // Full original: unlike an arbitrary preview size, this never needs an
  // on-demand transcode (handler/asset.go just streams the file), so on a LAN
  // it loads faster in practice than waiting on a resize — confirmed against
  // a real server. The loading state above covers the (network) wait either way.
  img.src = link.href;
}

function errorNode(message) {
  const el = document.createElement('div');
  el.className = 'lomo-lightbox-error';
  el.textContent = message;
  return el;
}

function confirmDelete() {
  const link = items[index];
  if (!link) return;

  const dialog = document.createElement('dialog');
  dialog.className = 'lomo-confirm';
  dialog.innerHTML = `
    <p>${polyglot.t('DeleteConfirm')}</p>
    <div class="lomo-confirm-actions">
      <button type="button" class="btn btn-warning" data-action="cancel">${polyglot.t('Cancel')}</button>
      <button type="button" class="btn btn-danger" data-action="ok">${polyglot.t('OK')}</button>
    </div>
  `;
  document.body.appendChild(dialog);
  dialog.addEventListener('close', () => dialog.remove());
  dialog.addEventListener('click', (e) => {
    if (e.target.dataset.action === 'ok') {
      dialog.close();
      doDelete(link);
    } else if (e.target.dataset.action === 'cancel' || e.target === dialog) {
      dialog.close();
    }
  });
  dialog.showModal();
}

async function doDelete(link) {
  try {
    const res = await fetch(link.href, { method: 'DELETE' });
    if (!res.ok) throw new Error('request failed: ' + res.status);
  } catch (err) {
    console.error(err);
    alert(polyglot.t('DeleteFail'));
    return;
  }

  const wrap = link.closest('.image-thumbnail, .video-thumbnail');
  (wrap || link).remove();

  items = currentItems();
  if (items.length === 0) {
    close();
    return;
  }
  index = Math.min(index, items.length - 1);
  render();
}
