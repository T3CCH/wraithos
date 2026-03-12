(()=>{
'use strict';
const $=(s,p)=>(p||document).querySelector(s);
const $$=(s,p)=>[...(p||document).querySelectorAll(s)];
const API='/api';
let _poll=null,_term=null;

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

const pages=['dashboard','compose','mounts','network','settings'];
let cur='dashboard';

function stopPoll(){if(_poll){clearInterval(_poll);_poll=null}}
function startPoll(fn,ms=5000){stopPoll();fn();_poll=setInterval(fn,ms)}

window.navigate=function(p){
  if(!pages.includes(p))return;cur=p;
  $$('.nav-item').forEach(el=>el.classList.toggle('active',el.dataset.page===p));
  $('#sidebar').classList.remove('open');stopPoll();
  if(p!=='compose'&&_term){_term.destroy();_term=null}
  const m=$('#main-content');m.style.opacity='0';
  setTimeout(()=>{({dashboard:pgDash,compose:pgCompose,mounts:pgMounts,network:pgNetwork,settings:pgSettings}[p]||pgDash)();m.style.opacity='1'},80);
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
  box:'<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="2" y="6" width="20" height="12" rx="2"/><path d="M6 6V4a2 2 0 012-2h8a2 2 0 012 2v2"/></svg>',
};

// ============ DASHBOARD ============
function pgDash(){
  $('#main-content').innerHTML=`
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
}

async function fetchDash(){
  try{
    const d=await api('/system/status');
    updStats(d.system||{});updNet(d.network||{});updCont(d.containers||[]);
    if(d.system&&d.system.uptime)$('#topbar-uptime').textContent=fU(d.system.uptime);
  }catch{}
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
  try{await api(`/compose/${a}`,{method:'POST'});toast(`Stack ${a} initiated`,'success');setTimeout(fetchDash,1500)}
  catch(e){toast(`Failed: ${e.message}`,'error')}
  if(b)b.disabled=false;
};

window.contAct=async function(name,a){
  if(a==='logs'){navigate('compose');setTimeout(async()=>{if(_term){_term.clear();try{const d=await api(`/containers/${encodeURIComponent(name)}/logs`);_term.write(d.logs||d||'No logs')}catch(e){_term.writeLine(`Error: ${e.message}`,'err')}}},200);return}
  try{await api(`/containers/${encodeURIComponent(name)}/${a}`,{method:'POST'});toast(`${name}: ${a} OK`,'success');setTimeout(fetchDash,1500)}
  catch(e){toast(`${name}: ${e.message}`,'error')}
};

// ============ COMPOSE EDITOR ============
function pgCompose(){
  $('#main-content').innerHTML=`<div class="editor-layout">
<div class="editor-toolbar"><div class="editor-toolbar-group"><h1 class="page-title" style="font-size:1.1rem">Compose Editor</h1></div>
<div class="editor-toolbar-group">
<button class="btn btn-sm btn-secondary" onclick="compAct('validate')" id="btn-validate">${ic.check} Validate</button>
<button class="btn btn-sm btn-secondary" onclick="compAct('save')" id="btn-save">${ic.save} Save</button>
<button class="btn btn-sm btn-primary" onclick="compAct('deploy')" id="btn-deploy">${ic.send} Deploy</button>
</div></div>
<div class="editor-container"><textarea id="compose-editor" class="compose-textarea" spellcheck="false" placeholder="Loading docker-compose.yml..."></textarea></div>
<div id="terminal-container"></div></div>`;
  _term=new WraithTerminal($('#terminal-container'));
  loadCompose();
  const ta=$('#compose-editor');
  ta.addEventListener('keydown',e=>{
    if(e.key==='Tab'){e.preventDefault();const s=ta.selectionStart,en=ta.selectionEnd;ta.value=ta.value.substring(0,s)+'  '+ta.value.substring(en);ta.selectionStart=ta.selectionEnd=s+2}
    if((e.ctrlKey||e.metaKey)&&e.key==='s'){e.preventDefault();compAct('save')}
  });
}

async function loadCompose(){
  try{const d=await api('/compose/file');const e=$('#compose-editor');if(e)e.value=d.content||d||''}
  catch(x){const e=$('#compose-editor');if(e)e.placeholder='No compose file found. Paste your docker-compose.yml here.'}
}

window.compAct=async function(a){
  const btn=$(`#btn-${a}`),ed=$('#compose-editor');if(!ed)return;if(btn)btn.disabled=true;
  try{
    if(a==='save'){await api('/compose/file',{method:'PUT',body:{content:ed.value}});toast('Saved','success')}
    else if(a==='validate'){const r=await api('/compose/validate',{method:'POST',body:{content:ed.value}});
      if(r.valid){toast('Valid YAML','success');if(_term)_term.writeLine('Validation passed.','ok')}
      else{toast('Validation errors','error');if(_term)_term.writeLine(r.error||'Invalid YAML','err')}}
    else if(a==='deploy'){
      await api('/compose/file',{method:'PUT',body:{content:ed.value}});
      if(_term){_term.clear();_term.writeLine('Deploying stack...','info')}
      const ws=`${location.protocol==='https:'?'wss:':'ws:'}//${location.host}/api/compose/deploy/ws`;
      try{_term.connectWS(ws)}catch{const r=await api('/compose/deploy',{method:'POST'});if(_term)_term.write(r.output||r||'Done')}
      toast('Deploy started','info');
    }
  }catch(e){toast(`Error: ${e.message}`,'error');if(_term)_term.writeLine(`Error: ${e.message}`,'err')}
  if(btn)btn.disabled=false;
};

// ============ SAMBA MOUNTS ============
function pgMounts(){
  $('#main-content').innerHTML=`<div class="page-header"><h1 class="page-title">Samba Mounts</h1>
<div class="page-actions"><button class="btn btn-sm btn-primary" onclick="showMntForm()">${ic.plus} Add Mount</button></div></div>
<div id="mnt-form" class="hidden" style="margin-bottom:24px"><div class="form-card">
<h3 style="font-size:.95rem;font-weight:600;margin-bottom:16px">New SMB/CIFS Mount</h3>
<div class="form-grid">
<div class="form-group"><label class="form-label">Server</label><input class="form-input" id="mnt-server" placeholder="192.168.1.100"></div>
<div class="form-group"><label class="form-label">Share</label><input class="form-input" id="mnt-share" placeholder="shared_folder"></div>
<div class="form-group"><label class="form-label">Mount Point</label><input class="form-input" id="mnt-mp" placeholder="/mnt/smb/myshare"></div>
<div class="form-group"><label class="form-label">Username</label><input class="form-input" id="mnt-user" placeholder="optional"></div>
<div class="form-group"><label class="form-label">Password</label><input class="form-input" id="mnt-pass" type="password" placeholder="optional"></div>
<div class="form-group"><label class="form-label">Options</label><input class="form-input" id="mnt-opts" placeholder="ro,vers=3.0"><div class="form-hint">Additional mount options</div></div>
</div><div class="form-actions"><button class="btn btn-primary" onclick="addMnt()">Mount</button>
<button class="btn btn-secondary" onclick="hideMntForm()">Cancel</button></div></div></div>
<div class="mount-list" id="mount-list">${skel(2)}</div>`;
  fetchMnts();
}

async function fetchMnts(){
  try{
    const d=await api('/mounts'),ms=d.mounts||d||[],l=$('#mount-list');if(!l)return;
    if(!ms.length){l.innerHTML='<div class="empty-state"><h3>No mounts configured</h3><p>Add a Samba/CIFS mount to share files with containers.</p></div>';return}
    l.innerHTML=ms.map((m,i)=>`<div class="mount-card" style="animation-delay:${i*60}ms"><div class="mount-info">
<h3><span class="container-status ${m.mounted?'status-running':'status-stopped'}"><span class="dot"></span>${m.mounted?'mounted':'unmounted'}</span> ${esc(m.mountpoint||m.path)}</h3>
<dl class="mount-details"><dt>Source</dt><dd>//${esc(m.server)}/${esc(m.share)}</dd><dt>User</dt><dd>${esc(m.username||'guest')}</dd><dt>Options</dt><dd>${esc(m.options||'defaults')}</dd></dl>
${(m.volumes&&m.volumes.length)?`<div class="mount-volumes"><div class="label">Used by volumes</div>${m.volumes.map(v=>`<span class="volume-tag">${esc(v)}</span>`).join('')}</div>`:''}</div>
<div class="mount-actions">${m.mounted?`<button class="btn btn-sm btn-secondary" data-mnt-action="unmount" data-mnt-id="${esc(m.id||m.mountpoint)}">Unmount</button>`
:`<button class="btn btn-sm btn-primary" data-mnt-action="mount" data-mnt-id="${esc(m.id||m.mountpoint)}">Mount</button>`}
<button class="btn btn-sm btn-danger" data-mnt-action="delete" data-mnt-id="${esc(m.id||m.mountpoint)}">Remove</button></div></div>`).join('');
    // Event delegation for mount action buttons
    l.addEventListener('click',function(e){const btn=e.target.closest('[data-mnt-action]');if(btn)mntAct(btn.dataset.mntId,btn.dataset.mntAction)});
  }catch(e){const l=$('#mount-list');if(l)l.innerHTML=`<div class="empty-state"><h3>Could not load mounts</h3><p>${esc(e.message)}</p></div>`}
}

window.showMntForm=function(){$('#mnt-form').classList.remove('hidden')};
window.hideMntForm=function(){$('#mnt-form').classList.add('hidden')};
window.addMnt=async function(){
  const d={server:$('#mnt-server').value,share:$('#mnt-share').value,mountpoint:$('#mnt-mp').value,
    username:$('#mnt-user').value,password:$('#mnt-pass').value,options:$('#mnt-opts').value};
  if(!d.server||!d.share||!d.mountpoint){toast('Server, share, and mount point required','error');return}
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
<div class="settings-section card animate-up"><div class="section-title">Backup &amp; Export</div>
<p style="font-size:.85rem;color:var(--tx-d);margin-bottom:16px">Download config backup (compose files, mount configs, network settings). Docker images not included.</p>
<button class="btn btn-secondary" onclick="exportCfg()">${ic.dl} Export Config Backup</button></div>
<div class="settings-section card animate-up"><div class="card-header"><div class="section-title" style="border:none;margin:0;padding:0">System Logs</div>
<button class="btn btn-sm btn-secondary" onclick="fetchLogs()">Refresh</button></div>
<div class="log-viewer" id="log-viewer">Loading logs...</div></div></div>`;
  fetchSysInfo();fetchLogs();
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
  a.href=u;a.download=`wraithos-config-${new Date().toISOString().split('T')[0]}.tar.gz`;a.click();URL.revokeObjectURL(u);toast('Exported','success')}
  catch(e){toast(`Export failed: ${e.message}`,'error')}
};

// === INIT ===
const ts=document.createElement('script');ts.src='/js/terminal.js';
ts.onload=()=>navigate('dashboard');ts.onerror=()=>navigate('dashboard');
document.head.appendChild(ts);
})();
