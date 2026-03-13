/* ============================================
   WraithOS Setup Wizard
   Multi-step first-run disk setup, network,
   timezone configuration modal
   ============================================ */

(function(){
'use strict';

const $=(s,p)=>(p||document).querySelector(s);
const $$=(s,p)=>[...(p||document).querySelectorAll(s)];

// Lazy-access shared utils from wraith.js via window._w.
// Wrappers resolve at call time (not IIFE time) so the reference is
// always live, even if this script executes before wraith.js finishes.
function api(path,opts){
  const fn=window._w&&window._w.api;
  if(!fn)return Promise.reject(new Error('API not available'));
  return fn(path,opts);
}
function toast(msg,type){
  const fn=window._w&&window._w.toast;
  if(fn)fn(msg,type);
}

// Wizard state
let wizState={
  step:1,
  totalSteps:6,
  status:null,        // GET /api/setup/status response
  assignments:{},     // device -> 'config' | 'cache' | null
  networkData:null,   // network config for step 4
  timezone:'UTC',     // selected timezone
  formatResult:null,  // POST /api/setup/disks response
};

// ============ PUBLIC API ============

window.setupWizard={
  // Check if wizard should show, and show it
  async checkAndShow(){
    try{
      const s=await api('/setup/status');
      wizState.status=s;
      if(s.needsDiskSetup){
        // Check dismiss cookie
        if(document.cookie.includes('wraith-wizard-dismissed=1'))return;
        this.show();
      }
    }catch(e){
      // API not available yet, skip silently
    }
  },

  // Force show (from dashboard banner or settings)
  async show(){
    if(!wizState.status){
      try{wizState.status=await api('/setup/status')}
      catch(e){toast('Could not load setup status','error');return}
    }
    wizState.step=1;
    wizState.assignments={};
    wizState.formatResult=null;
    renderWizard();
  },

  close(){
    const overlay=$('#setup-wizard-overlay');
    if(overlay){
      overlay.classList.add('wizard-exit');
      setTimeout(()=>overlay.remove(),200);
    }
  },

  dismiss(){
    // Set cookie to suppress auto-show (30 day expiry)
    document.cookie='wraith-wizard-dismissed=1;path=/;max-age=2592000;SameSite=Strict';
    this.close();
  }
};

// ============ RENDER ============

function renderWizard(){
  // Remove existing
  const existing=$('#setup-wizard-overlay');
  if(existing)existing.remove();

  const overlay=document.createElement('div');
  overlay.id='setup-wizard-overlay';
  overlay.className='wizard-overlay';
  overlay.innerHTML=`
<div class="wizard-card">
  <div class="wizard-header">
    <div class="wizard-title">WraithOS Setup</div>
    <button class="btn-icon wizard-close" onclick="setupWizard.close()" title="Close" aria-label="Close wizard">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
    </button>
  </div>
  <div class="wizard-steps" id="wizard-steps"></div>
  <div class="wizard-body" id="wizard-body"></div>
  <div class="wizard-footer" id="wizard-footer"></div>
</div>`;

  document.body.appendChild(overlay);
  requestAnimationFrame(()=>overlay.classList.add('wizard-visible'));
  renderStep();
}

function renderStep(){
  renderStepIndicator();
  const body=$('#wizard-body');
  const footer=$('#wizard-footer');
  if(!body||!footer)return;

  switch(wizState.step){
    case 1:renderStepWelcome(body,footer);break;
    case 2:renderStepDisks(body,footer);break;
    case 3:renderStepConfirm(body,footer);break;
    case 4:renderStepNetwork(body,footer);break;
    case 5:renderStepTimezone(body,footer);break;
    case 6:renderStepSummary(body,footer);break;
  }
}

function renderStepIndicator(){
  const el=$('#wizard-steps');
  if(!el)return;
  const labels=['Welcome','Disks','Format','Network','Timezone','Summary'];
  el.innerHTML=labels.map((l,i)=>{
    const n=i+1;
    const cls=n<wizState.step?'wizard-step done':n===wizState.step?'wizard-step active':'wizard-step';
    return `<div class="${cls}"><span class="wizard-step-num">${n<wizState.step?checkSVG():n}</span><span class="wizard-step-label">${l}</span></div>`;
  }).join('');
}

function checkSVG(){
  return '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="20 6 9 17 4 12"/></svg>';
}

// ============ STEP 1: WELCOME ============

function renderStepWelcome(body,footer){
  const s=wizState.status;
  const cfgPersist=s.configDisk&&s.configDisk.persistent;
  const cachePersist=s.cacheDisk&&s.cacheDisk.persistent;

  body.innerHTML=`
<div class="wizard-step-content">
  <h2>Welcome to WraithOS</h2>
  <p class="dim" style="margin-bottom:24px">Let's configure your system for persistent storage. Right now your disks are set up as follows:</p>
  <div class="wizard-disk-status">
    <div class="wizard-status-row">
      <span class="wizard-status-label">Config Disk</span>
      ${cfgPersist
        ?`<span class="wizard-status-badge badge-ok">Persistent (${esc(s.configDisk.type)} on ${esc(s.configDisk.device)})</span>`
        :`<span class="wizard-status-badge badge-warn">Temporary (tmpfs) -- will not survive reboot</span>`}
    </div>
    <div class="wizard-status-row">
      <span class="wizard-status-label">Cache Disk</span>
      ${cachePersist
        ?`<span class="wizard-status-badge badge-ok">Persistent (${esc(s.cacheDisk.type)} on ${esc(s.cacheDisk.device)})</span>`
        :`<span class="wizard-status-badge badge-warn">Temporary (tmpfs) -- will not survive reboot</span>`}
    </div>
  </div>
  ${cfgPersist&&cachePersist?`<div class="wizard-info-box"><strong>Both disks are already persistent.</strong> No disk setup needed. You can close this wizard or continue to configure network and timezone.</div>`:''}
  ${!cfgPersist||!cachePersist?`<div class="wizard-warn-box">Data on temporary storage will be lost when the system reboots. Set up persistent disks to keep your configuration and Docker data.</div>`:''}
</div>`;

  footer.innerHTML=`
<div class="wizard-footer-left">
  <button class="btn btn-secondary btn-sm" onclick="setupWizard.dismiss()">Don't show again</button>
</div>
<div class="wizard-footer-right">
  <button class="btn btn-primary" onclick="wizardNext()">Continue</button>
</div>`;
}

// ============ STEP 2: DISK DETECTION ============

function renderStepDisks(body,footer){
  const s=wizState.status;
  const disks=s.availableDisks||[];

  // Auto-assign disks by size: smallest -> config, largest -> cache.
  // Always prefer size-based assignment over label-based, because labels
  // from a previous setup may be swapped (e.g. user reformatted wrong).
  if(Object.keys(wizState.assignments).length===0){
    if(disks.length>=2){
      const sorted=[...disks].sort((a,b)=>a.sizeBytes-b.sizeBytes);
      wizState.assignments[sorted[0].device]='config';
      wizState.assignments[sorted[sorted.length-1].device]='cache';
    }else if(disks.length===1){
      // Single disk: default to config (more critical for persistence)
      wizState.assignments[disks[0].device]='config';
    }
  }

  body.innerHTML=`
<div class="wizard-step-content">
  <h2>Disk Detection & Assignment</h2>
  <p class="dim" style="margin-bottom:16px">Assign available disks to their roles. Each disk can only have one role.</p>
  <p class="dim" style="margin-bottom:24px;font-size:.8rem">Config disk only needs ~100MB for configuration files. Cache disk stores Docker images and container data.</p>
  <div id="wizard-disk-list">
    ${disks.length===0
      ?`<div class="wizard-no-disks">
          <p>No available disks detected.</p>
          <p class="dim" style="margin-top:8px;font-size:.85rem">Attach virtual disks via your hypervisor (XCP-ng, QEMU, etc.) then click Rescan.</p>
        </div>`
      :disks.map(d=>renderDiskCard(d)).join('')}
  </div>
  <div style="margin-top:16px">
    <button class="btn btn-secondary btn-sm" onclick="wizardRescan()" id="btn-rescan">Rescan Disks</button>
  </div>
</div>`;

  footer.innerHTML=`
<div class="wizard-footer-left">
  <button class="btn btn-secondary" onclick="wizardPrev()">Back</button>
</div>
<div class="wizard-footer-right">
  <button class="btn btn-primary" onclick="wizardNext()" id="btn-disk-next">Continue</button>
</div>`;
}

function renderDiskCard(d){
  const assignment=wizState.assignments[d.device]||'';
  const isWraithLabeled=d.label==='WRAITH-CONFIG'||d.label==='WRAITH-CACHE';
  const hasExistingFS=d.fstype&&!isWraithLabeled;
  const hasData=d.hasData||hasExistingFS;

  let statusIcon='';
  let statusNote='';
  if(isWraithLabeled){
    statusIcon='<span class="wizard-disk-icon disk-ok"></span>';
    statusNote=`<div class="wizard-disk-note note-ok">Already formatted as ${esc(d.label)} -- will mount without reformatting.</div>`;
  }else if(d.fstype){
    if(d.fstype==='ext4'){
      statusIcon='<span class="wizard-disk-icon disk-warn"></span>';
      statusNote=`<div class="wizard-disk-note note-warn">This disk has an existing ext4 filesystem. It may contain data from another use.</div>`;
    }else{
      statusIcon='<span class="wizard-disk-icon disk-warn"></span>';
      statusNote=`<div class="wizard-disk-note note-warn">This disk has a ${esc(d.fstype)} filesystem and must be formatted as ext4 before use.</div>`;
    }
  }

  return `
<div class="wizard-disk-card ${assignment?'disk-assigned':''}">
  <div class="wizard-disk-info">
    ${statusIcon}
    <div class="wizard-disk-details">
      <div class="wizard-disk-name">${esc(d.device)}</div>
      <div class="wizard-disk-meta">
        <span>${fmtBytes(d.sizeBytes)}</span>
        ${d.fstype?`<span class="mono">${esc(d.fstype)}</span>`:'<span class="dim">Unformatted</span>'}
        ${d.label?`<span class="mono">${esc(d.label)}</span>`:''}
      </div>
    </div>
  </div>
  ${statusNote}
  <div class="wizard-disk-roles">
    <label class="wizard-role-btn ${assignment==='config'?'role-active':''}">
      <input type="radio" name="role-${esc(d.device)}" value="config" ${assignment==='config'?'checked':''} onchange="wizardAssignDisk('${esc(d.device)}','config')">
      Config
    </label>
    <label class="wizard-role-btn ${assignment==='cache'?'role-active':''}">
      <input type="radio" name="role-${esc(d.device)}" value="cache" ${assignment==='cache'?'checked':''} onchange="wizardAssignDisk('${esc(d.device)}','cache')">
      Cache
    </label>
    <label class="wizard-role-btn ${assignment===''?'role-active':''}">
      <input type="radio" name="role-${esc(d.device)}" value="" ${assignment===''?'checked':''} onchange="wizardAssignDisk('${esc(d.device)}','')">
      Skip
    </label>
  </div>
</div>`;
}

window.wizardAssignDisk=function(device,role){
  // Clear any other disk with same role (except skip)
  if(role){
    for(const[dev,r]of Object.entries(wizState.assignments)){
      if(r===role&&dev!==device)wizState.assignments[dev]='';
    }
  }
  wizState.assignments[device]=role;
  // Re-render disk list to update radio states
  renderStepDisks($('#wizard-body'),$('#wizard-footer'));
};

window.wizardRescan=async function(){
  const btn=$('#btn-rescan');
  if(btn){btn.disabled=true;btn.innerHTML='<span class="spinner"></span> Scanning...';}
  try{
    const r=await api('/setup/rescan',{method:'POST'});
    wizState.status.availableDisks=r.availableDisks||[];
    toast('Disk scan complete','success');
  }catch(e){
    toast(`Rescan failed: ${e.message}`,'error');
  }
  renderStepDisks($('#wizard-body'),$('#wizard-footer'));
};

// ============ STEP 3: CONFIRM & FORMAT ============

function renderStepConfirm(body,footer){
  const s=wizState.status;
  const disks=s.availableDisks||[];
  const configDev=findAssigned('config');
  const cacheDev=findAssigned('cache');

  if(!configDev&&!cacheDev){
    body.innerHTML=`
<div class="wizard-step-content">
  <h2>No Disks Selected</h2>
  <p class="dim" style="margin-bottom:16px">You have not assigned any disks. Go back to select disks, or skip to continue with temporary storage.</p>
  <div class="wizard-warn-box">Without persistent disks, all data will be lost on reboot.</div>
</div>`;

    footer.innerHTML=`
<div class="wizard-footer-left">
  <button class="btn btn-secondary" onclick="wizardPrev()">Back</button>
</div>
<div class="wizard-footer-right">
  <button class="btn btn-primary" onclick="wizardNext()">Skip Disk Setup</button>
</div>`;
    return;
  }

  const configDisk=configDev?disks.find(d=>d.device===configDev):null;
  const cacheDisk=cacheDev?disks.find(d=>d.device===cacheDev):null;
  const configIsWraith=configDisk&&configDisk.label==='WRAITH-CONFIG';
  const cacheIsWraith=cacheDisk&&cacheDisk.label==='WRAITH-CACHE';
  const configHasData=configDisk&&configDisk.fstype&&!configIsWraith;
  const cacheHasData=cacheDisk&&cacheDisk.fstype&&!cacheIsWraith;
  const needsFormat=(!configIsWraith&&configDisk)||(!cacheIsWraith&&cacheDisk);
  const hasExistingData=configHasData||cacheHasData;

  let actions='';
  if(configDisk){
    if(configIsWraith){
      actions+=`<div class="wizard-action-row action-ok">Mount existing WRAITH-CONFIG disk (${esc(configDev)}) at /wraith/config -- no formatting needed</div>`;
    }else{
      actions+=`<div class="wizard-action-row action-destructive">Format ${esc(configDev)} (${fmtBytes(configDisk.sizeBytes)}) as ext4 with label WRAITH-CONFIG</div>`;
    }
  }
  if(cacheDisk){
    if(cacheIsWraith){
      actions+=`<div class="wizard-action-row action-ok">Mount existing WRAITH-CACHE disk (${esc(cacheDev)}) at /wraith/cache -- no formatting needed</div>`;
    }else{
      actions+=`<div class="wizard-action-row action-destructive">Format ${esc(cacheDev)} (${fmtBytes(cacheDisk.sizeBytes)}) as ext4 with label WRAITH-CACHE</div>`;
      actions+=`<div class="wizard-action-row action-warn">Docker will be temporarily stopped while the cache disk is formatted. Running containers will be interrupted.</div>`;
    }
  }

  body.innerHTML=`
<div class="wizard-step-content">
  <h2>Confirm & Format</h2>
  <div class="wizard-actions-list">${actions}</div>
  ${needsFormat?`
  <div class="wizard-danger-box">
    <strong>WARNING:</strong> Formatting will permanently erase all data on the selected disk(s). This cannot be undone.
  </div>
  ${hasExistingData?`
  <div class="form-group" style="margin-top:16px">
    <label class="form-label">Type FORMAT to confirm (disk has existing data)</label>
    <input class="form-input" id="format-confirm-text" placeholder="Type FORMAT to proceed" autocomplete="off" style="max-width:300px">
  </div>
  `:`
  <div class="wizard-checkbox-row" style="margin-top:16px">
    <label class="wizard-check-label">
      <input type="checkbox" id="format-confirm-check">
      <span>I understand that formatting will erase all data on the selected disk(s)</span>
    </label>
  </div>
  `}
  `:''}
  <div id="format-progress" class="hidden">
    <div class="wizard-progress">
      <div class="wizard-progress-bar" id="format-progress-bar"></div>
    </div>
    <div class="wizard-progress-text" id="format-progress-text">Preparing...</div>
  </div>
</div>`;

  footer.innerHTML=`
<div class="wizard-footer-left">
  <button class="btn btn-secondary" onclick="wizardPrev()" id="btn-format-back">Back</button>
</div>
<div class="wizard-footer-right">
  ${needsFormat
    ?`<button class="btn btn-danger" onclick="wizardFormat()" id="btn-format">Format & Mount</button>`
    :`<button class="btn btn-primary" onclick="wizardFormat()" id="btn-format">Mount Disks</button>`}
</div>`;
}

window.wizardFormat=async function(){
  const s=wizState.status;
  const disks=s.availableDisks||[];
  const configDev=findAssigned('config');
  const cacheDev=findAssigned('cache');
  const configDisk=configDev?disks.find(d=>d.device===configDev):null;
  const cacheDisk=cacheDev?disks.find(d=>d.device===cacheDev):null;
  const configIsWraith=configDisk&&configDisk.label==='WRAITH-CONFIG';
  const cacheIsWraith=cacheDisk&&cacheDisk.label==='WRAITH-CACHE';
  const needsFormat=(!configIsWraith&&configDisk)||(!cacheIsWraith&&cacheDisk);
  const configHasData=configDisk&&configDisk.fstype&&!configIsWraith;
  const cacheHasData=cacheDisk&&cacheDisk.fstype&&!cacheIsWraith;
  const hasExistingData=configHasData||cacheHasData;

  // Validation
  if(needsFormat){
    if(hasExistingData){
      const txt=$('#format-confirm-text');
      if(!txt||txt.value!=='FORMAT'){
        toast('Type FORMAT to confirm (disk has existing data)','error');
        return;
      }
    }else{
      const chk=$('#format-confirm-check');
      if(!chk||!chk.checked){
        toast('Confirm that you understand formatting will erase data','error');
        return;
      }
    }
  }

  // Disable buttons, show progress
  const btn=$('#btn-format');
  const backBtn=$('#btn-format-back');
  if(btn){btn.disabled=true;btn.innerHTML='<span class="spinner"></span> Working...';}
  if(backBtn)backBtn.disabled=true;

  const progress=$('#format-progress');
  const bar=$('#format-progress-bar');
  const txt=$('#format-progress-text');
  if(progress)progress.classList.remove('hidden');

  // Animate progress
  const steps=['Preparing...','Formatting disks...','Mounting...','Initializing layout...','Migrating data...','Complete'];
  let pctStep=0;
  const progressInterval=setInterval(()=>{
    pctStep++;
    if(pctStep>=steps.length-1){clearInterval(progressInterval);return}
    if(bar)bar.style.width=(pctStep/(steps.length-1)*100)+'%';
    if(txt)txt.textContent=steps[pctStep];
  },1500);

  try{
    const result=await api('/setup/disks',{method:'POST',body:{
      configDisk:configDev||'',
      cacheDisk:cacheDev||'',
      confirmFormat:true
    }});
    clearInterval(progressInterval);
    if(bar)bar.style.width='100%';
    if(txt)txt.textContent='Complete';
    wizState.formatResult=result;
    toast('Disk setup complete','success');
    // Refresh status
    try{wizState.status=await api('/setup/status')}catch{}
    setTimeout(()=>wizardNext(),800);
  }catch(e){
    clearInterval(progressInterval);
    if(bar){bar.style.width='100%';bar.classList.add('progress-error');}
    if(txt)txt.textContent='Error: '+e.message;
    toast(`Disk setup failed: ${e.message}`,'error');
    if(btn){btn.disabled=false;btn.textContent='Retry';}
    if(backBtn)backBtn.disabled=false;
  }
};

// ============ STEP 4: NETWORK ============

async function renderStepNetwork(body,footer){
  body.innerHTML=`
<div class="wizard-step-content">
  <h2>Network Configuration</h2>
  <p class="dim" style="margin-bottom:24px">Configure your network settings, or skip to keep the current configuration.</p>
  <div id="wizard-net-loading"><div class="skeleton skeleton-card"></div></div>
  <div id="wizard-net-form" class="hidden"></div>
</div>`;

  footer.innerHTML=`
<div class="wizard-footer-left">
  <button class="btn btn-secondary" onclick="wizardPrev()">Back</button>
</div>
<div class="wizard-footer-right">
  <button class="btn btn-secondary" onclick="wizardNext()" style="margin-right:8px">Skip</button>
  <button class="btn btn-primary" onclick="wizardSaveNetwork()" id="btn-save-net">Save & Continue</button>
</div>`;

  try{
    const d=await api('/network');
    const n=d.network||d;
    wizState.networkData=n;
    const formEl=$('#wizard-net-form');
    const loadEl=$('#wizard-net-loading');
    if(loadEl)loadEl.classList.add('hidden');
    if(formEl){
      formEl.classList.remove('hidden');
      formEl.innerHTML=`
<div class="toggle-wrap" style="margin-bottom:24px">
  <label class="toggle">
    <input type="checkbox" id="wiz-net-dhcp" ${n.dhcp!==false?'checked':''} onchange="wizTogDHCP(this.checked)">
    <span class="toggle-track"></span><span class="toggle-thumb"></span>
  </label>
  <span class="toggle-label">Use DHCP</span>
</div>
<div id="wiz-static-fields" ${n.dhcp!==false?'style="opacity:0.3;pointer-events:none"':''}>
  <div class="form-grid">
    <div class="form-group"><label class="form-label">IP Address</label>
      <input class="form-input" id="wiz-net-ip" placeholder="192.168.1.100" value="${esc(n.ip||'')}"></div>
    <div class="form-group"><label class="form-label">Subnet Mask</label>
      <input class="form-input" id="wiz-net-mask" placeholder="255.255.255.0" value="${esc(n.mask||n.subnet||'255.255.255.0')}"></div>
    <div class="form-group"><label class="form-label">Gateway</label>
      <input class="form-input" id="wiz-net-gw" placeholder="192.168.1.1" value="${esc(n.gateway||'')}"></div>
    <div class="form-group"><label class="form-label">DNS Servers</label>
      <input class="form-input" id="wiz-net-dns" placeholder="8.8.8.8, 1.1.1.1" value="${esc((n.dns||[]).join(', '))}">
      <div class="form-hint">Comma-separated</div></div>
  </div>
</div>`;
    }
  }catch(e){
    const loadEl=$('#wizard-net-loading');
    if(loadEl)loadEl.innerHTML=`<p class="dim">Could not load network settings: ${esc(e.message)}</p>`;
  }
}

window.wizTogDHCP=function(on){
  const f=$('#wiz-static-fields');
  if(f){f.style.opacity=on?'0.3':'1';f.style.pointerEvents=on?'none':'auto'}
};

window.wizardSaveNetwork=async function(){
  const dhcp=$('#wiz-net-dhcp').checked;
  const body={dhcp};
  if(!dhcp){
    body.ip=$('#wiz-net-ip').value;
    body.mask=$('#wiz-net-mask').value;
    body.gateway=$('#wiz-net-gw').value;
    body.dns=$('#wiz-net-dns').value.split(',').map(s=>s.trim()).filter(Boolean);
  }
  const btn=$('#btn-save-net');
  if(btn){btn.disabled=true;btn.innerHTML='<span class="spinner"></span> Saving...';}
  try{
    await api('/network',{method:'PUT',body});
    wizState.networkData=body;
    toast('Network settings saved','success');
    setTimeout(()=>wizardNext(),500);
  }catch(e){
    toast(`Network error: ${e.message}`,'error');
    if(btn){btn.disabled=false;btn.textContent='Save & Continue';}
  }
};

// ============ STEP 5: TIMEZONE ============

async function renderStepTimezone(body,footer){
  body.innerHTML=`
<div class="wizard-step-content">
  <h2>Timezone</h2>
  <p class="dim" style="margin-bottom:24px">Select your timezone, or skip to keep UTC.</p>
  <div id="wizard-tz-loading"><div class="skeleton skeleton-text"></div></div>
  <div id="wizard-tz-form" class="hidden"></div>
</div>`;

  footer.innerHTML=`
<div class="wizard-footer-left">
  <button class="btn btn-secondary" onclick="wizardPrev()">Back</button>
</div>
<div class="wizard-footer-right">
  <button class="btn btn-secondary" onclick="wizardNext()" style="margin-right:8px">Skip</button>
  <button class="btn btn-primary" onclick="wizardSaveTimezone()" id="btn-save-tz">Save & Continue</button>
</div>`;

  try{
    const d=await api('/system/timezone');
    const loadEl=$('#wizard-tz-loading');
    const formEl=$('#wizard-tz-form');
    if(loadEl)loadEl.classList.add('hidden');
    if(formEl){
      formEl.classList.remove('hidden');
      const tzList=d.available||[];
      const current=d.timezone||'UTC';
      wizState.timezone=current;
      formEl.innerHTML=`
<div class="form-group" style="max-width:400px">
  <label class="form-label">System Timezone</label>
  <select class="form-input" id="wiz-tz-select">
    ${tzList.map(tz=>`<option value="${esc(tz)}" ${tz===current?'selected':''}>${esc(tz)}</option>`).join('')}
  </select>
  <div class="form-hint">Current: ${esc(current)}</div>
</div>`;
    }
  }catch(e){
    const loadEl=$('#wizard-tz-loading');
    if(loadEl)loadEl.innerHTML=`<p class="dim">Could not load timezone data: ${esc(e.message)}</p>`;
  }
}

window.wizardSaveTimezone=async function(){
  const sel=$('#wiz-tz-select');
  if(!sel)return;
  const tz=sel.value;
  const btn=$('#btn-save-tz');
  if(btn){btn.disabled=true;btn.innerHTML='<span class="spinner"></span> Saving...';}
  try{
    await api('/system/timezone',{method:'PUT',body:{timezone:tz}});
    wizState.timezone=tz;
    toast('Timezone updated','success');
    setTimeout(()=>wizardNext(),500);
  }catch(e){
    toast(`Timezone error: ${e.message}`,'error');
    if(btn){btn.disabled=false;btn.textContent='Save & Continue';}
  }
};

// ============ STEP 6: SUMMARY ============

function renderStepSummary(body,footer){
  const r=wizState.formatResult;
  const s=wizState.status;
  const cfgPersist=s.configDisk&&s.configDisk.persistent;
  const cachePersist=s.cacheDisk&&s.cacheDisk.persistent;
  const disksFormatted=r&&r.status==='complete';
  const hotRemountOk=r&&r.hotRemountSuccess;
  const rebootRec=r&&r.rebootRecommended&&!hotRemountOk;

  let summaryItems=[];
  if(r){
    if(r.configDisk&&r.configDisk.mounted)summaryItems.push(`Config disk (${esc(r.configDisk.device)}): ${r.configDisk.action}`);
    if(r.cacheDisk&&r.cacheDisk.mounted)summaryItems.push(`Cache disk (${esc(r.cacheDisk.device)}): ${r.cacheDisk.action}`);
    if(r.migratedFiles&&r.migratedFiles.length)summaryItems.push(`Migrated files: ${r.migratedFiles.join(', ')}`);
  }
  if(wizState.networkData){
    summaryItems.push(`Network: ${wizState.networkData.dhcp?'DHCP':'Static IP'}`);
  }
  if(wizState.timezone!=='UTC'){
    summaryItems.push(`Timezone: ${wizState.timezone}`);
  }

  body.innerHTML=`
<div class="wizard-step-content">
  <h2>Setup Complete</h2>
  ${summaryItems.length?`
  <div class="wizard-summary-list">
    ${summaryItems.map(item=>`<div class="wizard-summary-item">${checkSVG()} ${item}</div>`).join('')}
  </div>`:'<p class="dim" style="margin-bottom:16px">No changes were made.</p>'}
  ${rebootRec?`
  <div class="wizard-warn-box" style="margin-top:24px">
    <strong>Reboot recommended.</strong> A reboot ensures all services start with persistent storage.
  </div>`:''}
  ${hotRemountOk?`
  <div class="wizard-info-box" style="margin-top:24px">
    Hot remount succeeded. Reboot is optional but recommended for full service restart.
  </div>`:''}
</div>`;

  footer.innerHTML=`
<div class="wizard-footer-left">
  ${disksFormatted?`<button class="btn btn-secondary" onclick="setupWizard.close()">Continue Without Reboot</button>`
    :`<button class="btn btn-secondary" onclick="setupWizard.close()">Close</button>`}
</div>
<div class="wizard-footer-right">
  ${disksFormatted?`<button class="btn btn-danger" onclick="wizardReboot()">Reboot Now</button>`:''}
</div>`;
}

window.wizardReboot=async function(){
  if(!confirm('Reboot the system now?'))return;
  try{
    await api('/system/reboot',{method:'POST'});
    toast('Rebooting...','info');
    setupWizard.close();
    // Show a rebooting overlay
    const overlay=document.createElement('div');
    overlay.className='wizard-overlay wizard-visible';
    overlay.innerHTML=`<div class="wizard-card" style="text-align:center;padding:48px">
      <div class="spinner" style="width:32px;height:32px;border-width:3px;margin:0 auto 16px"></div>
      <h2>Rebooting...</h2>
      <p class="dim" style="margin-top:8px">The system is restarting. This page will reload automatically.</p>
    </div>`;
    document.body.appendChild(overlay);
    // Poll for reconnection
    const pollReboot=setInterval(async()=>{
      try{const r=await fetch('/api/auth/status');if(r.ok){clearInterval(pollReboot);location.reload()}}catch{}
    },3000);
  }catch(e){
    toast(`Reboot failed: ${e.message}`,'error');
  }
};

// ============ NAVIGATION ============

window.wizardNext=function(){
  if(wizState.step<wizState.totalSteps){
    wizState.step++;
    renderStep();
  }
};

window.wizardPrev=function(){
  if(wizState.step>1){
    wizState.step--;
    renderStep();
  }
};

// ============ HELPERS ============

function findAssigned(role){
  for(const[dev,r]of Object.entries(wizState.assignments)){
    if(r===role)return dev;
  }
  return '';
}

function fmtBytes(b){
  if(!b||b<0)return '0 B';
  if(b<1024)return b+' B';
  if(b<1048576)return(b/1024).toFixed(1)+' KB';
  if(b<1073741824)return(b/1048576).toFixed(1)+' MB';
  return(b/1073741824).toFixed(1)+' GB';
}

function esc(s){if(!s)return'';const d=document.createElement('div');d.textContent=s;return d.innerHTML}

})();
