/* ── Scroll-based nav blur ─────────────────────────────────────────────────── */
(function () {
  const nav = document.querySelector('nav');
  window.addEventListener('scroll', () => {
    nav.classList.toggle('scrolled', window.scrollY > 50);
  }, { passive: true });
})();

/* ── Intersection Observer — fade-up ──────────────────────────────────────── */
(function () {
  const observer = new IntersectionObserver((entries) => {
    entries.forEach(e => {
      if (e.isIntersecting) {
        e.target.classList.add('visible');
        observer.unobserve(e.target);
      }
    });
  }, { threshold: 0.08, rootMargin: '0px 0px -40px 0px' });

  document.querySelectorAll('.fade-up').forEach(el => observer.observe(el));
})();

/* ── Counter animation ────────────────────────────────────────────────────── */
(function () {
  function animateCount(el, target, duration) {
    let start = 0;
    const step = target / (duration / 16);
    const timer = setInterval(() => {
      start += step;
      if (start >= target) { el.textContent = target + (el.dataset.suffix || ''); clearInterval(timer); return; }
      el.textContent = Math.floor(start) + (el.dataset.suffix || '');
    }, 16);
  }

  const counterObserver = new IntersectionObserver((entries) => {
    entries.forEach(e => {
      if (e.isIntersecting) {
        const el = e.target;
        animateCount(el, parseInt(el.dataset.target, 10), 1200);
        counterObserver.unobserve(el);
      }
    });
  }, { threshold: 0.5 });

  document.querySelectorAll('[data-target]').forEach(el => counterObserver.observe(el));
})();

/* ── Resource grid tabs ───────────────────────────────────────────────────── */
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

/* ── Copy buttons ─────────────────────────────────────────────────────────── */
(function () {
  document.querySelectorAll('.copy-btn').forEach(btn => {
    btn.addEventListener('click', () => {
      const target = document.getElementById(btn.dataset.copy);
      if (!target) return;
      const text = target.innerText;
      navigator.clipboard.writeText(text).then(() => {
        btn.textContent = '✓ copied';
        btn.classList.add('copied');
        setTimeout(() => {
          btn.textContent = 'copy';
          btn.classList.remove('copied');
        }, 2000);
      });
    });
  });
})();

/* ── Hero terminal typing animation ──────────────────────────────────────── */
(function () {
  const output = document.getElementById('terminal-output');
  if (!output) return;

  const lines = [
    { type: 'cmd',  text: 'kubectl apply -f iam-role.yaml' },
    { type: 'out',  text: 'iamrole.aws.konfig.io/my-app-role created' },
    { type: 'cmd',  text: 'kubectl apply -f vpc.yaml' },
    { type: 'out',  text: 'vpc.aws.konfig.io/prod-vpc created' },
    { type: 'cmd',  text: 'kubectl get iamrole my-app-role -n infra' },
    { type: 'ok',   text: 'NAME           READY   ARN' },
    { type: 'ok',   text: 'my-app-role    True    arn:aws:iam::123456789:role/my-app-role' },
    { type: 'cmd',  text: 'kubectl get vpc prod-vpc -n infra' },
    { type: 'ok',   text: 'NAME       READY   VPC-ID' },
    { type: 'ok',   text: 'prod-vpc   True    vpc-0abc12def3456789' },
  ];

  let lineIdx = 0;
  let charIdx = 0;
  let current = null;
  let pause = 0;

  function tick() {
    if (pause > 0) { pause--; setTimeout(tick, 20); return; }

    if (lineIdx >= lines.length) {
      // restart after a pause
      setTimeout(() => {
        output.innerHTML = '';
        lineIdx = 0; charIdx = 0; current = null;
        tick();
      }, 3000);
      return;
    }

    const line = lines[lineIdx];

    if (!current) {
      current = document.createElement('div');
      if (line.type === 'cmd') {
        current.innerHTML = '<span class="term-prompt">$ </span><span class="term-cmd"></span>';
      } else if (line.type === 'ok') {
        current.className = 'term-ok';
      } else {
        current.className = 'term-out';
      }
      output.appendChild(current);
    }

    const target = line.type === 'cmd'
      ? current.querySelector('.term-cmd')
      : current;

    if (charIdx < line.text.length) {
      target.textContent += line.text[charIdx++];
      setTimeout(tick, line.type === 'cmd' ? 45 : 12);
    } else {
      lineIdx++;
      charIdx = 0;
      current = null;
      pause = line.type === 'cmd' ? 8 : 4;
      setTimeout(tick, line.type === 'cmd' ? 300 : 80);
    }
  }

  // start after a short delay so page load completes
  setTimeout(tick, 800);
})();
