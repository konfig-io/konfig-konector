/* ── Sidebar Data ────────────────────────────────────────────────────────────── */
const SIDEBAR_DATA = [
  {
    id: 'iam',
    label: 'IAM',
    href: '/docs/iam.html',
    items: [
      { label: 'IAMRole', anchor: 'iamrole' },
      { label: 'IAMPolicy', anchor: 'iampolicy' },
      { label: 'IAMPolicyAttachment', anchor: 'iampolicyattachment' },
      { label: 'IAMRolePolicy', anchor: 'iamrolepolicy' },
      { label: 'PodIdentityAssociation', anchor: 'podidentityassociation' },
      { label: 'IAMUser', anchor: 'iamuser' },
      { label: 'IAMGroup', anchor: 'iamgroup' },
      { label: 'IAMGroupPolicyAttachment', anchor: 'iamgrouppolicyattachment' },
      { label: 'IAMGroupMembership', anchor: 'iamgroupmembership' },
      { label: 'IAMSAMLProvider', anchor: 'iamsamlprovider' },
      { label: 'IAMOIDCProvider', anchor: 'iamoidcprovider' },
    ]
  },
  {
    id: 'networking',
    label: 'Networking',
    href: '/docs/networking.html',
    items: [
      { label: 'VPC', anchor: 'vpc' },
      { label: 'Subnet', anchor: 'subnet' },
      { label: 'InternetGateway', anchor: 'internetgateway' },
      { label: 'RouteTable', anchor: 'routetable' },
      { label: 'NatGateway', anchor: 'natgateway' },
      { label: 'SecurityGroup', anchor: 'securitygroup' },
      { label: 'VPCEndpoint', anchor: 'vpcendpoint' },
    ]
  },
  {
    id: 'compute',
    label: 'Compute',
    href: '/docs/compute.html',
    items: [
      { label: 'KeyPair', anchor: 'keypair' },
      { label: 'LaunchTemplate', anchor: 'launchtemplate' },
      { label: 'AutoScalingGroup', anchor: 'autoscalinggroup' },
      { label: 'EC2Instance', anchor: 'ec2instance' },
    ]
  },
  {
    id: 'rds',
    label: 'Database',
    href: '/docs/rds.html',
    items: [
      { label: 'DBSubnetGroup', anchor: 'dbsubnetgroup' },
      { label: 'DBParameterGroup', anchor: 'dbparametergroup' },
      { label: 'DBClusterParameterGroup', anchor: 'dbclusterparametergroup' },
      { label: 'DBInstance', anchor: 'dbinstance' },
      { label: 'DBCluster', anchor: 'dbcluster' },
    ]
  },
  {
    id: 'storage',
    label: 'Storage',
    href: '/docs/storage.html',
    items: [
      { label: 'S3Bucket', anchor: 's3bucket' },
      { label: 'S3BucketPolicy', anchor: 's3bucketpolicy' },
    ]
  },
  {
    id: 'messaging',
    label: 'Messaging',
    href: '/docs/messaging.html',
    items: [
      { label: 'SQSQueue', anchor: 'sqsqueue' },
      { label: 'SNSTopic', anchor: 'snstopic' },
      { label: 'SNSSubscription', anchor: 'snssubscription' },
    ]
  },
  {
    id: 'elasticache',
    label: 'ElastiCache',
    href: '/docs/elasticache.html',
    items: [
      { label: 'ElastiCacheSubnetGroup', anchor: 'elasticachesubnetgroup' },
      { label: 'ElastiCacheReplicationGroup', anchor: 'elasticachereplicationgroup' },
    ]
  },
  {
    id: 'route53',
    label: 'Route53 / DNS',
    href: '/docs/route53.html',
    items: [
      { label: 'HostedZone', anchor: 'hostedzone' },
      { label: 'RecordSet', anchor: 'recordset' },
      { label: 'HealthCheck', anchor: 'healthcheck' },
    ]
  },
  {
    id: 'eks',
    label: 'EKS',
    href: '/docs/eks.html',
    items: [
      { label: 'EKSCluster', anchor: 'ekscluster' },
      { label: 'EKSNodeGroup', anchor: 'eksnodegroup' },
      { label: 'EKSAddon', anchor: 'eksaddon' },
      { label: 'EKSFargateProfile', anchor: 'eksfargateprofile' },
      { label: 'EKSAccessEntry', anchor: 'eksaccessentry' },
    ]
  },
  {
    id: 'lambda-ecs',
    label: 'Lambda & ECS',
    href: '/docs/lambda-ecs.html',
    items: [
      { label: 'LambdaFunction', anchor: 'lambdafunction' },
      { label: 'LambdaEventSourceMapping', anchor: 'lambdaeventsourcemapping' },
      { label: 'LambdaPermission', anchor: 'lambdapermission' },
      { label: 'ECSCluster', anchor: 'ecscluster' },
      { label: 'ECSTaskDefinition', anchor: 'ecstaskdefinition' },
      { label: 'ECSService', anchor: 'ecsservice' },
    ]
  },
];

/* ── Determine current page ──────────────────────────────────────────────────── */
function getCurrentPageId() {
  const path = window.location.pathname;
  if (path === '/docs/' || path === '/docs/index.html') return 'overview';
  const match = path.match(/\/docs\/([^/]+)\.html$/);
  return match ? match[1] : 'overview';
}

/* ── Build Sidebar HTML ──────────────────────────────────────────────────────── */
function buildSidebar() {
  const sidebar = document.getElementById('docs-sidebar');
  if (!sidebar) return;

  const currentPage = getCurrentPageId();

  let html = `
    <a class="sidebar-overview${currentPage === 'overview' ? ' active' : ''}" href="/docs/">
      Overview
    </a>
    <div class="sidebar-divider"></div>
    <div class="docs-section-divider">Resources</div>
  `;

  SIDEBAR_DATA.forEach(group => {
    const isCurrentPage = group.id === currentPage;
    const isOpen = isCurrentPage;

    html += `<div class="sidebar-group${isOpen ? ' open' : ''}" data-group="${group.id}">`;
    html += `<a class="sidebar-section-title${isCurrentPage ? ' active' : ''}" href="${group.href}">
      ${group.label}
      <span class="sidebar-arrow">▼</span>
    </a>`;
    html += `<ul class="sidebar-items">`;

    group.items.forEach(item => {
      const href = isCurrentPage
        ? `#${item.anchor}`
        : `${group.href}#${item.anchor}`;
      html += `<li><a href="${href}" data-anchor="${item.anchor}">${item.label}</a></li>`;
    });

    html += `</ul></div>`;
  });

  sidebar.innerHTML = html;

  // Attach toggle listeners for section titles
  sidebar.querySelectorAll('.sidebar-section-title').forEach(title => {
    title.addEventListener('click', function(e) {
      const group = this.closest('.sidebar-group');
      if (!group) return;
      // If clicking the link of the current page, just toggle open; otherwise navigate
      const href = this.getAttribute('href');
      const isCurrentPage = href === window.location.pathname ||
                            (href === '/docs/' && window.location.pathname.endsWith('/docs/')) ||
                            window.location.pathname.endsWith(href);
      if (isCurrentPage) {
        e.preventDefault();
        group.classList.toggle('open');
      }
      // else: let the link navigate normally
    });
  });
}

/* ── Scroll Spy ──────────────────────────────────────────────────────────────── */
function initScrollSpy() {
  const sections = document.querySelectorAll('.resource-section[id]');
  if (!sections.length) return;

  const sidebarLinks = document.querySelectorAll('.sidebar-items li a[data-anchor]');

  function onScroll() {
    const scrollY = window.scrollY + 80; // offset for nav height

    let current = null;
    sections.forEach(section => {
      if (section.offsetTop <= scrollY) {
        current = section.id;
      }
    });

    sidebarLinks.forEach(link => {
      const anchor = link.getAttribute('data-anchor');
      link.classList.toggle('active', anchor === current);
    });

    // Scroll the active sidebar link into view (within sidebar)
    const activeLink = document.querySelector('.sidebar-items li a.active');
    if (activeLink) {
      const sidebar = document.getElementById('docs-sidebar');
      if (sidebar) {
        const linkTop = activeLink.offsetTop;
        const sidebarHeight = sidebar.clientHeight;
        const sidebarScroll = sidebar.scrollTop;
        if (linkTop < sidebarScroll + 60 || linkTop > sidebarScroll + sidebarHeight - 60) {
          sidebar.scrollTo({ top: linkTop - sidebarHeight / 2 + 20, behavior: 'smooth' });
        }
      }
    }
  }

  window.addEventListener('scroll', onScroll, { passive: true });
  onScroll(); // run once on load
}

/* ── Copy Buttons ────────────────────────────────────────────────────────────── */
function initCopyButtons() {
  document.querySelectorAll('.code-copy-btn').forEach(btn => {
    btn.addEventListener('click', function() {
      const codeEl = this.closest('.code-wrap')?.querySelector('code');
      if (!codeEl) return;

      const text = codeEl.innerText || codeEl.textContent;
      navigator.clipboard.writeText(text).then(() => {
        this.textContent = 'copied!';
        this.classList.add('copied');
        setTimeout(() => {
          this.textContent = 'copy';
          this.classList.remove('copied');
        }, 2000);
      }).catch(() => {
        // Fallback for older browsers
        const textarea = document.createElement('textarea');
        textarea.value = text;
        textarea.style.position = 'fixed';
        textarea.style.opacity = '0';
        document.body.appendChild(textarea);
        textarea.select();
        document.execCommand('copy');
        document.body.removeChild(textarea);
        this.textContent = 'copied!';
        this.classList.add('copied');
        setTimeout(() => {
          this.textContent = 'copy';
          this.classList.remove('copied');
        }, 2000);
      });
    });
  });
}

/* ── Mobile Sidebar Toggle ───────────────────────────────────────────────────── */
function initMobileToggle() {
  const toggle = document.getElementById('sidebar-toggle');
  const sidebar = document.getElementById('docs-sidebar');
  const overlay = document.getElementById('sidebar-overlay');

  if (!toggle || !sidebar) return;

  function openSidebar() {
    sidebar.classList.add('open');
    if (overlay) overlay.classList.add('open');
    toggle.textContent = '✕';
    document.body.style.overflow = 'hidden';
  }

  function closeSidebar() {
    sidebar.classList.remove('open');
    if (overlay) overlay.classList.remove('open');
    toggle.textContent = '☰';
    document.body.style.overflow = '';
  }

  toggle.addEventListener('click', () => {
    if (sidebar.classList.contains('open')) {
      closeSidebar();
    } else {
      openSidebar();
    }
  });

  if (overlay) {
    overlay.addEventListener('click', closeSidebar);
  }

  // Close sidebar when clicking a link (on mobile)
  sidebar.querySelectorAll('a').forEach(link => {
    link.addEventListener('click', () => {
      if (window.innerWidth <= 900) {
        closeSidebar();
      }
    });
  });
}

/* ── Nav scroll effect ───────────────────────────────────────────────────────── */
function initNavScroll() {
  const nav = document.querySelector('nav');
  if (!nav) return;
  // Docs pages always show scrolled nav since content starts immediately
  nav.classList.add('scrolled');
}

/* ── Init ────────────────────────────────────────────────────────────────────── */
document.addEventListener('DOMContentLoaded', () => {
  buildSidebar();
  initScrollSpy();
  initCopyButtons();
  initMobileToggle();
  initNavScroll();

  // Re-run highlight.js on dynamically-highlighted blocks (it's called in HTML too)
  if (window.hljs) {
    hljs.highlightAll();
    // Re-init copy buttons after hljs potentially rewrites the DOM
    initCopyButtons();
  }
});
