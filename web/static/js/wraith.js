(()=>{
'use strict';
const $=(s,p)=>(p||document).querySelector(s);
const $$=(s,p)=>[...(p||document).querySelectorAll(s)];
const API='/api';
let _poll=null,_term=null,_yamlTimer=null,_diskStatus=null;

async function api(path,opts={}){
  const cfg={headers:{'Content-Type':'application/json',...opts.headers},...opts};
  if(opts.body&&typeof opts.body==='object')cfg.body=JSON.stringify(opts.body);
  const r=await fetch(`${API}${path}`,cfg);
  if(r.status===401){location.href='/login.html';throw new Error('Unauthorized')}
  if(!r.ok){const d=await r.json().catch(()=>({}));throw new Error(d.error||`HTTP ${r.status}`)}
  return(r.headers.get('content-type')||'').includes('json')?r.json():r.text();
}

function toast(msg,type='info'){
  const c=$('#toast-container'),el=document.createElement('div');
  el.className=`toast toast-${type}`;el.textContent=msg;c.appendChild(el);
  setTimeout(()=>{el.classList.add('toast-exit');setTimeout(()=>el.remove(),200)},3500);
}

const pages=['dashboard','compose','mounts','files','network','settings'];
let cur='dashboard';

function stopPoll(){if(_poll){clearInterval(_poll);_poll=null}}
function startPoll(fn,ms=5000){stopPoll();fn();_poll=setInterval(fn,ms)}

window.navigate=function(p){
  if(!pages.includes(p))return;cur=p;
  $$('.nav-item').forEach(el=>el.classList.toggle('active',el.dataset.page===p));
  $('#sidebar').classList.remove('open');stopPoll();
  if(p!=='compose'&&_term){_term.destroy();_term=null}
  const m=$('#main-content');m.style.opacity='0';
  setTimeout(()=>{({dashboard:pgDash,compose:pgCompose,mounts:pgMounts,files:pgFiles,network:pgNetwork,settings:pgSettings}[p]||pgDash)();m.style.opacity='1'},80);
};
window.toggleSidebar=function(){$('#sidebar').classList.toggle('open')};
window.logout=async function(){try{await api('/auth/logout',{method:'POST'})}catch{}location.href='/login.html'};

function esc(s){if(!s)return'';const d=document.createElement('div');d.textContent=s;return d.innerHTML}
function fB(b){if(b<1024)return b+' B';if(b<1048576)return(b/1024).toFixed(1)+' KB';if(b<1073741824)return(b/1048576).toFixed(1)+' MB';return(b/1073741824).toFixed(1)+' GB'}
function fU(s){const d=Math.floor(s/86400),h=Math.floor(s%86400/3600),m=Math.floor(s%3600/60);return d>0?`${d}d ${h}h ${m}m`:h>0?`${h}h ${m}m`:`${m}m`}
function bC(p){return p>90?'danger':p>70?'warn':''}
function sC(s){s=(s||'').toLowerCase();return s==='running'?'status-running':s==='restarting'?'status-restarting':'status-stopped'}
function skel(n=1){let h='';for(let i=0;i<n;i++)h+=`<div class="skeleton skeleton-card" style="animation-delay:${i*100}ms"></div>`;return h}

// === ICONS (inline SVG snippets) ===
const ic={
  restart:'<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="23 4 23 10 17 10"/><path d="M20.49 15a9 9 0 11-2.12-9.36L23 10"/></svg>',
  stop:'<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="6" y="6" width="12" height="12" rx="1"/></svg>',
  play:'<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polygon points="5 3 19 12 5 21 5 3"/></svg>',
  check:'<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="20 6 9 17 4 12"/></svg>',
  save:'<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M19 21H5a2 2 0 01-2-2V5a2 2 0 012-2h11l5 5v11a2 2 0 01-2 2z"/><polyline points="17 21 17 13 7 13 7 21"/><polyline points="7 3 7 8 15 8"/></svg>',
  send:'<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M22 2L11 13"/><polygon points="22 2 15 22 11 13 2 9 22 2"/></svg>',
  plus:'<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>',
  dl:'<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15v4a2 2 0 01-2 2H5a2 2 0 01-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>',
  ul:'<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15v4a2 2 0 01-2 2H5a2 2 0 01-2-2v-4"/><polyline points="17 8 12 3 7 8"/><line x1="12" y1="3" x2="12" y2="15"/></svg>',
  box:'<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="2" y="6" width="20" height="12" rx="2"/><path d="M6 6V4a2 2 0 012-2h8a2 2 0 012 2v2"/></svg>',
};

// ============ DASHBOARD ============
function pgDash(){
  $('#main-content').innerHTML=`
<div id="tmpfs-banner"></div>
<div id="expand-banner"></div>
<div class="page-header"><h1 class="page-title">Dashboard</h1>
<div class="page-actions">
<button class="btn btn-sm btn-secondary" onclick="dashAct('restart')" id="btn-restart-stack">${ic.restart} Restart Stack</button>
<button class="btn btn-sm btn-danger" onclick="dashAct('stop')" id="btn-stop-stack">${ic.stop} Stop</button>
<button class="btn btn-sm btn-primary" onclick="dashAct('start')" id="btn-start-stack">${ic.play} Start</button>
</div></div>
<div class="stats-grid stagger" id="stats-grid">${skel(4)}</div>
<div class="card" style="margin-bottom:24px"><div class="card-header"><div class="card-title">Network</div></div>
<div class="network-grid" id="net-grid"><div class="skeleton skeleton-text"></div></div></div>
<h2 style="font-size:1.1rem;font-weight:600;margin-bottom:16px">Containers</h2>
<div class="container-grid stagger" id="container-grid">${skel(3)}</div>`;
  startPoll(fetchDash,5000);
  checkDiskStatus();
  checkExpandableDisks();
}

async function fetchDash(){
  try{
    const d=await api('/system/status');
    updStats(d.system||{});updNet(d.network||{});updCont(d.containers||[]);
    if(d.system&&d.system.uptime)$('#topbar-uptime').textContent=fU(d.system.uptime);
  }catch{}
}

async function checkDiskStatus(){
  try{
    const s=await api('/setup/status');
    _diskStatus=s;
    const banner=$('#tmpfs-banner');
    if(!banner)return;
    const cfgTmpfs=s.configDisk&&!s.configDisk.persistent;
    const cacheTmpfs=s.cacheDisk&&!s.cacheDisk.persistent;
    if(cfgTmpfs||cacheTmpfs){
      banner.innerHTML=`<div class="tmpfs-warning-banner animate-in">
<span class="tmpfs-warning-icon">&#9888;</span>
<span>Running on temporary storage &mdash; data will not survive reboot.</span>
<a href="#" class="tmpfs-warning-link" onclick="event.preventDefault();setupWizard.show()">Set up disks</a>
</div>`;
    }else{
      banner.innerHTML='';
    }
  }catch{}
}

async function checkExpandableDisks(){
  try{
    const r=await api('/system/disks/expandable');
    const banner=$('#expand-banner');
    if(!banner)return;
    const disks=r.disks||[];
    if(disks.length===0){banner.innerHTML='';return}
    banner.innerHTML=disks.map(d=>{
      const role=d.role==='single'?'Disk':d.role==='config'?'Config Disk':'Cache Disk';
      return `<div class="expand-banner animate-in" style="background:var(--ac-glow);border:1px solid var(--ac-dim);border-radius:var(--r-md);padding:12px 16px;margin-bottom:12px;display:flex;align-items:center;gap:12px;flex-wrap:wrap">
<span style="font-size:1.2rem">&#9889;</span>
<span style="flex:1">${esc(role)} (${esc(d.device)}) has <strong>${fB(d.growBytes)}</strong> unused space available</span>
<button class="btn btn-sm btn-primary" onclick="expandDisk('${esc(d.device)}')">Expand Filesystem</button>
</div>`}).join('');
  }catch{}
}

async function expandDisk(device){
  if(!confirm('Expand the filesystem on '+device+'? This will grow the filesystem to use all available space on the device.'))return;
  try{
    await api('/system/disks/expand',{method:'POST',body:{device}});
    toast('Filesystem expanded successfully','success');
    checkExpandableDisks();
    // Refresh dashboard stats to show new disk size
    fetchDash();
  }catch(e){
    toast('Expand failed: '+e.message,'error');
  }
}

function updStats(s){
  const g=$('#stats-grid');if(!g)return;
  const cp=s.cpuPercent||0,ru=s.ramUsed||0,rt=s.ramTotal||1,rp=Math.round(ru/rt*100),
    cu=s.configDiskUsed||0,ct=s.configDiskTotal||1,cpp=Math.round(cu/ct*100),
    du=s.cacheDiskUsed||0,dt=s.cacheDiskTotal||1,dp=Math.round(du/dt*100);
  g.innerHTML=[
    ['CPU Usage',cp.toFixed(1)+'%',cp,''],
    ['Memory',rp+'%',rp,`${fB(ru)} / ${fB(rt)}`],
    ['Config Disk',cpp+'%',cpp,`${fB(cu)} / ${fB(ct)}`],
    ['Cache Disk',dp+'%',dp,`${fB(du)} / ${fB(dt)}`]
  ].map(([l,v,p,d])=>`<div class="stat-card animate-up"><div class="stat-label">${l}</div>
<div class="stat-value">${v}</div><div class="stat-bar"><div class="stat-bar-fill ${bC(p)}" style="width:${p}%"></div></div>
${d?`<div class="stat-detail">${d}</div>`:''}</div>`).join('');
}

function updNet(n){
  const g=$('#net-grid');if(!g)return;
  g.innerHTML=[['IP Address',n.ip],['Gateway',n.gateway],['DNS',(n.dns||[]).join(', ')],
    ['Mode',n.dhcp?'DHCP':'Static'],['Interface',n.interface||'eth0']]
    .map(([l,v])=>`<div class="net-item"><span class="net-label">${l}</span><span class="net-value">${v||'--'}</span></div>`).join('');
}

function updCont(cs){
  const g=$('#container-grid');if(!g)return;
  if(!cs.length){g.innerHTML=`<div class="empty-state" style="grid-column:1/-1">${ic.box}<h3>No containers</h3><p>Deploy a compose stack to see containers here.</p></div>`;return}
  g.innerHTML=cs.map((c,i)=>`<div class="container-card" style="animation-delay:${i*60}ms">
<div class="container-header"><div class="container-name">${ic.box} ${esc(c.name)}</div>
<span class="container-status ${sC(c.state)}"><span class="dot"></span>${c.state||'unknown'}</span></div>
<dl class="container-meta"><dt>Image</dt><dd>${esc(c.image||'--')}</dd><dt>Uptime</dt><dd>${c.uptime?fU(c.uptime):'--'}</dd>
<dt>Ports</dt><dd>${(c.ports||[]).join(', ')||'none'}</dd></dl>
<div class="container-actions">
<button class="btn btn-sm btn-secondary" data-cont-action="restart" data-cont-name="${esc(c.name)}">Restart</button>
${c.state==='running'?`<button class="btn btn-sm btn-secondary" data-cont-action="stop" data-cont-name="${esc(c.name)}">Stop</button>`
:`<button class="btn btn-sm btn-primary" data-cont-action="start" data-cont-name="${esc(c.name)}">Start</button>`}
<button class="btn btn-sm btn-secondary" data-cont-action="logs" data-cont-name="${esc(c.name)}" style="margin-left:auto">Logs</button>
</div></div>`).join('');
  // Event delegation for container action buttons
  g.addEventListener('click',function(e){const btn=e.target.closest('[data-cont-action]');if(btn)contAct(btn.dataset.contName,btn.dataset.contAction)});
}

window.dashAct=async function(a){
  const b=$(`#btn-${a}-stack`);if(b)b.disabled=true;
  // Show a temporary inline terminal for streaming output
  let termBox=$('#dash-term-container');
  if(!termBox){termBox=document.createElement('div');termBox.id='dash-term-container';termBox.style.cssText='margin-bottom:24px';
    const grid=$('#stats-grid');if(grid)grid.parentNode.insertBefore(termBox,grid);else $('#main-content').prepend(termBox)}
  const term=new WraithTerminal(termBox);
  term.clear();term.writeLine(`Stack ${a}...`,'info');
  toast(`Stack ${a} started`,'info');
  const result=await term.connectSSE(`/api/compose/${a}`);
  if(result.success){toast(`Stack ${a} completed`,'success')}
  else{toast(`Stack ${a} failed: ${result.error||'unknown'}`,'error')}
  if(b)b.disabled=false;
  setTimeout(fetchDash,1000);
};

window.contAct=async function(name,a){
  if(a==='logs'){navigate('compose');setTimeout(async()=>{if(_term){_term.clear();try{const d=await api(`/containers/${encodeURIComponent(name)}/logs`);_term.write(d.logs||d||'No logs')}catch(e){_term.writeLine(`Error: ${e.message}`,'err')}}},200);return}
  try{await api(`/containers/${encodeURIComponent(name)}/${a}`,{method:'POST'});toast(`${name}: ${a} OK`,'success');setTimeout(fetchDash,1500)}
  catch(e){toast(`${name}: ${e.message}`,'error')}
};

// ============ COMPOSE EDITOR ============
function pgCompose(){
  $('#main-content').innerHTML=`<div class="compose-page">
<div class="compose-header">
<div class="compose-header-left">
<h1 class="page-title" style="font-size:1.2rem">Compose Editor</h1>
<div id="yaml-status" class="yaml-status hidden"></div>
</div>
<div class="compose-header-right">
<div class="compose-kbd-hints">
<span class="kbd-hint"><kbd>Ctrl</kbd>+<kbd>S</kbd> Save</span>
<span class="kbd-hint"><kbd>Tab</kbd> Indent</span>
</div>
<button class="btn btn-sm btn-secondary" onclick="compAct('validate')" id="btn-validate">${ic.check} Validate</button>
<button class="btn btn-sm btn-secondary" onclick="compAct('save')" id="btn-save">${ic.save} Save</button>
<button class="btn btn-sm btn-primary" onclick="compAct('deploy')" id="btn-deploy">${ic.send} Deploy</button>
</div>
</div>
<div class="compose-body">
<div class="compose-editor-col">
<div id="compose-hint-banner"></div>
<div class="compose-settings-bar" id="compose-settings-bar">
<div class="toggle-wrap"><label class="toggle"><input type="checkbox" id="require-mounts-toggle" onchange="toggleRequireMounts(this.checked)"><span class="toggle-track"></span><span class="toggle-thumb"></span></label>
<div style="display:flex;flex-direction:column;gap:2px"><span class="toggle-label" style="font-size:.85rem">Require network mounts before deploy</span>
<span style="font-size:.75rem;color:var(--tx-m)">When enabled, containers won't start unless all configured mounts are available</span></div></div>
</div>
<div class="editor-container"><div class="editor-wrap"><div class="line-numbers" id="line-numbers" aria-hidden="true"></div><textarea id="compose-editor" class="compose-textarea" spellcheck="false" placeholder="Loading docker-compose.yml..."></textarea></div></div>
<div id="terminal-container"></div>
</div>
<aside class="compose-sidebar" id="compose-sidebar">
<div class="compose-sidebar-toggle" onclick="toggleComposeSidebar()">
<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 3H3v18h18V3z"/><path d="M15 3v18"/></svg>
<span>Paths</span>
</div>
<div class="compose-sidebar-inner" id="compose-sidebar-inner">
<div class="sidebar-section">
<h3 class="sidebar-section-title">
<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M22 19a2 2 0 01-2 2H4a2 2 0 01-2-2V5a2 2 0 012-2h5l2 3h9a2 2 0 012 2z"/></svg>
Storage Paths
</h3>
<div class="path-cards" id="path-cards-storage">
<div class="path-card">
<div class="path-card-header"><code class="path-code">/wraith/cache</code><span class="path-badge" id="badge-cache">checking</span></div>
<p class="path-desc">Docker volumes and container data. Use for databases, app state, and anything that should persist across container restarts.</p>
<div class="path-example"><span class="path-example-label">Volume example</span>
<pre class="path-snippet">volumes:\n  - /wraith/cache/myapp:/data</pre>
<button class="btn-copy" onclick="copySnippet(this)" title="Copy"><svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 01-2-2V4a2 2 0 012-2h9a2 2 0 012 2v1"/></svg></button>
</div>
</div>
<div class="path-card">
<div class="path-card-header"><code class="path-code">/wraith/config</code><span class="path-badge badge-system">system</span></div>
<p class="path-desc">WraithOS configuration. Compose files, auth, and network settings live here. Avoid mounting directly into containers.</p>
</div>
</div>
</div>
<div class="sidebar-section">
<h3 class="sidebar-section-title">
<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="2" y1="12" x2="22" y2="12"/><path d="M12 2a15.3 15.3 0 014 10 15.3 15.3 0 01-4 10 15.3 15.3 0 01-4-10 15.3 15.3 0 014-10z"/></svg>
Network Mounts
</h3>
<div id="path-cards-mounts" class="path-cards"><div class="skeleton skeleton-text" style="width:80%"></div><div class="skeleton skeleton-text" style="width:60%"></div></div>
</div>
<div class="sidebar-section">
<h3 class="sidebar-section-title">
<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><path d="M9.09 9a3 3 0 015.83 1c0 2-3 3-3 3"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg>
Tips
</h3>
<ul class="sidebar-tips">
<li>WraithOS boots from RAM. Only <code>/wraith/cache</code> and <code>/mnt/*</code> survive reboots (when on persistent disks).</li>
<li>Named Docker volumes are stored under <code>/wraith/cache</code> automatically.</li>
<li>Use bind mounts for network storage: <code>/mnt/&lt;name&gt;:/container/path</code></li>
<li>Set <code>restart: unless-stopped</code> so containers come back after reboot.</li>
</ul>
</div>
</div>
</aside>
</div>
</div>`;
  _term=new WraithTerminal($('#terminal-container'));
  loadCompose();
  loadComposeHint();
  loadComposeSidebar();
  loadComposeSettings();
  const ta=$('#compose-editor');
  ta.addEventListener('keydown',e=>{
    if(e.key==='Tab'){e.preventDefault();const s=ta.selectionStart,en=ta.selectionEnd;ta.value=ta.value.substring(0,s)+'  '+ta.value.substring(en);ta.selectionStart=ta.selectionEnd=s+2;updateLineNumbers()}
    if((e.ctrlKey||e.metaKey)&&e.key==='s'){e.preventDefault();compAct('save')}
  });
  ta.addEventListener('input',function(){
    updateLineNumbers();
    if(_yamlTimer)clearTimeout(_yamlTimer);
    _yamlTimer=setTimeout(()=>validateYAMLClient(ta.value),500);
  });
  ta.addEventListener('scroll',syncLineScroll);
  // Initial line numbers once content loads
  setTimeout(updateLineNumbers,200);
}

async function loadComposeHint(){
  try{
    const s=_diskStatus||await api('/setup/status');
    _diskStatus=s;
    const banner=$('#compose-hint-banner');
    if(!banner)return;
    const cachePersist=s.cacheDisk&&s.cacheDisk.persistent;
    // Update sidebar badge for cache disk
    const cacheBadge=$('#badge-cache');
    if(cacheBadge){
      if(cachePersist){cacheBadge.textContent='persistent';cacheBadge.className='path-badge badge-ok'}
      else{cacheBadge.textContent='tmpfs';cacheBadge.className='path-badge badge-warn'}
    }
    if(!cachePersist&&s.cacheDisk){
      banner.innerHTML=`<div class="compose-hint-banner hint-warn">
<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M10.29 3.86L1.82 18a2 2 0 001.71 3h16.94a2 2 0 001.71-3L13.71 3.86a2 2 0 00-3.42 0z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg>
<span>Cache disk is on temporary storage. Volumes will <strong>not</strong> persist across reboots. <a href="#" onclick="event.preventDefault();setupWizard.show()">Set up disks</a></span>
<button class="btn-icon compose-hint-dismiss" onclick="this.closest('.compose-hint-banner').remove()" title="Dismiss" aria-label="Dismiss hint">
<svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
</button></div>`;
    }
  }catch{}
}

async function loadComposeSidebar(){
  try{
    const d=await api('/mounts'),ms=d.mounts||d||[];
    const el=$('#path-cards-mounts');if(!el)return;
    if(!ms.length){
      el.innerHTML=`<div class="path-card path-card-empty">
<p class="path-desc">No network mounts configured yet.</p>
<button class="btn btn-sm btn-secondary" onclick="navigate('mounts')" style="margin-top:8px">${ic.plus} Add Mount</button>
</div>`;return}
    el.innerHTML=ms.map(m=>{
      const mp=esc(m.mountpoint||m.path);
      const src=m.type==='nfs'?`${esc(m.server)}:${esc(m.share)}`:`//${esc(m.server)}/${esc(m.share)}`;
      const mounted=m.mounted;
      return`<div class="path-card">
<div class="path-card-header"><code class="path-code">${mp}</code><span class="path-badge ${mounted?'badge-ok':'badge-warn'}">${mounted?'mounted':'unmounted'}</span></div>
<p class="path-desc dim">${(m.type||'cifs').toUpperCase()} from ${src}</p>
<div class="path-example"><span class="path-example-label">Bind mount</span>
<pre class="path-snippet">volumes:\n  - ${mp}:/container/path</pre>
<button class="btn-copy" onclick="copySnippet(this)" title="Copy"><svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 01-2-2V4a2 2 0 012-2h9a2 2 0 012 2v1"/></svg></button>
</div></div>`}).join('');
  }catch{
    const el=$('#path-cards-mounts');if(el)el.innerHTML='<p class="dim" style="font-size:.82rem">Could not load mounts.</p>';
  }
}

function updateLineNumbers(){
  const ta=$('#compose-editor'),ln=$('#line-numbers');
  if(!ta||!ln)return;
  const lines=ta.value.split('\n').length;
  let h='';for(let i=1;i<=lines;i++)h+=i+'\n';
  ln.textContent=h;
  syncLineScroll();
}

function syncLineScroll(){
  const ta=$('#compose-editor'),ln=$('#line-numbers');
  if(ta&&ln)ln.scrollTop=ta.scrollTop;
}

window.toggleComposeSidebar=function(){
  const sb=$('#compose-sidebar');
  if(sb)sb.classList.toggle('collapsed');
};

window.copySnippet=function(btn){
  const pre=btn.closest('.path-example').querySelector('.path-snippet');
  if(!pre)return;
  const text=pre.textContent.replace(/\\n/g,'\n');
  navigator.clipboard.writeText(text).then(()=>{
    btn.classList.add('copied');
    setTimeout(()=>btn.classList.remove('copied'),1200);
    toast('Copied to clipboard','success');
  }).catch(()=>toast('Copy failed','error'));
};

function validateYAMLClient(content){
  const el=$('#yaml-status');
  if(!el)return;
  if(!content||!content.trim()){el.classList.add('hidden');return}
  if(typeof jsyaml==='undefined'){el.classList.add('hidden');return}
  try{
    jsyaml.load(content);
    el.classList.remove('hidden');
    el.className='yaml-status yaml-valid';
    el.innerHTML=ic.check+' Valid YAML syntax';
  }catch(e){
    el.classList.remove('hidden');
    el.className='yaml-status yaml-error';
    const line=e.mark?e.mark.line+1:'?';
    const msg=e.reason||e.message||'Unknown error';
    el.textContent=`YAML Error: line ${line} \u2014 ${msg}`;
  }
}

async function loadCompose(){
  try{const d=await api('/compose/file');const e=$('#compose-editor');if(e)e.value=d.content||d||''}
  catch(x){const e=$('#compose-editor');if(e)e.placeholder='No compose file found. Paste your docker-compose.yml here.'}
}

async function loadComposeSettings(){
  try{
    const d=await api('/compose/settings');
    const cb=$('#require-mounts-toggle');
    if(cb)cb.checked=!!d.requireMounts;
  }catch{}
}

window.toggleRequireMounts=async function(enabled){
  const cb=$('#require-mounts-toggle');
  try{
    await api('/compose/settings',{method:'PUT',body:{requireMounts:enabled}});
    toast(enabled?'Mounts required before deploy':'Mount requirement disabled','success');
  }catch(e){
    toast('Failed to update setting: '+e.message,'error');
    if(cb)cb.checked=!enabled;
  }
};

window.compAct=async function(a){
  const btn=$(`#btn-${a}`),ed=$('#compose-editor');if(!ed)return;if(btn)btn.disabled=true;
  try{
    if(a==='save'){await api('/compose/file',{method:'PUT',body:{content:ed.value}});toast('Saved','success')}
    else if(a==='validate'){const r=await api('/compose/validate',{method:'POST',body:{content:ed.value}});
      if(r.valid){toast('Valid YAML','success');if(_term)_term.writeLine('Validation passed.','ok')}
      else{toast('Validation errors','error');if(_term)_term.writeLine(r.error||'Invalid YAML','err')}}
    else if(a==='deploy'){
      await api('/compose/file',{method:'PUT',body:{content:ed.value}});
      if(_term)_term.clear();
      toast('Deploy started','info');
      const result=await runDeployWithProgress();
      if(result&&!result.success&&result.error&&result.error.includes('mount')){
        if(_term)_term.writeLine('Go to Network Mounts to mount your shares, or disable "Require Mounts" above.','warn');
      }
    }
  }catch(e){toast(`Error: ${e.message}`,'error');if(_term)_term.writeLine(`Error: ${e.message}`,'err')}
  if(btn)btn.disabled=false;
};

// Run a phased deploy with progress tracking above the terminal
async function runDeployWithProgress(){
  let progressEl=$('#deploy-progress');
  if(!progressEl){
    progressEl=document.createElement('div');
    progressEl.id='deploy-progress';
    const termContainer=$('#terminal-container');
    if(termContainer)termContainer.parentNode.insertBefore(progressEl,termContainer);
    else return _term.connectSSE('/api/compose/deploy');
  }
  const state={phase:'init',services:[],images:[],pullStatus:{},serviceStatus:{},error:null};

  function renderProgress(){
    const phases=[
      {id:'pull',label:'Pull Images',icon:'<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15v4a2 2 0 01-2 2H5a2 2 0 01-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>'},
      {id:'create',label:'Create & Start',icon:'<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polygon points="5 3 19 12 5 21 5 3"/></svg>'}
    ];
    const phaseOrder=['init','pull','create','done'];
    const curIdx=phaseOrder.indexOf(state.phase);
    let html='<div class="deploy-progress-panel">';
    html+='<div class="deploy-phases">';
    phases.forEach((p,i)=>{
      const pIdx=phaseOrder.indexOf(p.id);
      let cls='deploy-phase';
      if(state.phase===p.id)cls+=' phase-active';
      else if(curIdx>pIdx)cls+=' phase-done';
      else cls+=' phase-pending';
      html+=`<div class="${cls}">${p.icon}<span>${p.label}</span></div>`;
      if(i<phases.length-1)html+='<div class="deploy-phase-arrow"><svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="9 18 15 12 9 6"/></svg></div>';
    });
    html+='</div>';
    if(state.phase==='pull'||curIdx>phaseOrder.indexOf('pull')){
      const imgs=state.images.length?state.images:Object.keys(state.pullStatus);
      if(imgs.length){
        html+='<div class="deploy-pull-section"><div class="deploy-section-title">Image Pull Progress</div>';
        imgs.forEach(img=>{
          const st=state.pullStatus[img]||'waiting';
          const shortImg=img.split('/').pop();
          let statusCls='pull-waiting',statusText='Waiting',statusIcon='';
          if(st==='pulling'){statusCls='pull-active';statusText='Pulling...';statusIcon='<span class="pull-spinner"></span>'}
          else if(st==='pulled'){statusCls='pull-done';statusText='Done';statusIcon='<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="20 6 9 17 4 12"/></svg>'}
          else if(st==='error'){statusCls='pull-error';statusText='Error'}
          html+=`<div class="deploy-pull-row ${statusCls}"><span class="pull-image-name" title="${esc(img)}">${esc(shortImg)}</span><span class="pull-status">${statusIcon} ${statusText}</span></div>`;
        });
        html+='</div>';
      }
    }
    if(state.phase==='create'||state.phase==='done'){
      const svcs=state.services.length?state.services:Object.keys(state.serviceStatus);
      if(svcs.length||Object.keys(state.serviceStatus).length){
        html+='<div class="deploy-service-section"><div class="deploy-section-title">Services</div>';
        const allSvcs=[...new Set([...svcs,...Object.keys(state.serviceStatus)])];
        allSvcs.forEach(svc=>{
          const st=state.serviceStatus[svc]||'waiting';
          let statusCls='svc-waiting',statusText='Waiting',statusIcon='';
          if(st==='created'){statusCls='svc-created';statusText='Created';statusIcon='<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="20 6 9 17 4 12"/></svg>'}
          else if(st==='started'||st==='exists'){statusCls='svc-started';statusText='Running';statusIcon='<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="20 6 9 17 4 12"/></svg>'}
          else if(st==='creating'){statusCls='svc-creating';statusText='Creating...';statusIcon='<span class="pull-spinner"></span>'}
          else if(st==='error'){statusCls='svc-error';statusText='Error'}
          html+=`<div class="deploy-svc-row ${statusCls}"><span class="svc-name">${esc(svc)}</span><span class="svc-status">${statusIcon} ${statusText}</span></div>`;
        });
        html+='</div>';
      }
    }
    if(state.error)html+=`<div class="deploy-error">${esc(state.error)}</div>`;
    html+='</div>';
    progressEl.innerHTML=html;
  }

  renderProgress();
  let result={success:false,error:null};
  try{
    const resp=await fetch('/api/compose/deploy',{method:'POST'});
    if(!resp.ok){
      const err=await resp.json().catch(()=>({error:'HTTP '+resp.status}));
      if(_term)_term.writeLine('Error: '+(err.error||'request failed'),'err');
      state.error=err.error||'request failed';renderProgress();
      return{success:false,error:state.error};
    }
    const reader=resp.body.getReader();
    const decoder=new TextDecoder();
    let buffer='';
    while(true){
      const{done,value}=await reader.read();
      if(done)break;
      buffer+=decoder.decode(value,{stream:true});
      const parts=buffer.split('\n\n');
      buffer=parts.pop();
      for(const part of parts){
        const line=part.replace(/^data: /,'').trim();
        if(!line)continue;
        try{
          const data=JSON.parse(line);
          if(data.type==='deploy_init'){
            if(data.services)state.services=data.services;
            if(data.images)state.images=data.images;
            renderProgress();
            if(_term)_term.writeLine('Deploying '+state.services.length+' service(s)...','info');
            continue;
          }
          if(data.type==='phase'){
            state.phase=data.phase;
            if(data.status==='done'&&data.phase==='create')state.phase='done';
            renderProgress();
            if(data.phase==='pull'&&data.status==='starting'&&_term)_term.writeLine('--- Pulling images ---','info');
            if(data.phase==='create'&&data.status==='starting'&&_term)_term.writeLine('--- Creating containers ---','info');
            continue;
          }
          if(data.type==='pull_progress'){
            if(data.service&&data.status){state.pullStatus[data.service]=data.status;renderProgress()}
            if(data.line&&_term){
              const cls=data.status==='pulled'?'ok':data.status==='pulling'?'info':'';
              _term.writeLine(data.line,cls);
            }
            continue;
          }
          if(data.type==='service'){
            if(data.service&&data.status){state.serviceStatus[data.service]=data.status;renderProgress()}
            if(data.line&&_term){
              const cls=data.status==='started'||data.status==='exists'?'ok':data.status==='created'?'info':'';
              _term.writeLine(data.line,cls);
            }
            continue;
          }
          if(data.type==='complete'){
            if(data.success){
              state.phase='done';renderProgress();
              if(_term)_term.writeLine('Deploy completed successfully.','ok');
              toast('Deploy completed','success');
              result={success:true};
            }else{
              state.error=data.error||'unknown error';renderProgress();
              if(_term)_term.writeLine('Deploy failed: '+state.error,'err');
              toast('Deploy failed: '+state.error,'error');
              result={success:false,error:state.error};
            }
            continue;
          }
          if(data.type==='warning'){if(data.line&&_term)_term.writeLine(data.line,'warn');continue}
          if(data.line&&_term){
            const cls=data.type==='error'?'err':data.type==='success'?'ok':data.type==='warning'?'warn':data.type==='pull'?'info':'';
            _term.writeLine(data.line,cls);
          }
        }catch{if(line&&_term)_term.write(line)}
      }
    }
  }catch(err){
    if(err.name==='AbortError')return{success:false,error:'cancelled'};
    if(_term)_term.writeLine('Stream error: '+err.message,'err');
    state.error=err.message;renderProgress();
    return{success:false,error:err.message};
  }
  if(result.success){
    setTimeout(()=>{
      const el=$('#deploy-progress');
      if(el)el.classList.add('deploy-progress-fade');
      setTimeout(()=>{if(el)el.remove()},300);
    },5000);
  }
  return result;
}

// ============ NETWORK MOUNTS ============
function pgMounts(){
  $('#main-content').innerHTML=`<div class="page-header"><h1 class="page-title">Network Mounts</h1>
<div class="page-actions"><button class="btn btn-sm btn-primary" onclick="showMntForm()">${ic.plus} Add Mount</button></div></div>
<div id="mnt-form" class="hidden" style="margin-bottom:24px"><div class="form-card">
<h3 style="font-size:.95rem;font-weight:600;margin-bottom:16px">New Network Mount</h3>
<div class="form-grid">
<div class="form-group" style="grid-column:1/-1"><label class="form-label">Mount Type</label>
<div style="display:flex;gap:16px;margin-top:4px">
<label style="display:flex;align-items:center;gap:6px;cursor:pointer"><input type="radio" name="mnt-type" value="cifs" checked onchange="mntTypeChanged()"> SMB/CIFS</label>
<label style="display:flex;align-items:center;gap:6px;cursor:pointer"><input type="radio" name="mnt-type" value="nfs" onchange="mntTypeChanged()"> NFS</label>
</div></div>
<div class="form-group"><label class="form-label">Server</label><input class="form-input" id="mnt-server" placeholder="MYSERVER or 192.168.1.100"><div class="form-hint">Hostname or IP of the file server</div></div>
<div class="form-group"><label class="form-label" id="mnt-share-label">Share</label><input class="form-input" id="mnt-share" placeholder="folder/path"><div class="form-hint" id="mnt-share-hint">Path relative to the share root</div></div>
<div class="form-group"><label class="form-label">Mount Name</label><input class="form-input" id="mnt-name" placeholder="my-share" oninput="mntNameChanged()">
<div class="form-hint" id="mnt-path-hint">Mounts at /mnt/&lt;name&gt;</div></div>
<div id="mnt-creds-group">
<div class="form-group"><label class="form-label">Username</label><input class="form-input" id="mnt-user" placeholder="optional"></div>
<div class="form-group"><label class="form-label">Password</label><input class="form-input" id="mnt-pass" type="password" placeholder="optional"></div>
</div>
<div class="form-group"><label class="form-label">Options</label><input class="form-input" id="mnt-opts" placeholder="ro,vers=3.0"><div class="form-hint" id="mnt-opts-hint">Additional mount options</div></div>
</div><div class="form-actions"><button class="btn btn-primary" onclick="addMnt()">Mount</button>
<button class="btn btn-secondary" onclick="hideMntForm()">Cancel</button></div></div></div>
<div class="mount-list" id="mount-list">${skel(2)}</div>`;
  fetchMnts();
}

async function fetchMnts(){
  try{
    const d=await api('/mounts'),ms=d.mounts||d||[],l=$('#mount-list');if(!l)return;
    if(!ms.length){l.innerHTML='<div class="empty-state"><h3>No mounts configured</h3><p>Add a network mount (SMB/CIFS or NFS) to share files with containers.</p></div>';return}
    l.innerHTML=ms.map((m,i)=>{const t=(m.type||'cifs').toUpperCase();const src=m.type==='nfs'?`${esc(m.server)}:${esc(m.share)}`:`//${esc(m.server)}/${esc(m.share)}`;
    return`<div class="mount-card" style="animation-delay:${i*60}ms"><div class="mount-info">
<h3><span class="container-status ${m.mounted?'status-running':'status-stopped'}"><span class="dot"></span>${m.mounted?'mounted':'unmounted'}</span> ${esc(m.mountpoint||m.path)}</h3>
<dl class="mount-details"><dt>Type</dt><dd>${t}</dd><dt>Source</dt><dd>${src}</dd>${m.type!=='nfs'?`<dt>User</dt><dd>${esc(m.username||'guest')}</dd>`:''}<dt>Options</dt><dd>${esc(m.options||'defaults')}</dd></dl>
${(m.volumes&&m.volumes.length)?`<div class="mount-volumes"><div class="label">Used by volumes</div>${m.volumes.map(v=>`<span class="volume-tag">${esc(v)}</span>`).join('')}</div>`:''}</div>
<div class="mount-actions">${m.mounted?`<button class="btn btn-sm btn-secondary" data-mnt-action="unmount" data-mnt-id="${esc(m.id||m.mountpoint)}">Unmount</button>`
:`<button class="btn btn-sm btn-primary" data-mnt-action="mount" data-mnt-id="${esc(m.id||m.mountpoint)}">Mount</button>`}
<button class="btn btn-sm btn-danger" data-mnt-action="delete" data-mnt-id="${esc(m.id||m.mountpoint)}">Remove</button></div></div>`}).join('');
    // Event delegation for mount action buttons
    l.addEventListener('click',function(e){const btn=e.target.closest('[data-mnt-action]');if(btn)mntAct(btn.dataset.mntId,btn.dataset.mntAction)});
  }catch(e){const l=$('#mount-list');if(l)l.innerHTML=`<div class="empty-state"><h3>Could not load mounts</h3><p>${esc(e.message)}</p></div>`}
}

window.showMntForm=function(){$('#mnt-form').classList.remove('hidden');mntTypeChanged()};
window.hideMntForm=function(){$('#mnt-form').classList.add('hidden')};
window.mntTypeChanged=function(){
  const t=document.querySelector('input[name="mnt-type"]:checked').value;
  const creds=$('#mnt-creds-group');
  const shareLabel=$('#mnt-share-label');
  const shareInput=$('#mnt-share');
  const optsHint=$('#mnt-opts-hint');
  const shareHint=$('#mnt-share-hint');
  if(t==='nfs'){
    if(creds)creds.style.display='none';
    if(shareLabel)shareLabel.textContent='Export Path';
    if(shareInput)shareInput.placeholder='/data/exports';
    if(shareHint)shareHint.textContent='Absolute path on the NFS server';
    if(optsHint)optsHint.textContent='Default: noatime,nfsvers=4';
  }else{
    if(creds)creds.style.display='';
    if(shareLabel)shareLabel.textContent='Share';
    if(shareInput)shareInput.placeholder='folder/path';
    if(shareHint)shareHint.textContent='Path relative to the share root';
    if(optsHint)optsHint.textContent='Additional mount options';
  }
};
window.mntNameChanged=function(){
  const v=$('#mnt-name').value;
  const hint=$('#mnt-path-hint');
  if(hint)hint.textContent=v?`Mounts at /mnt/${v}`:'Mounts at /mnt/<name>';
};
window.addMnt=async function(){
  const t=document.querySelector('input[name="mnt-type"]:checked').value;
  const d={type:t,server:$('#mnt-server').value,share:$('#mnt-share').value,mountpoint:$('#mnt-name').value,
    options:$('#mnt-opts').value};
  if(t==='cifs'){d.username=$('#mnt-user').value;d.password=$('#mnt-pass').value}
  if(!d.server||!d.share||!d.mountpoint){toast('Server, share, and mount name are required','error');return}
  if(!/^[a-zA-Z0-9_-]+$/.test(d.mountpoint)){toast('Mount name must be alphanumeric, hyphens, or underscores only','error');return}
  try{await api('/mounts',{method:'POST',body:d});toast('Mount added','success');hideMntForm();fetchMnts()}
  catch(e){toast(`Error: ${e.message}`,'error')}
};
window.mntAct=async function(id,a){
  try{if(a==='delete')await api(`/mounts/${encodeURIComponent(id)}`,{method:'DELETE'});
  else await api(`/mounts/${encodeURIComponent(id)}/${a}`,{method:'POST'});toast(`Mount ${a==='delete'?'removed':a+'ed'}`,'success');fetchMnts()}
  catch(e){toast(`Error: ${e.message}`,'error')}
};

// ============ NETWORK ============
function pgNetwork(){
  $('#main-content').innerHTML=`<div class="page-header"><h1 class="page-title">Network Settings</h1></div>
<div class="form-card"><div style="margin-bottom:24px"><div class="card-title" style="margin-bottom:16px">Current Configuration</div>
<div class="network-grid" id="net-current"><div class="skeleton skeleton-text"></div></div></div>
<div class="section-title">IP Configuration</div>
<div class="toggle-wrap" style="margin-bottom:24px"><label class="toggle" id="dhcp-toggle">
<input type="checkbox" id="net-dhcp" onchange="togDHCP(this.checked)"><span class="toggle-track"></span><span class="toggle-thumb"></span></label>
<span class="toggle-label">Use DHCP</span></div>
<div id="static-fields"><div class="form-grid">
<div class="form-group"><label class="form-label">IP Address</label><input class="form-input" id="net-ip" placeholder="192.168.1.100"></div>
<div class="form-group"><label class="form-label">Subnet Mask</label><input class="form-input" id="net-mask" placeholder="255.255.255.0" value="255.255.255.0"></div>
<div class="form-group"><label class="form-label">Gateway</label><input class="form-input" id="net-gw" placeholder="192.168.1.1"></div>
<div class="form-group"><label class="form-label">DNS Servers</label><input class="form-input" id="net-dns" placeholder="8.8.8.8, 1.1.1.1"><div class="form-hint">Comma-separated</div></div>
</div></div>
<div class="form-actions"><button class="btn btn-primary" onclick="saveNet()">Save &amp; Apply</button>
<button class="btn btn-secondary" onclick="fetchNet()">Reset</button></div></div>`;
  fetchNet();
}

async function fetchNet(){
  try{
    const d=await api('/network'),n=d.network||d,g=$('#net-current');
    if(g)g.innerHTML=[['IP',n.ip],['Gateway',n.gateway],['DNS',(n.dns||[]).join(', ')],['Mode',n.dhcp?'DHCP':'Static']]
      .map(([l,v])=>`<div class="net-item"><span class="net-label">${l}</span><span class="net-value">${v||'--'}</span></div>`).join('');
    const dhcp=n.dhcp!==false;$('#net-dhcp').checked=dhcp;togDHCP(dhcp);
    if(!dhcp){$('#net-ip').value=n.ip||'';$('#net-mask').value=n.mask||n.subnet||'255.255.255.0';$('#net-gw').value=n.gateway||'';$('#net-dns').value=(n.dns||[]).join(', ')}
  }catch{}
}

window.togDHCP=function(on){const f=$('#static-fields');if(f){f.style.opacity=on?'0.3':'1';f.style.pointerEvents=on?'none':'auto'}};
window.saveNet=async function(){
  const dhcp=$('#net-dhcp').checked,body={dhcp};
  if(!dhcp){body.ip=$('#net-ip').value;body.mask=$('#net-mask').value;body.gateway=$('#net-gw').value;body.dns=$('#net-dns').value.split(',').map(s=>s.trim()).filter(Boolean)}
  try{await api('/network',{method:'PUT',body});toast('Network settings saved','success');setTimeout(fetchNet,2000)}
  catch(e){toast(`Error: ${e.message}`,'error')}
};

// ============ FILE MANAGER ============
let _fileCurPath='',_fileRoots=[],_fileSelection=new Set();

function fileIcon(name,isDir){
  if(isDir)return '<span class="fi fi-dir">&#128193;</span>';
  const ext=(name.split('.').pop()||'').toLowerCase();
  if(['mp4','mkv','avi','mov','wmv','flv','webm'].includes(ext))return '<span class="fi fi-video">&#127916;</span>';
  if(['mp3','flac','wav','aac','ogg','wma','m4a'].includes(ext))return '<span class="fi fi-audio">&#127925;</span>';
  if(['jpg','jpeg','png','gif','bmp','svg','webp','ico','tiff'].includes(ext))return '<span class="fi fi-image">&#128444;</span>';
  if(['zip','tar','gz','bz2','xz','rar','7z','tgz'].includes(ext))return '<span class="fi fi-archive">&#128230;</span>';
  if(['pdf'].includes(ext))return '<span class="fi fi-pdf">&#128196;</span>';
  if(['txt','md','log','csv','json','xml','yml','yaml','conf','cfg','ini'].includes(ext))return '<span class="fi fi-text">&#128196;</span>';
  return '<span class="fi fi-file">&#128196;</span>';
}

function fileBreadcrumb(path){
  if(!path)return '';
  // Find which root this path belongs to
  const root=_fileRoots.find(r=>path===r.path||path.startsWith(r.path+'/'));
  if(!root)return esc(path);
  const rel=path===root.path?'':path.slice(root.path.length+1);
  let html=`<span class="fb-crumb fb-root" onclick="fileBrowse('${esc(root.path)}')">${esc(root.name)}</span>`;
  if(rel){
    const parts=rel.split('/');
    let built=root.path;
    for(const p of parts){
      built+='/'+p;
      const bp=built;
      html+=`<span class="fb-sep">/</span><span class="fb-crumb" onclick="fileBrowse('${esc(bp)}')">${esc(p)}</span>`;
    }
  }
  return html;
}

function pgFiles(){
  _fileSelection.clear();
  $('#main-content').innerHTML=`<div class="page-header"><h1 class="page-title">File Manager</h1>
<div class="page-actions">
<button class="btn btn-sm btn-secondary" onclick="fileNewFolder()">${ic.plus} New Folder</button>
<label class="btn btn-sm btn-primary" style="cursor:pointer">${ic.ul} Upload<input type="file" multiple style="display:none" onchange="fileUploadInput(this)"></label>
</div></div>
<div class="fb-toolbar">
<div class="fb-roots" id="fb-roots"><div class="skeleton skeleton-text"></div></div>
<div class="fb-breadcrumb" id="fb-breadcrumb"></div>
</div>
<div class="fb-drop-zone" id="fb-drop-zone">
<div class="fb-drop-overlay hidden" id="fb-drop-overlay"><div class="fb-drop-label">${ic.ul} Drop files here to upload</div></div>
<div class="fb-upload-progress hidden" id="fb-upload-progress"><div class="fb-progress-bar" id="fb-progress-bar"></div><span class="fb-progress-text" id="fb-progress-text"></span></div>
<table class="fb-table" id="fb-table"><thead><tr>
<th class="fb-th-check"><input type="checkbox" id="fb-select-all" onchange="fileSelectAll(this.checked)"></th>
<th class="fb-th-name" onclick="fileSortBy('name')">Name</th>
<th class="fb-th-size" onclick="fileSortBy('size')">Size</th>
<th class="fb-th-date" onclick="fileSortBy('modified')">Modified</th>
<th class="fb-th-actions">Actions</th>
</tr></thead><tbody id="fb-body"><tr><td colspan="5">${skel(3)}</td></tr></tbody></table>
</div>`;
  fileLoadRoots();
  fileInitDragDrop();
}

async function fileLoadRoots(){
  try{
    const d=await api('/files/roots');
    _fileRoots=d.roots||[];
    const el=$('#fb-roots');
    if(!_fileRoots.length){
      el.innerHTML='<div class="fb-no-roots">No mounted shares or volumes found. <a href="#" onclick="event.preventDefault();navigate(\'mounts\')">Add a mount</a> first.</div>';
      $('#fb-body').innerHTML='<tr><td colspan="5" class="fb-empty">No locations available to browse</td></tr>';
      return;
    }
    el.innerHTML=_fileRoots.map(r=>`<button class="btn btn-sm fb-root-btn" onclick="fileBrowse('${esc(r.path)}')" title="${esc(r.path)}">
<span class="fb-root-type fb-type-${esc(r.type)}">${r.type==='local'?'LOCAL':r.type==='volumes'?'VOL':r.type==='nfs'?'NFS':'SMB'}</span>
${esc(r.name)}</button>`).join('');
    // Auto-browse first root
    fileBrowse(_fileRoots[0].path);
  }catch(e){toast('Failed to load file roots: '+e.message,'error')}
}

async function fileBrowse(path,sortBy){
  _fileCurPath=path;
  _fileSelection.clear();
  const chk=$('#fb-select-all');if(chk)chk.checked=false;
  $('#fb-breadcrumb').innerHTML=fileBreadcrumb(path);
  $('#fb-body').innerHTML=`<tr><td colspan="5">${skel(3)}</td></tr>`;
  // Highlight active root button
  $$('.fb-root-btn').forEach(btn=>{
    const btnPath=btn.getAttribute('title');
    btn.classList.toggle('fb-root-active',path===btnPath||path.startsWith(btnPath+'/'));
  });
  try{
    const q=new URLSearchParams({path});
    if(sortBy)q.set('sort',sortBy);
    const d=await api('/files/list?'+q);
    const files=d.files||[];
    if(!files.length){
      $('#fb-body').innerHTML='<tr><td colspan="5" class="fb-empty">Empty directory</td></tr>';
      return;
    }
    $('#fb-body').innerHTML=files.map(f=>{
      const fp=_fileCurPath+'/'+f.name;
      const mod=f.modified?new Date(f.modified).toLocaleString():'-';
      const size=f.isDir?'-':fB(f.size);
      return`<tr class="fb-row" data-path="${esc(fp)}" data-name="${esc(f.name)}" data-isdir="${f.isDir}">
<td class="fb-td-check"><input type="checkbox" onchange="fileToggleSelect('${esc(fp)}',this.checked)"></td>
<td class="fb-td-name" onclick="${f.isDir?`fileBrowse('${esc(fp)}')`:`filePreviewOrDownload('${esc(fp)}','${esc(f.name)}')`}">
${fileIcon(f.name,f.isDir)}<span class="fb-name">${esc(f.name)}</span></td>
<td class="fb-td-size">${size}</td>
<td class="fb-td-date">${mod}</td>
<td class="fb-td-actions">
<button class="btn-icon btn-sm" onclick="fileCopyPrompt('${esc(fp)}','${esc(f.name)}',${f.isDir})" title="Copy to..."><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 01-2-2V4a2 2 0 012-2h9a2 2 0 012 2v1"/></svg></button>
${f.isDir?'':`<button class="btn-icon btn-sm" onclick="fileDownload('${esc(fp)}')" title="Download">${ic.dl}</button>`}
<button class="btn-icon btn-sm" onclick="fileRenamePrompt('${esc(fp)}','${esc(f.name)}')" title="Rename"><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M11 4H4a2 2 0 00-2 2v14a2 2 0 002 2h14a2 2 0 002-2v-7"/><path d="M18.5 2.5a2.121 2.121 0 013 3L12 15l-4 1 1-4 9.5-9.5z"/></svg></button>
<button class="btn-icon btn-sm" onclick="fileDeletePrompt('${esc(fp)}','${esc(f.name)}',${f.isDir})" title="Delete"><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 01-2 2H7a2 2 0 01-2-2V6m3 0V4a2 2 0 012-2h4a2 2 0 012 2v2"/></svg></button>
</td></tr>`}).join('');
  }catch(e){
    $('#fb-body').innerHTML=`<tr><td colspan="5" class="fb-empty fb-error">Error: ${esc(e.message)}</td></tr>`;
  }
}

window.fileBrowse=fileBrowse;

function fileSortBy(field){fileBrowse(_fileCurPath,field)}
window.fileSortBy=fileSortBy;

function fileToggleSelect(path,checked){
  if(checked)_fileSelection.add(path);else _fileSelection.delete(path);
}
window.fileToggleSelect=fileToggleSelect;

function fileSelectAll(checked){
  _fileSelection.clear();
  $$('#fb-body input[type="checkbox"]').forEach(cb=>{
    cb.checked=checked;
    const row=cb.closest('tr');
    if(row&&checked)_fileSelection.add(row.dataset.path);
  });
}
window.fileSelectAll=fileSelectAll;

function fileDownload(path){
  const a=document.createElement('a');
  a.href=`${API}/files/download?path=${encodeURIComponent(path)}`;
  a.download='';a.click();
}
window.fileDownload=fileDownload;

function filePreviewOrDownload(path,name){fileDownload(path)}
window.filePreviewOrDownload=filePreviewOrDownload;

async function fileNewFolder(){
  if(!_fileCurPath){toast('Navigate to a directory first','error');return}
  const name=prompt('New folder name:');
  if(!name||!name.trim())return;
  try{
    await api('/files/mkdir',{method:'POST',body:{path:_fileCurPath+'/'+name.trim()}});
    toast('Folder created','success');
    fileBrowse(_fileCurPath);
  }catch(e){toast('Failed to create folder: '+e.message,'error')}
}
window.fileNewFolder=fileNewFolder;

async function fileDeletePrompt(path,name,isDir){
  const what=isDir?'directory "'+name+'" and all its contents':'file "'+name+'"';
  if(!confirm('Delete '+what+'? This cannot be undone.'))return;
  try{
    await api('/files/delete',{method:'POST',body:{path}});
    toast('Deleted '+name,'success');
    fileBrowse(_fileCurPath);
  }catch(e){toast('Delete failed: '+e.message,'error')}
}
window.fileDeletePrompt=fileDeletePrompt;

async function fileRenamePrompt(path,oldName){
  const newName=prompt('Rename to:',oldName);
  if(!newName||newName===oldName||!newName.trim())return;
  try{
    await api('/files/rename',{method:'POST',body:{path,newName:newName.trim()}});
    toast('Renamed to '+newName.trim(),'success');
    fileBrowse(_fileCurPath);
  }catch(e){toast('Rename failed: '+e.message,'error')}
}
window.fileRenamePrompt=fileRenamePrompt;

async function fileCopyPrompt(path,name,isDir){
  if(!_fileRoots||!_fileRoots.length){toast('No destinations available','error');return}
  // Build destination picker: show all roots except the one this file is already in
  const srcRoot=_fileRoots.find(r=>path===r.path||path.startsWith(r.path+'/'));
  const dests=_fileRoots.filter(r=>r!==srcRoot);
  if(!dests.length){toast('No other locations to copy to. Add another mount or use Local Storage.','error');return}
  let destPath;
  if(dests.length===1){
    destPath=dests[0].path;
    if(!confirm(`Copy "${name}" to ${dests[0].name}?`))return;
  }else{
    const choices=dests.map((r,i)=>`${i+1}. ${r.name}`).join('\n');
    const pick=prompt(`Copy "${name}" to:\n${choices}\n\nEnter number:`);
    if(!pick)return;
    const idx=parseInt(pick,10)-1;
    if(isNaN(idx)||idx<0||idx>=dests.length){toast('Invalid selection','error');return}
    destPath=dests[idx].path;
  }
  const dest=destPath+'/'+name;
  try{
    toast(`Copying ${name}...`,'info');
    await api('/files/copy',{method:'POST',body:{source:path,destination:dest}});
    toast(`Copied ${name}`,'success');
  }catch(e){toast('Copy failed: '+e.message,'error')}
}
window.fileCopyPrompt=fileCopyPrompt;

async function fileUploadFiles(files){
  if(!files||!files.length)return;
  if(!_fileCurPath){toast('Navigate to a directory first','error');return}
  const prog=$('#fb-upload-progress');
  const bar=$('#fb-progress-bar');
  const txt=$('#fb-progress-text');
  prog.classList.remove('hidden');
  txt.textContent=`Uploading ${files.length} file(s)...`;
  bar.style.width='0%';

  const form=new FormData();
  form.append('path',_fileCurPath);
  for(const f of files)form.append('file',f);

  try{
    const xhr=new XMLHttpRequest();
    xhr.upload.addEventListener('progress',e=>{
      if(e.lengthComputable){
        const pct=Math.round(e.loaded/e.total*100);
        bar.style.width=pct+'%';
        txt.textContent=`Uploading... ${pct}% (${fB(e.loaded)} / ${fB(e.total)})`;
      }
    });
    await new Promise((resolve,reject)=>{
      xhr.onload=()=>{
        if(xhr.status>=200&&xhr.status<300){
          const d=JSON.parse(xhr.responseText);
          toast(`Uploaded ${d.uploaded} file(s)`,'success');
          resolve();
        }else{
          try{const d=JSON.parse(xhr.responseText);reject(new Error(d.error||'Upload failed'))}
          catch{reject(new Error('Upload failed: HTTP '+xhr.status))}
        }
      };
      xhr.onerror=()=>reject(new Error('Upload failed: network error'));
      xhr.open('POST',`${API}/files/upload`);
      xhr.send(form);
    });
    fileBrowse(_fileCurPath);
  }catch(e){toast('Upload failed: '+e.message,'error')}
  finally{setTimeout(()=>prog.classList.add('hidden'),1500)}
}

function fileUploadInput(input){
  if(input.files.length)fileUploadFiles(input.files);
  input.value='';
}
window.fileUploadInput=fileUploadInput;

function fileInitDragDrop(){
  const zone=$('#fb-drop-zone');
  const overlay=$('#fb-drop-overlay');
  if(!zone||!overlay)return;
  let dragCount=0;
  zone.addEventListener('dragenter',e=>{e.preventDefault();dragCount++;overlay.classList.remove('hidden')});
  zone.addEventListener('dragleave',e=>{e.preventDefault();dragCount--;if(dragCount<=0){dragCount=0;overlay.classList.add('hidden')}});
  zone.addEventListener('dragover',e=>{e.preventDefault()});
  zone.addEventListener('drop',e=>{
    e.preventDefault();dragCount=0;overlay.classList.add('hidden');
    if(e.dataTransfer.files.length)fileUploadFiles(e.dataTransfer.files);
  });
}

// ============ SETTINGS ============
function pgSettings(){
  $('#main-content').innerHTML=`<div class="page-header"><h1 class="page-title">System Settings</h1></div>
<div class="settings-sections stagger">
<div class="settings-section card animate-up"><div class="section-title">System Information</div>
<dl class="info-grid" id="sys-info"><dt>Version</dt><dd>--</dd><dt>Uptime</dt><dd>--</dd><dt>Kernel</dt><dd>--</dd><dt>Architecture</dt><dd>--</dd><dt>Hostname</dt><dd>--</dd></dl></div>
<div class="settings-section card animate-up"><div class="section-title">Change Password</div>
<div class="form-grid" style="max-width:480px">
<div class="form-group"><label class="form-label">Current Password</label><input class="form-input" id="pw-cur" type="password"></div>
<div class="form-group"><label class="form-label">New Password</label><input class="form-input" id="pw-new" type="password"></div>
<div class="form-group"><label class="form-label">Confirm New Password</label><input class="form-input" id="pw-cfm" type="password"></div>
</div><div class="form-actions"><button class="btn btn-primary" onclick="chgPw()">Update Password</button></div></div>
<div class="settings-section card animate-up"><div class="section-title">SSH Access</div>
<p style="font-size:.85rem;color:var(--tx-d);margin-bottom:16px">Enable or disable remote SSH access. SSH is disabled by default for security. This setting persists across reboots.</p>
<div class="toggle-wrap"><label class="toggle" id="ssh-toggle">
<input type="checkbox" id="ssh-enabled" onchange="toggleSSH(this.checked)"><span class="toggle-track"></span><span class="toggle-thumb"></span></label>
<span class="toggle-label" id="ssh-label">SSH Disabled</span>
<span id="ssh-status" style="margin-left:12px;font-size:.82rem;font-family:var(--mono)"></span></div></div>
<div class="settings-section card animate-up" id="settings-disk-section"><div class="section-title">Disk Management</div>
<div id="settings-disk-status">Loading disk status...</div>
<div class="form-actions"><button class="btn btn-primary" onclick="setupWizard.show()">Set Up Disks</button>
<button class="btn btn-secondary" onclick="settingsRescanDisks()">Rescan Disks</button></div></div>
<div class="settings-section card animate-up"><div class="section-title">Backup &amp; Restore</div>
<p style="font-size:.85rem;color:var(--tx-d);margin-bottom:16px">Download or restore a config backup (compose files, mount configs, network settings). Docker images not included.</p>
<div class="form-actions" style="gap:12px;flex-wrap:wrap">
<button class="btn btn-secondary" onclick="exportCfg()">${ic.dl} Download Backup</button>
<button class="btn btn-secondary" onclick="importCfg()">${ic.ul} Restore Backup</button>
</div>
<input type="file" id="backup-file-input" accept=".tar.gz,.tgz" style="display:none">
<div id="restore-status" style="margin-top:12px"></div></div>
<div class="settings-section card animate-up"><div class="card-header"><div class="section-title" style="border:none;margin:0;padding:0">System Logs</div>
<button class="btn btn-sm btn-secondary" onclick="fetchLogs()">Refresh</button></div>
<div class="log-viewer" id="log-viewer">Loading logs...</div></div>
<div class="settings-section card animate-up"><div class="section-title">Danger Zone</div>
<div class="danger-zone-items">
<div class="danger-item" style="display:flex;align-items:center;justify-content:space-between;padding:12px 0;border-bottom:1px solid var(--bd)">
<div><div style="font-weight:600;font-size:.9rem;margin-bottom:2px">Re-run Setup Wizard</div>
<div style="font-size:.82rem;color:var(--tx-d)">Open the first-run setup wizard to reconfigure disks, network, and timezone.</div></div>
<button class="btn btn-secondary" onclick="rerunSetupWizard()">Re-run Wizard</button></div>
<div class="danger-item" style="display:flex;align-items:center;justify-content:space-between;padding:12px 0;border-bottom:1px solid var(--bd)">
<div><div style="font-weight:600;font-size:.9rem;margin-bottom:2px">Wipe Config Disk</div>
<div style="font-size:.82rem;color:var(--tx-d)">Erase all configuration data (compose files, credentials, network settings). The disk will be reformatted and current RAM config synced back.</div></div>
<button class="btn btn-danger" onclick="wipeDisk('config')" id="btn-wipe-config">Wipe Config</button></div>
<div class="danger-item" style="display:flex;align-items:center;justify-content:space-between;padding:12px 0;border-bottom:1px solid var(--bd)">
<div><div style="font-weight:600;font-size:.9rem;margin-bottom:2px">Wipe Cache Disk</div>
<div style="font-size:.82rem;color:var(--tx-d)">Erase all Docker images, volumes, and container data. Docker will be stopped and restarted.</div></div>
<button class="btn btn-danger" onclick="wipeDisk('cache')" id="btn-wipe-cache">Wipe Cache</button></div>
<div class="danger-item" style="display:flex;align-items:center;justify-content:space-between;padding:12px 0">
<div><div style="font-weight:600;font-size:.9rem;margin-bottom:2px">Reboot Server</div>
<div style="font-size:.82rem;color:var(--tx-d)">Reboot the server. All running containers will be stopped and restarted on boot.</div></div>
<button class="btn btn-danger" onclick="rebootServer()">Reboot Server</button></div>
</div></div></div>`;
  fetchSysInfo();fetchLogs();fetchSettingsDiskStatus();fetchSSHStatus();
}

async function fetchSysInfo(){
  try{const d=await api('/system/info'),i=d.info||d,g=$('#sys-info');if(!g)return;
  g.innerHTML=`<dt>Version</dt><dd>${esc(i.version||'--')}</dd><dt>Uptime</dt><dd>${i.uptime?fU(i.uptime):'--'}</dd>
<dt>Kernel</dt><dd>${esc(i.kernel||'--')}</dd><dt>Architecture</dt><dd>${esc(i.arch||'--')}</dd><dt>Hostname</dt><dd>${esc(i.hostname||'--')}</dd>`}catch{}
}

async function fetchLogs(){
  try{const d=await api('/system/logs');const v=$('#log-viewer');if(v)v.textContent=d.logs||d||'No logs'}
  catch(e){const v=$('#log-viewer');if(v)v.textContent=`Error: ${e.message}`}
}

async function fetchSettingsDiskStatus(){
  try{
    const s=await api('/setup/status');
    _diskStatus=s;
    const el=$('#settings-disk-status');if(!el)return;
    const cfg=s.configDisk||{};
    const cache=s.cacheDisk||{};
    let html=`<dl class="info-grid" style="margin-bottom:16px">
<dt>Config Disk</dt><dd>${cfg.persistent?`Persistent (${esc(cfg.type)} on ${esc(cfg.device)})`:'Temporary (tmpfs)'}</dd>
<dt>Cache Disk</dt><dd>${cache.persistent?`Persistent (${esc(cache.type)} on ${esc(cache.device)})`:'Temporary (tmpfs)'}</dd>
<dt>Available Disks</dt><dd>${(s.availableDisks||[]).length} detected</dd>
</dl>`;
    // Check for expandable disks
    try{
      const r=await api('/system/disks/expandable');
      const disks=r.disks||[];
      if(disks.length>0){
        html+=`<div style="margin-bottom:16px">`;
        disks.forEach(d=>{
          const role=d.role==='single'?'Disk':d.role==='config'?'Config Disk':'Cache Disk';
          html+=`<div style="display:flex;align-items:center;gap:12px;padding:10px 12px;background:var(--ac-glow);border:1px solid var(--ac-dim);border-radius:var(--r-md);margin-bottom:8px;flex-wrap:wrap">
<span style="font-size:1.1rem">&#9889;</span>
<span style="flex:1;font-size:.9rem">${esc(role)} (${esc(d.device)}): <strong>${fB(d.growBytes)}</strong> available to expand</span>
<button class="btn btn-sm btn-primary" onclick="expandDisk('${esc(d.device)}')">Expand</button>
</div>`;
        });
        html+=`</div>`;
      }
    }catch{}
    el.innerHTML=html;
  }catch(e){
    const el=$('#settings-disk-status');if(el)el.textContent='Could not load disk status';
  }
}

window.settingsRescanDisks=async function(){
  try{
    await api('/setup/rescan',{method:'POST'});
    toast('Disk rescan complete','success');
    fetchSettingsDiskStatus();
  }catch(e){toast(`Rescan failed: ${e.message}`,'error')}
};

async function fetchSSHStatus(){
  try{
    const d=await api('/system/ssh');
    const cb=$('#ssh-enabled'),lbl=$('#ssh-label'),st=$('#ssh-status');
    if(cb)cb.checked=!!d.enabled;
    if(lbl)lbl.textContent=d.enabled?'SSH Enabled':'SSH Disabled';
    if(st){
      if(d.running)st.innerHTML='<span style="color:var(--grn)">&#x25CF; Running</span>';
      else if(d.enabled)st.innerHTML='<span style="color:var(--yel)">&#x25CF; Stopped</span>';
      else st.innerHTML='<span style="color:var(--tx-m)">&#x25CF; Stopped</span>';
    }
  }catch{}
}

window.toggleSSH=async function(enabled){
  const cb=$('#ssh-enabled'),lbl=$('#ssh-label'),st=$('#ssh-status');
  if(lbl)lbl.textContent=enabled?'Enabling...':'Disabling...';
  if(st)st.innerHTML='<span class="spinner"></span>';
  try{
    const d=await api('/system/ssh',{method:'PUT',body:{enabled}});
    toast(enabled?'SSH enabled':'SSH disabled','success');
    if(cb)cb.checked=!!d.enabled;
    if(lbl)lbl.textContent=d.enabled?'SSH Enabled':'SSH Disabled';
    if(st){
      if(d.running)st.innerHTML='<span style="color:var(--grn)">&#x25CF; Running</span>';
      else st.innerHTML='<span style="color:var(--tx-m)">&#x25CF; Stopped</span>';
    }
  }catch(e){
    toast(`SSH toggle failed: ${e.message}`,'error');
    if(cb)cb.checked=!enabled;
    if(lbl)lbl.textContent=enabled?'SSH Disabled':'SSH Enabled';
    if(st)st.textContent='';
    fetchSSHStatus();
  }
};

window.wipeDisk=async function(diskType){
  const label=diskType==='config'?'CONFIG':'CACHE';
  const warnings=diskType==='config'
    ?'This will ERASE all configuration data on the config disk (compose files, credentials, network settings). Current RAM config will be synced back to the fresh disk.'
    :'This will ERASE all Docker images, volumes, and container data on the cache disk. Docker will be stopped during the wipe.';
  if(!confirm(`Wipe ${label} disk?\n\n${warnings}\n\nThis action cannot be undone.`))return;
  if(!confirm(`Are you absolutely sure? Type confirms wiping the ${label} disk.`))return;
  const btn=$(`#btn-wipe-${diskType}`);if(btn){btn.disabled=true;btn.textContent='Wiping...'}
  try{
    await api('/setup/wipe',{method:'POST',body:{diskType,confirmWipe:true}});
    toast(`${label} disk wiped successfully. Reboot recommended.`,'success');
    fetchSettingsDiskStatus();
  }catch(e){toast(`Wipe failed: ${e.message}`,'error')}
  finally{if(btn){btn.disabled=false;btn.textContent=`Wipe ${diskType==='config'?'Config':'Cache'}`}}
};

window.rerunSetupWizard=function(){
  if(typeof setupWizard!=='undefined'){
    // Clear the dismiss cookie so wizard shows fully
    document.cookie='wraith-wizard-dismissed=;expires=Thu, 01 Jan 1970 00:00:00 GMT;path=/';
    // Reset wizard state so it fetches fresh status
    setupWizard.show();
  }else{
    toast('Setup wizard is not available','error');
  }
};

window.rebootServer=async function(){
  if(!confirm('Reboot the server? All running containers will be stopped.'))return;
  const btn=document.querySelector('[onclick="rebootServer()"]');
  if(btn){btn.disabled=true;btn.textContent='Saving config...';}
  try{
    const res=await api('/system/reboot',{method:'POST'});
    const saved=res&&res.configSaved;
    toast(saved?'Config saved. Rebooting server...':'Rebooting server...','success');
    setTimeout(()=>{document.body.innerHTML='<div style="display:flex;align-items:center;justify-content:center;height:100vh;font-family:sans-serif;color:#fff;background:#0a0a0a"><div style="text-align:center"><h2>Server is rebooting...</h2><p>'+(saved?'Configuration was saved to disk. ':'')+'This page will refresh automatically.</p></div></div>';const check=setInterval(async()=>{try{await fetch('/api/system/info');clearInterval(check);location.reload()}catch{}},3000)},1000);
  }catch(e){
    if(btn){btn.disabled=false;btn.textContent='Reboot Server';}
    toast(`Reboot failed: ${e.message}`,'error');
  }
};

window.chgPw=async function(){
  const c=$('#pw-cur').value,n=$('#pw-new').value,cf=$('#pw-cfm').value;
  if(!c||!n){toast('Fill in all fields','error');return}
  if(n!==cf){toast('Passwords do not match','error');return}
  if(n.length<6){toast('Min 6 characters','error');return}
  try{await api('/auth/password',{method:'PUT',body:{current:c,password:n}});toast('Password updated','success');$('#pw-cur').value=$('#pw-new').value=$('#pw-cfm').value=''}
  catch(e){toast(`Error: ${e.message}`,'error')}
};

window.exportCfg=async function(){
  try{const r=await fetch(`${API}/system/backup`);if(r.status===401){location.href='/login.html';return}
  if(!r.ok)throw new Error(`HTTP ${r.status}`);const b=await r.blob(),u=URL.createObjectURL(b),a=document.createElement('a');
  a.href=u;a.download=`wraithos-config-${new Date().toISOString().split('T')[0]}.tar.gz`;a.click();URL.revokeObjectURL(u);toast('Backup downloaded','success')}
  catch(e){toast(`Download failed: ${e.message}`,'error')}
};

window.importCfg=function(){
  const input=$('#backup-file-input');
  if(!input)return;
  input.value='';
  input.onchange=async function(){
    const file=input.files[0];
    if(!file)return;
    if(!file.name.endsWith('.tar.gz')&&!file.name.endsWith('.tgz')){
      toast('Please select a .tar.gz backup file','error');return;
    }
    if(!confirm(`Restore backup from "${file.name}"?\n\nThis will overwrite current configuration files (compose files, mount configs, network settings). A reboot is recommended after restore.`))return;
    const status=$('#restore-status');
    if(status)status.innerHTML='<p style="font-size:.85rem;color:var(--tx-d)">Restoring backup...</p>';
    try{
      const fd=new FormData();
      fd.append('backup',file);
      const r=await fetch(`${API}/system/restore`,{method:'POST',body:fd});
      if(r.status===401){location.href='/login.html';return}
      const d=await r.json();
      if(!r.ok)throw new Error(d.error||`HTTP ${r.status}`);
      toast(`Backup restored (${d.restored} files). Reboot recommended.`,'success');
      if(status)status.innerHTML=`<p style="font-size:.85rem;color:var(--green)">Restored ${d.restored} files from backup. Reboot recommended to apply all changes.</p>`;
    }catch(e){
      toast(`Restore failed: ${e.message}`,'error');
      if(status)status.innerHTML=`<p style="font-size:.85rem;color:var(--red)">Restore failed: ${esc(e.message)}</p>`;
    }
  };
  input.click();
};

// === EXPOSE SHARED UTILS FOR EXTERNAL SCRIPTS (setup-wizard.js, etc.) ===
window._w={api,toast,$,$$,esc,fB,fU};

// === INIT ===
// Load dependencies then navigate to dashboard
let _scriptsLoaded=0;
const _scriptsNeeded=3;
function _onScriptReady(){_scriptsLoaded++;if(_scriptsLoaded>=_scriptsNeeded){navigate('dashboard');setTimeout(()=>{if(typeof setupWizard!=='undefined')setupWizard.checkAndShow()},500)}}

const ts=document.createElement('script');ts.src='/js/terminal.js';
ts.onload=_onScriptReady;ts.onerror=_onScriptReady;
document.head.appendChild(ts);

const sw=document.createElement('script');sw.src='/js/setup-wizard.js';
sw.onload=_onScriptReady;sw.onerror=_onScriptReady;
document.head.appendChild(sw);

const yj=document.createElement('script');yj.src='/js/vendor/js-yaml.min.js';
yj.onload=_onScriptReady;yj.onerror=_onScriptReady;
document.head.appendChild(yj);
})();
