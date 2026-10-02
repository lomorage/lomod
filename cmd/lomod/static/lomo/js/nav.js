// Mobile menu toggle for the dependency-free .lomo-navbar (no Bootstrap JS here).
(function () {
  var toggle = document.querySelector('.lomo-navbar-toggle');
  var links = document.querySelector('.lomo-navbar-links');
  var backdrop = document.querySelector('.lomo-navbar-backdrop');
  if (!toggle || !links) return;

  function setOpen(isOpen) {
    links.classList.toggle('is-open', isOpen);
    toggle.setAttribute('aria-expanded', String(isOpen));
    if (backdrop) backdrop.classList.toggle('is-open', isOpen);

    // Lock background scroll while the menu is open, compensating for the
    // scrollbar's width so the page doesn't shift/reflow when it disappears
    // (see gallery/lightbox.js — same issue, same fix, small enough here
    // that it's not worth sharing a module for a handful of lines).
    if (isOpen) {
      var scrollbarWidth = window.innerWidth - document.documentElement.clientWidth;
      if (scrollbarWidth > 0) document.body.style.paddingRight = scrollbarWidth + 'px';
      document.body.classList.add('lomo-no-scroll');
    } else {
      document.body.classList.remove('lomo-no-scroll');
      document.body.style.paddingRight = '';
    }
  }

  toggle.addEventListener('click', function () {
    setOpen(!links.classList.contains('is-open'));
  });

  if (backdrop) {
    backdrop.addEventListener('click', function () {
      setOpen(false);
    });
  }

  links.addEventListener('click', function (e) {
    if (e.target.tagName === 'A') setOpen(false);
  });
})();
