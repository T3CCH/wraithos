(()=>{
'use strict';
const $=(s,p)=>(p||document).querySelector(s);
const $$=(s,p)=>[...(p||document).querySelectorAll(s)];
const API='/api';
let _poll=null,_term=null,_yamlTimer=null,_diskStatus=null,_stackTerm=null,_stackLogWs=null;

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

const pages=['dashboard','stacks','compose','mounts','files','network','docker-networks','settings'];
let cur='dashboard';

function stopPoll(){if(_poll){clearInterval(_poll);_poll=null}}
function startPoll(fn,ms=5000){stopPoll();fn();_poll=setInterval(fn,ms)}

function _closeStackTerm(){if(_stackTerm){_stackTerm.destroy();_stackTerm=null}if(_stackLogWs){_stackLogWs.close();_stackLogWs=null}const p=$('#stack-terminal-panel');if(p)p.remove()}

window.navigate=function(p){
  if(!pages.includes(p))return;cur=p;
  $$('.nav-item').forEach(el=>el.classList.toggle('active',el.dataset.page===p));
  $('#sidebar').classList.remove('open');stopPoll();
  if(p!=='compose'&&_term){_term.destroy();_term=null}
  if(p!=='stacks')_closeStackTerm();
  const m=$('#main-content');m.style.opacity='0';
  setTimeout(()=>{({dashboard:pgDash,stacks:pgStacks,compose:pgCompose,mounts:pgMounts,files:pgFiles,network:pgNetwork,'docker-networks':pgDockerNetworks,settings:pgSettings}[p]||pgDash)();m.style.opacity='1'},80);
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
<div class="card animate-up" style="margin-bottom:24px"><div class="card-header"><div class="card-title">Storage</div></div>
<div id="dash-storage"><div class="skeleton skeleton-text"></div></div></div>
<div class="card animate-up" style="margin-bottom:24px"><div class="card-header"><div class="card-title">Docker Images</div>
<button class="btn btn-sm btn-secondary" onclick="dockerPrune()" id="btn-docker-prune">Clean Up</button></div>
<div id="dash-images"><div class="skeleton skeleton-text"></div></div>
<div class="dim" style="margin-top:8px;font-size:.82rem" id="dash-reclaimable"></div></div>
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
    updStats(d.system||{});updStorage(d.system||{});updNet(d.network||{});updCont(d.containers||[]);
    updImages(d.images||[],d.reclaimable||0);
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
<button class="btn btn-sm btn-secondary" data-cont-action="logs" data-cont-name="${esc(c.name)}" data-cont-stack="${esc(c.stack||'')}" style="margin-left:auto">Logs</button>
</div></div>`).join('');
  // Event delegation for container action buttons
  g.addEventListener('click',function(e){const btn=e.target.closest('[data-cont-action]');if(btn)contAct(btn.dataset.contName,btn.dataset.contAction,btn.dataset.contStack)});
}

function updStorage(s){
  const el=$('#dash-storage');if(!el)return;
  const cu=s.configDiskUsed||0,ct=s.configDiskTotal||1,cpp=Math.round(cu/ct*100),
    du=s.cacheDiskUsed||0,dt=s.cacheDiskTotal||1,dp=Math.round(du/dt*100);
  el.innerHTML=`<div class="dash-storage-item">
<div class="dash-storage-label"><span>Config Disk</span><span class="dim">${fB(cu)} / ${fB(ct)} (${cpp}%)</span></div>
<div class="dash-storage-bar"><div class="dash-storage-fill ${bC(cpp)}" style="width:${cpp}%"></div></div>
</div>
<div class="dash-storage-item">
<div class="dash-storage-label"><span>Cache Disk</span><span class="dim">${fB(du)} / ${fB(dt)} (${dp}%)</span></div>
<div class="dash-storage-bar"><div class="dash-storage-fill ${bC(dp)}" style="width:${dp}%"></div></div>
</div>`;
}

function updImages(imgs,reclaimable){
  const el=$('#dash-images');if(!el)return;
  if(!imgs.length){el.innerHTML='<div class="dim" style="font-size:.85rem">No Docker images found.</div>';$('#dash-reclaimable').textContent='';return}
  el.innerHTML='<div class="dash-images-list">'+imgs.map(img=>{
    const tag=(img.tags&&img.tags[0])||'<none>';
    const used=img.inUse;
    return`<div class="dash-image-row${used?'':' dash-image-unused'}">
<div class="dash-image-tag">${esc(tag)}</div>
<div class="dash-image-meta"><span>${fB(img.size)}</span><span class="dash-image-badge ${used?'badge-used':'badge-unused'}">${used?'in use':'unused'}</span></div>
</div>`}).join('')+'</div>';
  const re=$('#dash-reclaimable');
  if(re)re.textContent=reclaimable>0?`Reclaimable: ${fB(reclaimable)} from unused images`:'All images are in use.';
}

window.dockerPrune=async function(){
  if(!confirm('Remove all unused Docker images, containers, and networks? This cannot be undone.'))return;
  const btn=$('#btn-docker-prune');if(btn){btn.disabled=true;btn.innerHTML='<span class="spinner"></span> Cleaning...'}
  try{
    const r=await api('/docker/prune',{method:'POST'});
    const msg=`Cleaned up: ${r.imagesDeleted||0} images, ${r.containersDeleted?r.containersDeleted.length:0} containers. Reclaimed ${fB(r.spaceReclaimed||0)}.`;
    toast(msg,'success');
    setTimeout(fetchDash,1000);
  }catch(e){toast('Prune failed: '+e.message,'error')}
  finally{if(btn){btn.disabled=false;btn.textContent='Clean Up'}}
};

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

window.contAct=async function(name,a,stack){
  if(a==='logs'){if(stack){pgStackDetail(stack,'logs');return}navigate('compose');setTimeout(async()=>{if(_term){_term.clear();try{const d=await api(`/containers/${encodeURIComponent(name)}/logs`);_term.write(d.logs||d||'No logs')}catch(e){_term.writeLine(`Error: ${e.message}`,'err')}}},200);return}
  try{await api(`/containers/${encodeURIComponent(name)}/${a}`,{method:'POST'});toast(`${name}: ${a} OK`,'success');setTimeout(fetchDash,1500)}
  catch(e){toast(`${name}: ${e.message}`,'error')}
};

// ============ STACKS (MULTI-STACK DOCKER COMPOSE) ============
function pgStacks(){
  $('#main-content').innerHTML=`<div class="page-header"><h1 class="page-title">Stacks</h1>
<div class="page-actions"><button class="btn btn-sm btn-primary" onclick="stackNewModal()">${ic.plus} New Stack</button></div></div>
<div class="stack-grid" id="stack-grid">${skel(3)}</div>`;
  startPoll(fetchStacks,5000);
}

async function fetchStacks(){
  try{
    const d=await api('/stacks');
    const stacks=d.stacks||[];
    const g=$('#stack-grid');if(!g)return;
    if(!stacks.length){
      g.innerHTML=`<div class="stack-card stack-card-new" onclick="stackNewModal()"><div style="text-align:center;color:var(--tx-d)"><div style="font-size:2rem;margin-bottom:8px">+</div><div>Create your first stack</div></div></div>`;
      return;
    }
    let h='';
    stacks.forEach(s=>{
      const running=s.containers?s.containers.filter(c=>c.status==='running').length:0;
      const total=s.containers?s.containers.length:0;
      const stCls=s.status==='running'?'stack-running':s.status==='partial'?'stack-partial':'stack-stopped';
      const dotCls=s.status==='running'?'status-running':s.status==='partial'?'status-restarting':'status-stopped';
      h+=`<div class="stack-card ${stCls}">
<div class="stack-status"><span class="container-status ${dotCls}"><span class="dot"></span>${esc(s.status)}</span></div>
<div style="font-size:1.1rem;font-weight:600;margin-bottom:4px">${esc(s.name)}</div>
<div class="dim" style="font-size:.82rem;margin-bottom:12px">${total} container${total!==1?'s':''} ${running>0?`(${running} running)`:''}</div>
<div class="stack-actions">
<button class="btn btn-sm btn-primary" onclick="stackAct('${esc(s.name)}','start')" title="Start">${ic.play}</button>
<button class="btn btn-sm btn-danger" onclick="stackAct('${esc(s.name)}','stop')" title="Stop">${ic.stop}</button>
<button class="btn btn-sm btn-secondary" onclick="stackAct('${esc(s.name)}','restart')" title="Restart">${ic.restart}</button>
<button class="btn btn-sm btn-secondary" onclick="stackAct('${esc(s.name)}','pull')" title="Pull">${ic.dl}</button>
<button class="btn btn-sm btn-secondary" onclick="pgStackDetail('${esc(s.name)}')" title="Edit"><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M11 4H4a2 2 0 00-2 2v14a2 2 0 002 2h14a2 2 0 002-2v-7"/><path d="M18.5 2.5a2.121 2.121 0 013 3L12 15l-4 1 1-4 9.5-9.5z"/></svg></button>
</div></div>`;
    });
    h+=`<div class="stack-card stack-card-new" onclick="stackNewModal()"><div style="text-align:center;color:var(--tx-d)"><div style="font-size:2rem;margin-bottom:8px">+</div><div>New Stack</div></div></div>`;
    g.innerHTML=h;
  }catch(e){const g=$('#stack-grid');if(g)g.innerHTML=`<div class="empty-state"><h3>Failed to load stacks</h3><p>${esc(e.message)}</p></div>`}
}

window.stackAct=async function(name,action){
  _showStackTerminal(name,action);
  try{
    const resp=await fetch(`${API}/stacks/${encodeURIComponent(name)}/${action}`,{method:'POST'});
    if(!resp.ok){const err=await resp.json().catch(()=>({error:'HTTP '+resp.status}));if(_stackTerm)_stackTerm.writeLine('Error: '+(err.error||'failed'),'err');return}
    const reader=resp.body.getReader();
    const decoder=new TextDecoder();
    let buffer='';
    while(true){
      const{done,value}=await reader.read();
      if(done)break;
      buffer+=decoder.decode(value,{stream:true});
      const parts=buffer.split('\n\n');buffer=parts.pop();
      for(const part of parts){
        const line=part.replace(/^data: /,'').trim();if(!line)continue;
        try{const data=JSON.parse(line);
          if(data.type==='complete'){
            if(data.success){if(_stackTerm)_stackTerm.writeLine(action+' completed successfully.','ok');toast(`${name}: ${action} completed`,'success')}
            else{if(_stackTerm)_stackTerm.writeLine('Failed: '+(data.error||'unknown'),'err');toast(`${name}: ${action} failed`,'error')}
          }else{if(_stackTerm)_stackTerm.writeLine(data.line||'',data.type==='error'?'err':data.type==='success'?'ok':'')}
        }catch{if(line&&_stackTerm)_stackTerm.write(line)}
      }
    }
    setTimeout(fetchStacks,1500);
  }catch(e){if(_stackTerm)_stackTerm.writeLine('Error: '+e.message,'err')}
};

function _showStackTerminal(name,action){
  let panel=$('#stack-terminal-panel');
  if(!panel){
    panel=document.createElement('div');panel.id='stack-terminal-panel';panel.className='stack-terminal';
    panel.innerHTML=`<div class="stack-terminal-header"><span id="stack-term-title">Output: ${esc(name)}</span><button class="btn-icon" onclick="_closeStackTerm()" title="Close"><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg></button></div><div id="stack-term-body" class="stack-terminal-body"></div>`;
    document.body.appendChild(panel);
  }else{
    const title=$('#stack-term-title');if(title)title.textContent='Output: '+name;
  }
  if(!_stackTerm){
    const body=$('#stack-term-body');
    _stackTerm={
      body:body,lines:[],autoScroll:true,
      writeLine(text,type){const l=document.createElement('div');if(type)l.className='line-'+type;l.textContent=text;body.appendChild(l);if(this.lines.length>500){const old=this.lines.shift();old.remove()}this.lines.push(l);if(this.autoScroll)body.scrollTop=body.scrollHeight},
      write(text){text.split('\n').forEach(l=>{if(l.trim())this.writeLine(l)})},
      clear(){body.innerHTML='';this.lines=[]},
      destroy(){body.innerHTML='';this.lines=[]}
    };
  }else{_stackTerm.clear()}
  _stackTerm.writeLine(`Running ${action} on ${name}...`,'info');
}
window._closeStackTerm=_closeStackTerm;

// ============ STACK DETAIL PAGE ============
function pgStackDetail(name,tab){
  stopPoll();_closeStackTerm();cur='stacks';
  $$('.nav-item').forEach(el=>el.classList.toggle('active',el.dataset.page==='stacks'));
  const m=$('#main-content');
  m.innerHTML=`<div style="margin-bottom:16px"><button class="btn btn-sm btn-secondary" onclick="pgStacks()"><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="19" y1="12" x2="5" y2="12"/><polyline points="12 19 5 12 12 5"/></svg> Back to Stacks</button></div>
<div id="stack-detail-header"><h1 class="page-title">${esc(name)}</h1></div>
<div class="stack-tabs" id="stack-tabs">
<div class="stack-tab active" data-tab="containers" onclick="stackTab('containers')">Containers</div>
<div class="stack-tab" data-tab="compose" onclick="stackTab('compose')">Compose</div>
<div class="stack-tab" data-tab="env" onclick="stackTab('env')">Env</div>
<div class="stack-tab" data-tab="logs" onclick="stackTab('logs')">Logs</div>
</div>
<div id="stack-tab-content">${skel(1)}</div>`;
  window._stackDetailName=name;
  loadStackDetail(name,tab);
}
window.pgStackDetail=pgStackDetail;

async function loadStackDetail(name,tab){
  try{
    const d=await api(`/stacks/${encodeURIComponent(name)}`);
    window._stackData=d;
    const running=d.containers?d.containers.filter(c=>c.status==='running').length:0;
    const total=d.containers?d.containers.length:0;
    const dotCls=d.status==='running'?'status-running':d.status==='partial'?'status-restarting':'status-stopped';
    const hdr=$('#stack-detail-header');
    if(hdr)hdr.innerHTML=`<div class="page-header"><h1 class="page-title">${esc(name)}</h1><span class="container-status ${dotCls}" style="font-size:.85rem"><span class="dot"></span>${esc(d.status)} (${running}/${total})</span></div>`;
    stackTab(tab||'containers');
  }catch(e){$('#stack-tab-content').innerHTML=`<div class="empty-state"><h3>Failed to load stack</h3><p>${esc(e.message)}</p></div>`}
}

window.stackTab=function(tab){
  $$('.stack-tab').forEach(t=>t.classList.toggle('active',t.dataset.tab===tab));
  const c=$('#stack-tab-content');if(!c)return;
  const d=window._stackData||{};const name=window._stackDetailName;
  if(tab==='compose')renderComposeTab(c,d,name);
  else if(tab==='containers')renderContainersTab(c,d,name);
  else if(tab==='logs')renderLogsTab(c,d,name);
  else if(tab==='env')renderEnvTab(c,d,name);
};

function renderComposeTab(c,d,name){
  c.innerHTML=`<div class="form-hint" style="margin-bottom:12px;padding:8px 12px;background:var(--surface-1);border-radius:var(--r-md);border:1px solid var(--bdr)">Volume paths for this stack: <strong>/dockerapps/${esc(name)}/</strong> &mdash; Example: <code>/dockerapps/${esc(name)}/config:/config</code></div>
<div style="margin-bottom:16px"><div class="editor-container" style="min-height:300px"><div class="editor-wrap"><div class="line-numbers" id="stack-line-numbers" aria-hidden="true"></div>
<textarea id="stack-compose-editor" class="compose-textarea" style="min-height:280px" spellcheck="false" placeholder="Paste docker-compose.yml...">${esc(d.compose||'')}</textarea></div></div></div>
<div id="stack-mount-checkboxes" style="margin-bottom:16px"></div>
<div style="display:flex;gap:8px;flex-wrap:wrap">
<button class="btn btn-sm btn-secondary" onclick="stackSaveCompose()">${ic.save} Save</button>
<button class="btn btn-sm btn-primary" onclick="stackAct('${esc(name)}','deploy')">${ic.send} Deploy</button>
<button class="btn btn-sm btn-danger" onclick="stackDelete('${esc(name)}')" style="margin-left:auto">Delete Stack</button>
</div>`;
  const ta=$('#stack-compose-editor');
  if(ta){
    ta.addEventListener('input',()=>_updateStackLineNums());
    ta.addEventListener('scroll',()=>{const ln=$('#stack-line-numbers');if(ln)ln.scrollTop=ta.scrollTop});
    ta.addEventListener('keydown',e=>{if(e.key==='Tab'){e.preventDefault();const s=ta.selectionStart,en=ta.selectionEnd;ta.value=ta.value.substring(0,s)+'  '+ta.value.substring(en);ta.selectionStart=ta.selectionEnd=s+2;_updateStackLineNums()}});
    _updateStackLineNums();
  }
  loadStackMountCheckboxes(name,d.requiredMounts||[]);
}

function _updateStackLineNums(){
  const ta=$('#stack-compose-editor'),ln=$('#stack-line-numbers');if(!ta||!ln)return;
  const lines=ta.value.split('\n').length;let h='';for(let i=1;i<=lines;i++)h+=i+'\n';ln.textContent=h;
}

async function loadStackMountCheckboxes(name,selected){
  try{
    const d=await api('/mounts'),ms=d.mounts||d||[];
    const el=$('#stack-mount-checkboxes');if(!el)return;
    if(!ms.length){el.innerHTML='';return}
    const mountNames=ms.map(m=>m.mountpoint?m.mountpoint.split('/').pop():m.id);
    el.innerHTML=`<div style="font-size:.82rem;font-weight:600;color:var(--tx-d);text-transform:uppercase;letter-spacing:.5px;margin-bottom:8px">Required Mounts</div>
<div style="display:flex;flex-wrap:wrap;gap:12px">${mountNames.map(mn=>`<label style="display:flex;align-items:center;gap:6px;font-size:.88rem;cursor:pointer"><input type="checkbox" class="stack-mount-cb" value="${esc(mn)}" ${selected.includes(mn)?'checked':''} onchange="stackSaveMounts('${esc(name)}')">${esc(mn)}</label>`).join('')}</div>`;
  }catch{}
}

window.stackSaveMounts=async function(name){
  const cbs=$$('.stack-mount-cb');
  const mounts=cbs.filter(cb=>cb.checked).map(cb=>cb.value);
  try{await api(`/stacks/${encodeURIComponent(name)}/mounts`,{method:'PUT',body:{mounts}});toast('Mount requirements updated','success')}
  catch(e){toast('Failed: '+e.message,'error')}
};

window.stackSaveCompose=async function(){
  const name=window._stackDetailName;if(!name)return;
  const ta=$('#stack-compose-editor');if(!ta)return;
  try{await api(`/stacks/${encodeURIComponent(name)}`,{method:'PUT',body:{compose:ta.value}});if(window._stackData)window._stackData.compose=ta.value;toast('Compose file saved','success')}
  catch(e){toast('Save failed: '+e.message,'error')}
};

window.stackDelete=async function(name){
  if(!confirm(`Delete stack "${name}"?\n\nThis will stop all containers and remove the stack directory. This cannot be undone.`))return;
  try{await api(`/stacks/${encodeURIComponent(name)}`,{method:'DELETE'});toast(`Stack ${name} deleted`,'success');pgStacks()}
  catch(e){toast('Delete failed: '+e.message,'error')}
};

function renderContainersTab(c,d){
  const containers=d.containers||[];
  if(!containers.length){c.innerHTML='<div class="empty-state"><h3>No containers</h3><p>Deploy this stack to create containers.</p></div>';return}
  c.innerHTML=`<table class="fb-table"><thead><tr><th>Name</th><th>Image</th><th>Status</th><th>Ports</th><th>Actions</th></tr></thead>
<tbody>${containers.map(ct=>{
    const stCls=ct.status==='running'?'status-running':'status-stopped';
    return`<tr class="fb-row"><td style="font-weight:600">${esc(ct.name)}</td><td class="mono dim">${esc(ct.image)}</td>
<td><span class="container-status ${stCls}"><span class="dot"></span>${esc(ct.status)}</span></td>
<td class="mono dim" style="font-size:.8rem">${esc(ct.ports||'')}</td>
<td><button class="btn btn-sm btn-secondary" onclick="stackRestartContainer('${esc(ct.name)}')">${ic.restart}</button></td></tr>`}).join('')}</tbody></table>`;
}

window.stackRestartContainer=async function(containerName){
  try{await api('/stacks/container/restart',{method:'POST',body:{container:containerName}});toast(`Restarted ${containerName}`,'success');
    const name=window._stackDetailName;if(name)loadStackDetail(name)}
  catch(e){toast('Restart failed: '+e.message,'error')}
};

function renderLogsTab(c,d,name){
  c.innerHTML=`<div style="display:flex;align-items:center;gap:12px;margin-bottom:12px">
<select class="form-input" id="stack-log-container" style="max-width:200px" onchange="stackReconnectLogs()">
<option value="all">All Containers</option>
${(d.containers||[]).map(ct=>`<option value="${esc(ct.name)}">${esc(ct.name)}</option>`).join('')}
</select>
<button class="btn btn-sm btn-secondary" id="stack-log-pause" onclick="stackToggleLogPause()">Pause</button>
<button class="btn btn-sm btn-secondary" onclick="stackReconnectLogs()">Reconnect</button>
</div>
<div id="stack-log-body" style="background:var(--bg-inp);border:1px solid var(--bdr);border-radius:var(--r-md);padding:12px 16px;font-family:var(--mono);font-size:.8rem;line-height:1.5;max-height:500px;overflow-y:auto;white-space:pre-wrap;color:var(--tx-d)"></div>`;
  window._stackLogPaused=false;
  stackConnectLogs(name);
}

function stackConnectLogs(name){
  if(_stackLogWs){_stackLogWs.close();_stackLogWs=null}
  const sel=$('#stack-log-container');
  const container=sel?sel.value:'all';
  const proto=location.protocol==='https:'?'wss':'ws';
  const url=`${proto}://${location.host}/api/stacks/${encodeURIComponent(name)}/logs?container=${encodeURIComponent(container)}`;
  try{
    _stackLogWs=new WebSocket(url);
    const body=$('#stack-log-body');if(!body)return;
    body.innerHTML='';
    _stackLogWs.onmessage=e=>{
      if(window._stackLogPaused)return;
      try{const data=JSON.parse(e.data);
        const line=document.createElement('div');line.textContent=data.data||'';
        body.appendChild(line);
        while(body.children.length>1000)body.firstChild.remove();
        body.scrollTop=body.scrollHeight;
      }catch{const line=document.createElement('div');line.textContent=e.data;body.appendChild(line);body.scrollTop=body.scrollHeight}
    };
    _stackLogWs.onclose=()=>{const line=document.createElement('div');line.className='line-warn';line.textContent='Log stream disconnected';if(body)body.appendChild(line)};
    _stackLogWs.onerror=()=>{};
  }catch{}
}

window.stackReconnectLogs=function(){
  const name=window._stackDetailName;if(name)stackConnectLogs(name);
};
window.stackToggleLogPause=function(){
  window._stackLogPaused=!window._stackLogPaused;
  const btn=$('#stack-log-pause');if(btn)btn.textContent=window._stackLogPaused?'Resume':'Pause';
};

function renderEnvTab(c,d,name){
  c.innerHTML=`<div class="form-hint" style="margin-bottom:12px">Environment variables in KEY=VALUE format, one per line.</div>
<textarea id="stack-env-editor" class="compose-textarea" style="min-height:200px;background:var(--bg-inp);border:1px solid var(--bdr);border-radius:var(--r-md);padding:16px" spellcheck="false" placeholder="KEY=value">${esc(d.env||'')}</textarea>
<div style="margin-top:12px;display:flex;gap:8px"><button class="btn btn-sm btn-secondary" onclick="stackSaveEnv('${esc(name)}')">${ic.save} Save</button><button class="btn btn-sm btn-primary" onclick="stackAct('${esc(name)}','deploy')">${ic.send} Deploy</button></div>`;
}

window.stackSaveEnv=async function(name){
  const ta=$('#stack-env-editor');if(!ta)return;
  try{await api(`/stacks/${encodeURIComponent(name)}`,{method:'PUT',body:{env:ta.value}});if(window._stackData)window._stackData.env=ta.value;toast('Environment saved','success')}
  catch(e){toast('Save failed: '+e.message,'error')}
};

// ============ NEW STACK MODAL ============
window.stackNewModal=function(){
  const existing=$('#stack-new-overlay');if(existing)existing.remove();
  const overlay=document.createElement('div');overlay.id='stack-new-overlay';overlay.className='wizard-overlay wizard-visible';
  overlay.innerHTML=`<div class="wizard-card" style="max-width:700px">
<div class="wizard-header"><div class="wizard-title">New Stack</div>
<button class="wizard-close" onclick="document.getElementById('stack-new-overlay').remove()"><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg></button></div>
<div class="wizard-body">
<div class="form-group"><label class="form-label">App Name</label><input class="form-input" id="stack-new-name" placeholder="my-app" oninput="this.value=this.value.toLowerCase().replace(/[^a-z0-9-]/g,'');stackNewNameHint()">
<div id="stack-new-name-hint" class="form-hint hidden" style="margin-top:6px"></div></div>
<div class="form-group"><label class="form-label">Start From</label>
<div style="display:flex;flex-wrap:wrap;gap:8px;margin-top:4px">
<label class="wizard-role-btn role-active" onclick="stackNewMode('paste',this)"><input type="radio" name="stack-new-mode" value="paste" checked>Paste YAML</label>
<label class="wizard-role-btn" onclick="stackNewMode('upload',this)"><input type="radio" name="stack-new-mode" value="upload">Upload File</label>
<label class="wizard-role-btn" onclick="stackNewMode('convert',this)"><input type="radio" name="stack-new-mode" value="convert">Convert docker run</label>
</div></div>
<div id="stack-new-compose-area">
<div class="form-group"><label class="form-label">docker-compose.yml</label>
<textarea id="stack-new-compose" class="compose-textarea" style="min-height:200px;background:var(--bg-inp);border:1px solid var(--bdr);border-radius:var(--r-md);padding:12px" spellcheck="false" placeholder="services:\n  app:\n    image: nginx:latest\n    ports:\n      - '80:80'"></textarea></div>
</div>
<div id="stack-new-upload-area" class="hidden">
<div class="form-group"><label class="form-label">Upload docker-compose.yml</label>
<label class="btn btn-sm btn-secondary" style="cursor:pointer">${ic.ul} Choose File<input type="file" accept=".yml,.yaml" style="display:none" onchange="stackNewUpload(this)"></label>
<span id="stack-new-upload-name" class="dim" style="margin-left:8px"></span></div>
</div>
<div id="stack-new-convert-area" class="hidden">
<div class="form-group"><label class="form-label">docker run command</label>
<textarea id="stack-new-docker-run" class="compose-textarea" style="min-height:100px;background:var(--bg-inp);border:1px solid var(--bdr);border-radius:var(--r-md);padding:12px" spellcheck="false" placeholder="docker run -d --name myapp -p 80:80 -v /data:/data nginx:latest"></textarea>
<button class="btn btn-sm btn-primary" style="margin-top:8px" onclick="stackConvertRun()">Convert</button></div>
<div id="stack-new-convert-preview" class="hidden">
<label class="form-label">Generated Compose</label>
<textarea id="stack-new-convert-result" class="compose-textarea" style="min-height:150px;background:var(--bg-inp);border:1px solid var(--bdr);border-radius:var(--r-md);padding:12px" spellcheck="false"></textarea>
</div></div>
<div class="form-group"><label class="form-label">.env (optional)</label>
<textarea id="stack-new-env" class="compose-textarea" style="min-height:80px;background:var(--bg-inp);border:1px solid var(--bdr);border-radius:var(--r-md);padding:12px" spellcheck="false" placeholder="KEY=value"></textarea></div>
</div>
<div class="wizard-footer"><div class="wizard-footer-left"></div><div class="wizard-footer-right">
<button class="btn btn-secondary" onclick="document.getElementById('stack-new-overlay').remove()">Cancel</button>
<button class="btn btn-primary" onclick="stackNewCreate()">Create Stack</button>
</div></div></div>`;
  document.body.appendChild(overlay);
};

window.stackNewNameHint=function(){
  const name=$('#stack-new-name');const hint=$('#stack-new-name-hint');
  if(!name||!hint)return;
  const n=name.value.trim();
  if(n){
    hint.classList.remove('hidden');
    hint.innerHTML='Volume paths for this app should use: <strong>/dockerapps/'+esc(n)+'/</strong><br>Example: <code>/dockerapps/'+esc(n)+'/config:/config</code>';
  }else{hint.classList.add('hidden')}
};

window.stackNewMode=function(mode,el){
  $$('#stack-new-overlay .wizard-role-btn').forEach(b=>b.classList.remove('role-active'));
  if(el)el.classList.add('role-active');
  $('#stack-new-compose-area').classList.toggle('hidden',mode==='upload'||mode==='convert');
  $('#stack-new-upload-area').classList.toggle('hidden',mode!=='upload');
  $('#stack-new-convert-area').classList.toggle('hidden',mode!=='convert');
  if(mode==='paste'){
    const ta=$('#stack-new-compose');if(ta)ta.focus();
  }
};

window.stackNewUpload=function(input){
  const file=input.files[0];if(!file)return;
  const nameSpan=$('#stack-new-upload-name');if(nameSpan)nameSpan.textContent=file.name;
  const reader=new FileReader();
  reader.onload=e=>{
    const ta=$('#stack-new-compose');if(ta)ta.value=e.target.result;
    stackNewMode('paste',null);
    $$('#stack-new-overlay .wizard-role-btn').forEach(b=>{
      const r=b.querySelector('input');if(r&&r.value==='paste')b.classList.add('role-active');else b.classList.remove('role-active');
    });
  };
  reader.readAsText(file);
};

window.stackConvertRun=function(){
  const input=$('#stack-new-docker-run');if(!input)return;
  const nameEl=$('#stack-new-name');
  const appName=nameEl?nameEl.value.trim():'';
  const yaml=convertDockerRun(input.value,appName);
  if(!yaml){toast('Could not parse docker run command','error');return}
  const preview=$('#stack-new-convert-preview');if(preview)preview.classList.remove('hidden');
  const result=$('#stack-new-convert-result');if(result)result.value=yaml;
};

window.stackNewCreate=async function(){
  const nameEl=$('#stack-new-name');if(!nameEl)return;
  const name=nameEl.value.trim();
  if(!name){toast('Stack name is required','error');return}
  // Get compose content from either the main textarea or the convert result
  let compose='';
  const convertResult=$('#stack-new-convert-result');
  const mainCompose=$('#stack-new-compose');
  if(convertResult&&!$('#stack-new-convert-area').classList.contains('hidden')&&convertResult.value.trim()){
    compose=convertResult.value;
  }else if(mainCompose){
    compose=mainCompose.value;
  }
  if(!compose.trim()){toast('Compose content is required','error');return}
  const envEl=$('#stack-new-env');
  const env=envEl?envEl.value:'';
  try{
    await api('/stacks',{method:'POST',body:{name,compose,env}});
    toast(`Stack ${name} created`,'success');
    const overlay=$('#stack-new-overlay');if(overlay)overlay.remove();
    pgStacks();
  }catch(e){toast('Create failed: '+e.message,'error')}
};

// ============ DOCKER RUN CONVERTER (CLIENT-SIDE) ============
function convertDockerRun(cmd,appName){
  if(!cmd||!cmd.trim())return null;
  cmd=cmd.trim();
  // Remove leading 'docker run' or 'docker create'
  cmd=cmd.replace(/^(sudo\s+)?docker\s+(run|create)\s+/i,'');
  // Normalize: remove backslash line continuations before tokenizing
  cmd=cmd.replace(/\\\s*\n\s*/g,' ').replace(/\\\s*\r\n?\s*/g,' ');
  // Collapse multiple spaces
  cmd=cmd.replace(/\s+/g,' ').trim();
  // Parse tokens respecting quotes
  const tokens=[];let current='',inQ=false,qChar='';
  for(let i=0;i<cmd.length;i++){
    const ch=cmd[i];
    if(inQ){if(ch===qChar){inQ=false}else{current+=ch}}
    else if(ch==='"'||ch==="'"){inQ=true;qChar=ch}
    else if(ch===' '||ch==='\t'){if(current){tokens.push(current);current=''}}
    else{current+=ch}
  }
  if(current)tokens.push(current);
  if(!tokens.length)return null;

  const svc={};const networks=[];
  let image=null,command=[];let i=0;
  while(i<tokens.length){
    const t=tokens[i];
    if(t==='-d'||t==='--detach'){i++;continue}
    if(t==='--name'&&tokens[i+1]){svc.container_name=tokens[++i];i++;continue}
    if((t==='-p'||t==='--publish')&&tokens[i+1]){if(!svc.ports)svc.ports=[];svc.ports.push(tokens[++i]);i++;continue}
    if(t.startsWith('-p=')){ if(!svc.ports)svc.ports=[];svc.ports.push(t.slice(3));i++;continue}
    if((t==='-v'||t==='--volume')&&tokens[i+1]){if(!svc.volumes)svc.volumes=[];svc.volumes.push(tokens[++i]);i++;continue}
    if(t.startsWith('-v=')){ if(!svc.volumes)svc.volumes=[];svc.volumes.push(t.slice(3));i++;continue}
    if((t==='-e'||t==='--env')&&tokens[i+1]){if(!svc.environment)svc.environment=[];svc.environment.push(tokens[++i]);i++;continue}
    if(t.startsWith('-e=')){ if(!svc.environment)svc.environment=[];svc.environment.push(t.slice(3));i++;continue}
    if(t==='--env-file'&&tokens[i+1]){svc.env_file=tokens[++i];i++;continue}
    if(t==='--restart'&&tokens[i+1]){svc.restart=tokens[++i];i++;continue}
    if(t.startsWith('--restart=')){svc.restart=t.split('=',2)[1];i++;continue}
    if(t==='--network'&&tokens[i+1]){networks.push(tokens[++i]);i++;continue}
    if(t.startsWith('--network=')){networks.push(t.split('=',2)[1]);i++;continue}
    if(t==='--privileged'){svc.privileged=true;i++;continue}
    if(t==='--cap-add'&&tokens[i+1]){if(!svc.cap_add)svc.cap_add=[];svc.cap_add.push(tokens[++i]);i++;continue}
    if(t==='--label'&&tokens[i+1]){if(!svc.labels)svc.labels=[];svc.labels.push(tokens[++i]);i++;continue}
    if((t==='-w'||t==='--workdir')&&tokens[i+1]){svc.working_dir=tokens[++i];i++;continue}
    if(t==='--hostname'&&tokens[i+1]){svc.hostname=tokens[++i];i++;continue}
    if(t==='--user'&&tokens[i+1]){svc.user=tokens[++i];i++;continue}
    if(t==='--entrypoint'&&tokens[i+1]){svc.entrypoint=tokens[++i];i++;continue}
    if((t==='-m'||t==='--memory')&&tokens[i+1]){if(!svc.deploy)svc.deploy={resources:{limits:{}}};svc.deploy.resources.limits.memory=tokens[++i];i++;continue}
    if(t==='--cpus'&&tokens[i+1]){if(!svc.deploy)svc.deploy={resources:{limits:{}}};svc.deploy.resources.limits.cpus=tokens[++i];i++;continue}
    // Skip unknown flags
    if(t.startsWith('-')&&!t.startsWith('-/')){
      // Check if next token is a value (not a flag)
      if(tokens[i+1]&&!tokens[i+1].startsWith('-'))i++;
      i++;continue;
    }
    // First non-flag token is the image
    if(!image){image=t;i++;continue}
    // Remaining tokens are the command
    command.push(t);i++;
  }
  if(!image)return null;
  svc.image=image;
  if(command.length)svc.command=command.join(' ');
  if(networks.length){svc.networks=networks}

  // Build YAML
  const serviceName=svc.container_name||image.split(':')[0].split('/').pop()||'app';
  let yaml='services:\n  '+serviceName+':\n';
  yaml+='    image: '+svc.image+'\n';
  if(svc.container_name)yaml+='    container_name: '+svc.container_name+'\n';
  if(svc.command)yaml+='    command: '+svc.command+'\n';
  if(svc.entrypoint)yaml+='    entrypoint: '+svc.entrypoint+'\n';
  if(svc.restart)yaml+='    restart: '+svc.restart+'\n';
  if(svc.hostname)yaml+='    hostname: '+svc.hostname+'\n';
  if(svc.user)yaml+='    user: "'+svc.user+'"\n';
  if(svc.working_dir)yaml+='    working_dir: '+svc.working_dir+'\n';
  if(svc.privileged)yaml+='    privileged: true\n';
  if(svc.env_file)yaml+='    env_file: '+svc.env_file+'\n';
  if(svc.ports){yaml+='    ports:\n';svc.ports.forEach(p=>yaml+='      - "'+p+'"\n')}
  if(svc.volumes){yaml+='    volumes:\n';svc.volumes.forEach(v=>{
    // Auto-rewrite volume host paths to /dockerapps/APPNAME/ when app name is set
    if(appName){
      const parts=v.split(':');
      if(parts.length>=2){
        let hostPath=parts[0];
        // Rewrite absolute paths that aren't already under /dockerapps
        if(hostPath.startsWith('/')&&!hostPath.startsWith('/dockerapps/')){
          const dirName=hostPath.split('/').filter(Boolean).pop()||'data';
          parts[0]='/dockerapps/'+appName+'/'+dirName;
        }
        // Rewrite named volumes to local paths
        else if(!hostPath.startsWith('/')&&!hostPath.startsWith('.')){
          parts[0]='/dockerapps/'+appName+'/'+hostPath;
        }
        v=parts.join(':');
      }
    }
    yaml+='      - '+v+'\n'})}
  if(svc.environment){yaml+='    environment:\n';svc.environment.forEach(e=>yaml+='      - '+e+'\n')}
  if(svc.cap_add){yaml+='    cap_add:\n';svc.cap_add.forEach(c=>yaml+='      - '+c+'\n')}
  if(svc.labels){yaml+='    labels:\n';svc.labels.forEach(l=>yaml+='      - '+l+'\n')}
  if(svc.networks){yaml+='    networks:\n';svc.networks.forEach(n=>yaml+='      - '+n+'\n')}
  if(svc.deploy){
    yaml+='    deploy:\n      resources:\n        limits:\n';
    if(svc.deploy.resources.limits.memory)yaml+='          memory: '+svc.deploy.resources.limits.memory+'\n';
    if(svc.deploy.resources.limits.cpus)yaml+='          cpus: "'+svc.deploy.resources.limits.cpus+'"\n';
  }
  // Add top-level networks if needed
  if(networks.length){yaml+='\nnetworks:\n';networks.forEach(n=>yaml+='  '+n+':\n    external: true\n')}
  return yaml;
}

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
<li>WraithOS boots from RAM. Only <code>/dockerapps</code> and <code>/remotemounts/*</code> survive reboots (when on persistent disks).</li>
<li>Named Docker volumes are stored under <code>/wraith/cache</code> automatically.</li>
<li>Use bind mounts for network storage: <code>/remotemounts/&lt;name&gt;:/container/path</code></li>
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
<div class="form-hint" id="mnt-path-hint">Mounts at /remotemounts/&lt;name&gt;</div></div>
<div id="mnt-creds-group">
<div class="form-group"><label class="form-label">Username</label><input class="form-input" id="mnt-user" placeholder="optional"></div>
<div class="form-group"><label class="form-label">Password</label><input class="form-input" id="mnt-pass" type="password" placeholder="optional"></div>
</div>
<div class="form-group"><label class="form-label">Options</label><input class="form-input" id="mnt-opts" placeholder="ro,vers=3.0"><div class="form-hint" id="mnt-opts-hint">Additional mount options</div></div>
<div class="form-group" style="grid-column:1/-1"><label style="display:flex;align-items:center;gap:8px;cursor:pointer"><input type="checkbox" id="mnt-docker-req"> <span class="form-label" style="margin:0">Required for Docker</span></label><div class="form-hint">Auto-remount and restart Docker stack if this mount goes down</div></div>
</div><div class="form-actions"><button class="btn btn-primary" onclick="addMnt()">Add Mount</button>
<button class="btn btn-secondary" onclick="hideMntForm()">Cancel</button></div></div></div>
<div class="mount-list" id="mount-list">${skel(2)}</div>`;
  fetchMnts();
}

async function fetchMnts(){
  try{
    const d=await api('/mounts'),ms=d.mounts||d||[],l=$('#mount-list');if(!l)return;
    if(!ms.length){l.innerHTML='<div class="empty-state"><h3>No mounts configured</h3><p>Add a network mount (SMB/CIFS or NFS) to share files with containers.</p></div>';return}
    l.innerHTML=ms.map((m,i)=>{const t=(m.type||'cifs').toUpperCase();const src=m.type==='nfs'?`${esc(m.server)}:${esc(m.share)}`:`//${esc(m.server)}/${esc(m.share)}`;
    const mid=esc(m.id||m.mountpoint);
    return`<div class="mount-card" style="animation-delay:${i*60}ms"><div class="mount-info">
<h3><span class="container-status ${m.mounted?'status-running':'status-stopped'}"><span class="dot"></span>${m.mounted?'mounted':'unmounted'}</span> ${esc(m.mountpoint||m.path)}${m.dockerRequired?'<span style="margin-left:8px;font-size:.75rem;padding:2px 8px;border-radius:4px;background:var(--ac-glow);border:1px solid var(--ac-dim);color:var(--ac)">Docker</span>':''}</h3>
<dl class="mount-details"><dt>Type</dt><dd>${t}</dd><dt>Source</dt><dd>${src}</dd>${m.type!=='nfs'?`<dt>User</dt><dd>${esc(m.username||'guest')}</dd>`:''}<dt>Options</dt><dd>${esc(m.options||'defaults')}</dd></dl>
${(m.volumes&&m.volumes.length)?`<div class="mount-volumes"><div class="label">Used by volumes</div>${m.volumes.map(v=>`<span class="volume-tag">${esc(v)}</span>`).join('')}</div>`:''}</div>
<div class="mount-actions"><label style="display:flex;align-items:center;gap:4px;font-size:.82rem;color:var(--tx-d);cursor:pointer;margin-right:8px" title="Auto-remount and restart Docker stack if unmounted"><input type="checkbox" ${m.dockerRequired?'checked':''} onchange="toggleDockerReq('${mid}',this.checked)"> Docker</label>${m.mounted?`<button class="btn btn-sm btn-secondary" data-mnt-action="unmount" data-mnt-id="${mid}">Unmount</button>`
:`<button class="btn btn-sm btn-primary" data-mnt-action="mount" data-mnt-id="${mid}">Mount</button>`}
<button class="btn btn-sm btn-danger" data-mnt-action="delete" data-mnt-id="${mid}">Remove</button></div></div>`}).join('');
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
  if(hint)hint.textContent=v?`Mounts at /remotemounts/${v}`:'Mounts at /remotemounts/<name>';
};
window.addMnt=async function(){
  const t=document.querySelector('input[name="mnt-type"]:checked').value;
  const d={type:t,server:$('#mnt-server').value,share:$('#mnt-share').value,mountpoint:$('#mnt-name').value,
    options:$('#mnt-opts').value,dockerRequired:!!$('#mnt-docker-req').checked};
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
window.toggleDockerReq=async function(id,required){
  try{await api(`/mounts/${encodeURIComponent(id)}/docker-required`,{method:'PUT',body:{required}});
  toast(required?'Mount marked as required for Docker':'Docker requirement removed','success')}
  catch(e){toast(`Error: ${e.message}`,'error');fetchMnts()}
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
<div class="fb-bulk-bar hidden" id="fb-bulk-bar">
<span class="fb-bulk-count" id="fb-bulk-count">0 selected</span>
<button class="btn btn-sm btn-secondary" onclick="fileBulkCopy()"><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 01-2-2V4a2 2 0 012-2h9a2 2 0 012 2v1"/></svg> Copy</button>
<button class="btn btn-sm btn-secondary" onclick="fileBulkMove()"><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M5 12h14M12 5l7 7-7 7"/></svg> Move</button>
<button class="btn btn-sm btn-danger" onclick="fileBulkDelete()"><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 01-2 2H7a2 2 0 01-2-2V6m3 0V4a2 2 0 012-2h4a2 2 0 012 2v2"/></svg> Delete</button>
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

function fileUpdateBulkToolbar(){
  const bar=$('#fb-bulk-bar');
  if(!bar)return;
  const count=_fileSelection.size;
  if(count>0){
    bar.classList.remove('hidden');
    $('#fb-bulk-count').textContent=count+' selected';
  }else{
    bar.classList.add('hidden');
  }
}

function fileToggleSelect(path,checked){
  if(checked)_fileSelection.add(path);else _fileSelection.delete(path);
  fileUpdateBulkToolbar();
}
window.fileToggleSelect=fileToggleSelect;

function fileSelectAll(checked){
  _fileSelection.clear();
  $$('#fb-body input[type="checkbox"]').forEach(cb=>{
    cb.checked=checked;
    const row=cb.closest('tr');
    if(row&&checked)_fileSelection.add(row.dataset.path);
  });
  fileUpdateBulkToolbar();
}
window.fileSelectAll=fileSelectAll;

async function fileBulkDelete(){
  const paths=[..._fileSelection];
  if(!paths.length)return;
  if(!confirm(`Delete ${paths.length} item(s)? This cannot be undone.`))return;
  let ok=0,fail=0;
  for(const p of paths){
    try{await api('/files/delete',{method:'POST',body:{path:p}});ok++}catch{fail++}
  }
  toast(`Deleted ${ok} item(s)${fail?' ('+fail+' failed)':''}`,'success');
  _fileSelection.clear();fileUpdateBulkToolbar();fileBrowse(_fileCurPath);
}
window.fileBulkDelete=fileBulkDelete;

function fileBulkCopy(){
  const paths=[..._fileSelection];
  if(!paths.length)return;
  fileShowCopyModal(paths,paths.map(p=>p.split('/').pop()),false);
}
window.fileBulkCopy=fileBulkCopy;

function fileBulkMove(){
  const paths=[..._fileSelection];
  if(!paths.length)return;
  fileShowCopyModal(paths,paths.map(p=>p.split('/').pop()),true);
}
window.fileBulkMove=fileBulkMove;

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

// ---- Copy/Move Modal with folder browser ----
let _copyModalState={sources:[],names:[],isMove:false,curPath:''};

function fileShowCopyModal(sources,names,isMove){
  if(!_fileRoots||!_fileRoots.length){toast('No destinations available','error');return}
  _copyModalState={sources:Array.isArray(sources)?sources:[sources],names:Array.isArray(names)?names:[names],isMove:!!isMove,curPath:_fileRoots[0].path};
  const label=isMove?'Move':'Copy';
  const fileLabel=_copyModalState.names.length===1?'"'+_copyModalState.names[0]+'"':_copyModalState.names.length+' items';
  // Build modal HTML
  let html=`<div class="wizard-overlay wizard-visible" id="copy-modal-overlay" onclick="if(event.target===this)fileCopyModalClose()">
<div class="wizard-card" style="max-width:560px">
<div class="wizard-header"><div class="wizard-title">${label} ${esc(fileLabel)}</div>
<button class="wizard-close" onclick="fileCopyModalClose()"><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M18 6L6 18M6 6l12 12"/></svg></button></div>
<div class="wizard-body" style="padding:16px 24px">
<div class="fb-roots" id="copy-modal-roots" style="margin-bottom:12px"></div>
<div class="fb-breadcrumb" id="copy-modal-breadcrumb" style="margin-bottom:12px"></div>
<div class="copy-modal-list" id="copy-modal-list" style="max-height:280px;overflow-y:auto;border:1px solid var(--bdr);border-radius:var(--r-md);background:var(--bg-inp)"></div>
<div style="margin-top:12px"><button class="btn btn-sm btn-secondary" onclick="fileCopyModalNewFolder()"><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg> New Folder</button></div>
</div>
<div class="wizard-footer"><div class="wizard-footer-left"><button class="btn btn-secondary" onclick="fileCopyModalClose()">Cancel</button></div>
<div class="wizard-footer-right"><button class="btn btn-primary" id="copy-modal-confirm" onclick="fileCopyModalConfirm()">${label} Here</button></div></div>
</div></div>`;
  // Remove existing modal if any
  const existing=$('#copy-modal-overlay');
  if(existing)existing.remove();
  document.body.insertAdjacentHTML('beforeend',html);
  // Render roots
  const rootsEl=$('#copy-modal-roots');
  rootsEl.innerHTML=_fileRoots.map(r=>`<button class="btn btn-sm fb-root-btn" onclick="fileCopyModalBrowse('${esc(r.path)}')" title="${esc(r.path)}">
<span class="fb-root-type fb-type-${esc(r.type)}">${r.type==='local'?'LOCAL':r.type==='volumes'?'VOL':r.type==='nfs'?'NFS':'SMB'}</span>
${esc(r.name)}</button>`).join('');
  fileCopyModalBrowse(_copyModalState.curPath);
}

function fileCopyModalClose(){
  const overlay=$('#copy-modal-overlay');
  if(overlay)overlay.remove();
}
window.fileCopyModalClose=fileCopyModalClose;

async function fileCopyModalBrowse(path){
  _copyModalState.curPath=path;
  // Update breadcrumb
  const bcEl=$('#copy-modal-breadcrumb');
  if(bcEl)bcEl.innerHTML=fileBreadcrumb(path);
  // Highlight active root
  $$('#copy-modal-roots .fb-root-btn').forEach(btn=>{
    const btnPath=btn.getAttribute('title');
    btn.classList.toggle('fb-root-active',path===btnPath||path.startsWith(btnPath+'/'));
  });
  const list=$('#copy-modal-list');
  if(!list)return;
  list.innerHTML='<div style="padding:16px;text-align:center;color:var(--tx-d)">Loading...</div>';
  try{
    const d=await api('/files/list?'+new URLSearchParams({path}));
    const dirs=(d.files||[]).filter(f=>f.isDir);
    if(!dirs.length){
      list.innerHTML='<div style="padding:16px;text-align:center;color:var(--tx-d);font-size:.85rem">No subdirectories</div>';
      return;
    }
    list.innerHTML=dirs.map(f=>{
      const fp=path+'/'+f.name;
      return`<div class="copy-modal-dir" onclick="fileCopyModalBrowse('${esc(fp)}')" style="display:flex;align-items:center;gap:8px;padding:8px 14px;cursor:pointer;border-bottom:1px solid var(--bdr);transition:background 150ms">
<span class="fi fi-dir">&#128193;</span><span style="font-size:.88rem">${esc(f.name)}</span>
<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="var(--tx-m)" stroke-width="2" style="margin-left:auto"><path d="M9 18l6-6-6-6"/></svg></div>`}).join('');
  }catch(e){list.innerHTML=`<div style="padding:16px;text-align:center;color:var(--red);font-size:.85rem">${esc(e.message)}</div>`}
}
window.fileCopyModalBrowse=fileCopyModalBrowse;

async function fileCopyModalNewFolder(){
  const name=prompt('New folder name:');
  if(!name||!name.trim())return;
  try{
    await api('/files/mkdir',{method:'POST',body:{path:_copyModalState.curPath+'/'+name.trim()}});
    toast('Folder created','success');
    fileCopyModalBrowse(_copyModalState.curPath);
  }catch(e){toast('Failed to create folder: '+e.message,'error')}
}
window.fileCopyModalNewFolder=fileCopyModalNewFolder;

async function fileCopyModalConfirm(){
  const dest=_copyModalState.curPath;
  const isMove=_copyModalState.isMove;
  const endpoint=isMove?'/files/move':'/files/copy';
  const label=isMove?'Moving':'Copying';
  const past=isMove?'Moved':'Copied';
  const btn=$('#copy-modal-confirm');
  if(btn){btn.disabled=true;btn.textContent=label+'...';}
  let ok=0,fail=0;
  for(let i=0;i<_copyModalState.sources.length;i++){
    const src=_copyModalState.sources[i];
    const name=_copyModalState.names[i];
    try{
      await api(endpoint,{method:'POST',body:{source:src,destination:dest+'/'+name}});
      ok++;
    }catch(e){fail++;toast(`Failed: ${name} - ${e.message}`,'error')}
  }
  if(ok)toast(`${past} ${ok} item(s)`,'success');
  fileCopyModalClose();
  fileBrowse(_fileCurPath);
}
window.fileCopyModalConfirm=fileCopyModalConfirm;

function fileCopyPrompt(path,name,isDir){
  fileShowCopyModal([path],[name],false);
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

// ============ DOCKER NETWORKS ============
function pgDockerNetworks(){
  $('#main-content').innerHTML=`<div class="page-header"><h1 class="page-title">Docker Networks</h1>
<div class="page-actions"><button class="btn btn-sm btn-primary" onclick="dnetShowCreate()">${ic.plus} Create Network</button></div></div>
<div id="dnet-create-form" class="hidden" style="margin-bottom:24px"><div class="form-card">
<h3 style="font-size:.95rem;font-weight:600;margin-bottom:16px">Create Network</h3>
<div class="form-grid">
<div class="form-group"><label class="form-label">Name</label><input class="form-input" id="dnet-name" placeholder="my-network"></div>
<div class="form-group"><label class="form-label">Driver</label><select class="form-input" id="dnet-driver" onchange="dnetDriverChanged()">
<option value="bridge">bridge</option><option value="macvlan">macvlan</option><option value="ipvlan">ipvlan</option>
<option value="host">host</option><option value="overlay">overlay</option><option value="none">none</option></select></div>
<div class="form-group"><label class="form-label">Subnet</label><input class="form-input" id="dnet-subnet" placeholder="172.20.0.0/16"><div class="form-hint">CIDR notation (optional for bridge)</div></div>
<div class="form-group"><label class="form-label">Gateway</label><input class="form-input" id="dnet-gateway" placeholder="172.20.0.1"><div class="form-hint">Optional</div></div>
<div class="form-group"><label class="form-label">IP Range</label><input class="form-input" id="dnet-iprange" placeholder="172.20.0.0/24"><div class="form-hint">Optional</div></div>
<div class="form-group" id="dnet-parent-group" style="display:none"><label class="form-label">Parent Interface</label><input class="form-input" id="dnet-parent" placeholder="eth0"><div class="form-hint">Required for macvlan/ipvlan</div></div>
<div class="form-group" style="grid-column:1/-1"><label style="display:flex;align-items:center;gap:8px;cursor:pointer;font-size:.9rem">
<input type="checkbox" id="dnet-internal"> Internal network (no external access)</label></div>
</div>
<div class="form-actions"><button class="btn btn-primary" onclick="dnetCreate()">Create</button>
<button class="btn btn-secondary" onclick="dnetHideCreate()">Cancel</button></div>
</div></div>
<div id="dnet-list" class="stagger">${skel(3)}</div>`;
  dnetFetch();
}

window.dnetShowCreate=function(){$('#dnet-create-form').classList.remove('hidden')};
window.dnetHideCreate=function(){$('#dnet-create-form').classList.add('hidden')};

window.dnetDriverChanged=function(){
  const drv=$('#dnet-driver').value;
  const pg=$('#dnet-parent-group');
  if(pg)pg.style.display=(drv==='macvlan'||drv==='ipvlan')?'':'none';
};

async function dnetFetch(){
  try{
    const d=await api('/docker/networks');
    const nets=d.networks||[];
    const el=$('#dnet-list');if(!el)return;
    if(!nets.length){el.innerHTML='<div class="empty-state"><h3>No networks</h3><p>Docker has no networks configured.</p></div>';return}
    el.innerHTML=nets.map((n,i)=>{
      const builtIn=['bridge','host','none'].includes(n.name);
      return`<div class="card" style="margin-bottom:12px;animation-delay:${i*60}ms">
<div style="display:flex;align-items:center;justify-content:space-between;padding:16px">
<div style="flex:1">
<div style="display:flex;align-items:center;gap:8px;margin-bottom:8px">
<span style="font-weight:600;font-size:.95rem">${esc(n.name)}</span>
<span class="badge" style="font-size:.75rem;padding:2px 8px;border-radius:4px;background:var(--bg-card);border:1px solid var(--bd)">${esc(n.driver)}</span>
${n.internal?'<span class="badge" style="font-size:.75rem;padding:2px 8px;border-radius:4px;background:var(--yel);color:#000">internal</span>':''}
</div>
<dl class="container-meta" style="margin:0">
<dt>ID</dt><dd><code style="font-size:.8rem">${esc(n.id)}</code></dd>
<dt>Scope</dt><dd>${esc(n.scope)}</dd>
${n.subnet?`<dt>Subnet</dt><dd>${esc(n.subnet)}</dd>`:''}
${n.gateway?`<dt>Gateway</dt><dd>${esc(n.gateway)}</dd>`:''}
<dt>Containers</dt><dd>${n.containers}</dd>
</dl></div>
${builtIn?'':`<button class="btn btn-sm btn-danger" onclick="dnetDelete('${esc(n.id)}','${esc(n.name)}')">Delete</button>`}
</div></div>`}).join('');
  }catch(e){
    const el=$('#dnet-list');if(el)el.innerHTML=`<div class="empty-state"><h3>Error</h3><p>${esc(e.message)}</p></div>`;
  }
}

window.dnetCreate=async function(){
  const name=$('#dnet-name').value.trim();
  if(!name){toast('Network name is required','error');return}
  const body={
    name,
    driver:$('#dnet-driver').value,
    subnet:$('#dnet-subnet').value.trim(),
    gateway:$('#dnet-gateway').value.trim(),
    ipRange:$('#dnet-iprange').value.trim(),
    internal:$('#dnet-internal').checked,
    parent:$('#dnet-parent').value.trim()
  };
  try{
    await api('/docker/networks',{method:'POST',body});
    toast(`Network "${name}" created`,'success');
    dnetHideCreate();
    $('#dnet-name').value='';$('#dnet-subnet').value='';$('#dnet-gateway').value='';
    $('#dnet-iprange').value='';$('#dnet-parent').value='';$('#dnet-internal').checked=false;
    dnetFetch();
  }catch(e){toast(`Failed: ${e.message}`,'error')}
};

window.dnetDelete=async function(id,name){
  if(!confirm(`Delete network "${name}"?\n\nThis cannot be undone.`))return;
  try{
    await api('/docker/networks',{method:'DELETE',body:{id}});
    toast(`Network "${name}" deleted`,'success');
    dnetFetch();
  }catch(e){toast(`Failed: ${e.message}`,'error')}
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
<div class="danger-item" style="display:flex;align-items:center;justify-content:space-between;padding:12px 0;border-bottom:1px solid var(--bd)">
<div><div style="font-weight:600;font-size:.9rem;margin-bottom:2px">Docker System Prune</div>
<div style="font-size:.82rem;color:var(--tx-d)">Remove all stopped containers, unused networks, dangling images, and build cache. Running containers are not affected.</div></div>
<button class="btn btn-danger" onclick="dockerPrune()" id="btn-docker-prune">System Prune</button></div>
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
    if(!d.installed){
      if(cb){cb.checked=false;cb.disabled=true}
      if(lbl)lbl.textContent='SSH Not Available';
      if(st)st.innerHTML='<span style="color:var(--tx-d);font-size:.82rem">openssh-server not installed</span>';
      return;
    }
    if(cb){cb.disabled=false;cb.checked=!!d.enabled}
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

window.dockerPrune=async function(){
  if(!confirm('Run Docker System Prune?\n\nThis will remove:\n- All stopped containers\n- All unused networks\n- All unused images (not just dangling)\n- All build cache\n\nRunning containers are NOT affected.\n\nThis action cannot be undone.'))return;
  const btn=$('#btn-docker-prune');if(btn){btn.disabled=true;btn.textContent='Pruning...'}
  try{
    const d=await api('/docker/prune',{method:'POST'});
    const parts=[];
    if(d.containersDeleted&&d.containersDeleted.length)parts.push(`${d.containersDeleted.length} containers`);
    if(d.imagesDeleted)parts.push(`${d.imagesDeleted} images`);
    if(d.networksDeleted&&d.networksDeleted.length)parts.push(`${d.networksDeleted.length} networks`);
    parts.push(`${fB(d.spaceReclaimed||0)} reclaimed`);
    toast(`Prune complete: ${parts.join(', ')}`,'success');
  }catch(e){toast(`Prune failed: ${e.message}`,'error')}
  finally{if(btn){btn.disabled=false;btn.textContent='System Prune'}}
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
