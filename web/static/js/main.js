/* Scroll shadow on nav */
(function () {
  const nav = document.querySelector('nav');
  window.addEventListener('scroll', () => {
    nav.classList.toggle('scrolled', window.scrollY > 50);
  }, { passive: true });
})();

/* Resource registry tabs */
(function () {
  const buttons = document.querySelectorAll('.tab-btn');
  const panels  = document.querySelectorAll('.tab-panel');

  buttons.forEach(btn => {
    btn.addEventListener('click', () => {
      const target = btn.dataset.tab;
      buttons.forEach(b => b.classList.toggle('active', b === btn));
      panels.forEach(p  => p.classList.toggle('active', p.id === target));
    });
  });
})();

/* Copy buttons on code blocks */
(function () {
  document.querySelectorAll('.copy-btn').forEach(btn => {
    btn.addEventListener('click', () => {
      const target = document.getElementById(btn.dataset.copy);
      if (!target) return;
      navigator.clipboard.writeText(target.innerText).then(() => {
        btn.textContent = 'copied';
        btn.classList.add('copied');
        setTimeout(() => {
          btn.textContent = 'copy';
          btn.classList.remove('copied');
        }, 2000);
      });
    });
  });
})();
