import { getMimeType } from './mime.js';
import { open as openLightbox } from './lightbox.js';

function authHeaders() {
  return { Authorization: 'token=' + sessionStorage.getItem('token') };
}

async function fetchJSON(url) {
  const res = await fetch(url, { headers: authHeaders() });
  if (!res.ok) throw new Error('request failed: ' + res.status);
  return res.json();
}

function thumbnailEl(assetRec) {
  const mime = getMimeType(assetRec.Name);
  const isImage = mime.split('/')[0] === 'image';

  const a = document.createElement('a');
  a.href = CONFIG.getAssetUrl(assetRec.Name);
  a.title = assetRec.Name;
  a.dataset.mime = mime;
  a.dataset.gallery = '';

  const wrap = document.createElement('div');
  wrap.className = isImage ? 'image-thumbnail' : 'video-thumbnail';

  const img = document.createElement('img');
  img.loading = 'lazy';
  img.addEventListener('load', () => img.classList.add('is-loaded'));
  img.src = CONFIG.getPreviewUrl(assetRec.Name);
  if (img.complete) img.classList.add('is-loaded'); // already cached, load won't fire again
  wrap.appendChild(img);
  a.appendChild(wrap);

  a.addEventListener('click', (e) => {
    e.preventDefault();
    openLightbox(a);
  });

  return a;
}

// Shown in place of the timeline on a brand-new account -- otherwise
// there's nothing on this page to point a first-time user toward Local
// Import / Upload / Users, and an empty gallery looks the same whether
// that's expected or broken.
function emptyState() {
  const el = document.createElement('div');
  el.className = 'lomo-empty';
  el.innerHTML = `
    <p class="lomo-empty-title">${polyglot.t('EmptyGalleryTitle')}</p>
    <p class="lomo-empty-hint">${polyglot.t('EmptyGalleryHint')}</p>
    <div class="lomo-empty-actions">
      <a class="btn btn-primary" href="/localimport">${polyglot.t('EmptyGalleryLocalImport')}</a>
      <a class="btn btn-info" href="/import">${polyglot.t('EmptyGalleryUpload')}</a>
      <a class="btn btn-success" href="/users">${polyglot.t('EmptyGalleryUsers')}</a>
    </div>
  `;
  return el;
}

function ledgerLabel(year, month) {
  const header = document.createElement('div');
  header.className = 'lomo-ledger';
  const label = document.createElement('span');
  label.className = 'lomo-ledger-label';
  label.textContent = year + ' · ' + String(month).padStart(2, '0');
  const rule = document.createElement('span');
  rule.className = 'lomo-ledger-rule';
  header.append(label, rule);
  return header;
}

// Only fetches a month's actual asset list (the expensive call) once its
// placeholder scrolls near the viewport, instead of eagerly loading every
// month in the library up front.
async function loadMonth(year, month, groupEl) {
  groupEl.dataset.state = 'loading';
  groupEl.classList.remove('lomo-load-error');

  let tree;
  try {
    tree = await fetchJSON(CONFIG.getAssetLevelMerkleTreeUrl(year, month));
  } catch (err) {
    groupEl.classList.remove('lomo-loading');
    groupEl.classList.add('lomo-load-error');
    groupEl.dataset.state = 'idle'; // allow retry next time it scrolls into view
    console.error('failed to load ' + year + '-' + month, err);
    return;
  }

  const frag = document.createDocumentFragment();
  const days = (tree.Days || []).slice().reverse();
  for (const day of days) {
    for (const assetRec of day.Assets) {
      frag.appendChild(thumbnailEl(assetRec));
    }
  }
  groupEl.classList.remove('lomo-loading');
  groupEl.style.minHeight = '';
  groupEl.replaceChildren(frag);
  groupEl.dataset.state = 'loaded';
}

// Drops a loaded month's thumbnails back to a placeholder once it's scrolled
// far out of view, so a 10k+ library doesn't accumulate tens of thousands of
// live <img> nodes (and their decoded bitmaps) as the user scrolls through it.
// Freezes the placeholder at the section's last rendered height first, so
// removing hundreds of thumbnails doesn't yank the scroll position.
function unloadMonth(groupEl) {
  groupEl.style.minHeight = groupEl.offsetHeight + 'px';
  groupEl.replaceChildren();
  groupEl.classList.add('lomo-loading');
  groupEl.dataset.state = 'idle';
}

function onIntersect(entries) {
  for (const entry of entries) {
    const el = entry.target;
    const state = el.dataset.state;
    if (entry.isIntersecting) {
      if (state === 'idle') {
        loadMonth(el.dataset.year, el.dataset.month, el);
      }
    } else if (state === 'loaded') {
      unloadMonth(el);
    }
  }
}

async function renderGallery() {
  const links = document.getElementById('links');
  const skeleton = await fetchJSON(CONFIG.getMonthLevelMerkleTreeUrl());

  const years = (skeleton.Years || []).slice().reverse();
  if (years.length === 0) {
    links.appendChild(emptyState());
    return;
  }

  // One observer drives both directions: load a month shortly before it's
  // scrolled into view, and unload it once it's scrolled back out past the
  // same margin. Observation stays on for a group's whole lifetime (unlike a
  // one-shot "load once" observer) so it can be unloaded and reloaded
  // repeatedly as the user scrolls back and forth through a large library.
  // ~2-3 screens of margin (rather than ~1) so a fast scroll/fling reaches
  // content that's already loaded (or loading) instead of hitting a shimmer
  // placeholder — the tradeoff is more months held in memory at once, which
  // is still bounded, just a larger bound than the tightest possible one.
  const observer = new IntersectionObserver(onIntersect, { rootMargin: '2400px 0px' });
  const frag = document.createDocumentFragment();

  for (const yearRec of years) {
    const months = (yearRec.Months || []).slice().reverse();
    for (const monthRec of months) {
      frag.appendChild(ledgerLabel(yearRec.Year, monthRec.Month));
      const group = document.createElement('div');
      group.className = 'lomo-ledger-group lomo-loading';
      group.dataset.year = String(yearRec.Year);
      group.dataset.month = String(monthRec.Month);
      group.dataset.state = 'idle';
      frag.appendChild(group);
    }
  }
  links.appendChild(frag);

  // Observe after the placeholders are in the DOM so rootMargin is measured correctly.
  links.querySelectorAll('.lomo-ledger-group').forEach((el) => observer.observe(el));
}

function wireLogout() {
  const logout = document.getElementById('logout');
  logout.textContent = polyglot.t('Logout') + sessionStorage.getItem('username');
  logout.addEventListener('click', (e) => {
    e.preventDefault();
    sessionStorage.removeItem('token');
    sessionStorage.removeItem('userid');
    sessionStorage.removeItem('username');
    document.location.href = '/';
  });
}

function init() {
  if (sessionStorage.getItem('token') === null) {
    document.location.href = '/';
    return;
  }
  wireLogout();
  renderGallery().catch((err) => {
    console.error(err);
    alert(polyglot.t('FetchError'));
  });
}

init();
