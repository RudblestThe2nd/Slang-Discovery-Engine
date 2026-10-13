'use strict';
const $=id=>document.getElementById(id);
const ui={state:null,selected:null,view:'overview',refreshing:false,poll:null};
const token=window.APP_TOKEN;
const esc=s=>String(s??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const prettyKind={unusual_token:'Sıra dışı kelime',phrase_candidate:'İfade adayı',context_drift:'Bağlam değişimi'};
const prettyStatus={pending:'Bekliyor',accepted:'Onaylandı',rejected:'Reddedildi',needs_review:'Tekrar incele'};
async function request(url,opts={}){
 const res=await fetch(url,{cache:'no-store',...opts});
 let data;try{data=await res.json()}catch(e){throw new Error('Sunucu yanıtı okunamadı')}
 if(!res.ok)throw new Error(data.error||'İstek başarısız');return data;
}
async function post(url,content){return request(url,{method:'POST',headers:{'Content-Type':'application/json','X-App-Token':token},body:JSON.stringify(content)});}
function switchView(view){ui.view=view;document.querySelectorAll('.nav').forEach(n=>n.classList.toggle('active',n.dataset.view===view));document.querySelectorAll('.view').forEach(n=>n.classList.toggle('active',n.id==='view-'+view));if(view==='discover')renderCandidates();}
function notice(text,type=''){const n=$('job-notice');n.textContent=text;n.className='notice '+type;}
function status(text){$('footer-status').textContent=text;$('side-status').textContent=text;}
function loadSettings(){
 const s=ui.state.settings;
 $('client-id').value=s.client_id||'';$('client-secret').value='';$('clear-secret').checked=false;
 $('secret-state').textContent=ui.state.has_secret?'(ŞİFRELİ OLARAK KAYDEDİLDİ)':'(HENÜZ YOK)';
 $('user-agent').value=s.user_agent||'';$('subreddits').value=s.subreddits||'';
 $('posts-per-sub').value=s.posts_per_sub;$('comments-per-post').value=s.comments_per_post;
 $('retention-days').value=s.retention_days;
 $('stat-api').textContent=(s.client_id&&ui.state.has_secret)?'AYARLI':'KAPALI';
}
function renderCandidates(){
 if(!ui.state)return;
 const term=$('search').value.trim().toLowerCase(),kind=$('kind-filter').value,st=$('status-filter').value;
 const filtered=(ui.state.candidates||[]).filter(c=>(kind==='all'||c.kind===kind)&&(st==='all'||c.status===st)&&c.term.includes(term));
 $('candidate-count').textContent=filtered.length+' aday görüntüleniyor';
 $('candidate-body').innerHTML=filtered.length?filtered.map(c=>`<tr><td class="term">${esc(c.term)}</td><td>${esc(prettyKind[c.kind]||c.kind)}</td><td class="score">${Number(c.score).toFixed(1)}</td><td>${Number(c.mentions)}</td><td>${Number(c.authors)}</td><td><span class="tag ${esc(c.status)}">${esc(prettyStatus[c.status]||c.status)}</span></td><td><button class="btn mini-btn inspect" data-term="${esc(c.term)}" data-kind="${esc(c.kind)}">İNCELE →</button></td></tr>`).join(''):'<tr><td colspan="7" class="empty-row">Bu filtre için bir aday bulunamadı.</td></tr>';
 document.querySelectorAll('button.inspect').forEach(b=>b.addEventListener('click',()=>inspect(b.dataset.term,b.dataset.kind)));
}
async function inspect(term,kind){
 const c=(ui.state.candidates||[]).find(x=>x.term===term&&x.kind===kind);
 if(!c)return;ui.selected=c;
 switchView('evidence');$('no-selection').hidden=true;$('selection-content').hidden=false;
 $('selected-chip').textContent=c.term+' • '+(prettyKind[c.kind]||c.kind);
 $('review-status').value=c.status||'needs_review';$('review-note').value=c.note||'';
 $('evidence-list').textContent='Kaynak örnekleri alınıyor...';$('trend-chart').textContent='Trend hesaplanıyor...';
 try{
  const [records,trend]=await Promise.all([request('/api/evidence?term='+encodeURIComponent(term)+'&kind='+encodeURIComponent(kind)),request('/api/trend?term='+encodeURIComponent(term)+'&kind='+encodeURIComponent(kind))]);
  $('evidence-list').innerHTML=records.length?records.map(d=>`<div class="evidence-card"><div class="evidence-meta">${esc(d.source.toUpperCase())} / ${esc(d.subreddit)} · ${new Date(d.created*1000).toLocaleDateString('tr-TR')}</div><div class="evidence-body">${esc(d.body)}</div>${d.source==='reddit'?`<a class="evidence-link" href="${esc(d.url)}" target="_blank" rel="noopener noreferrer">REDDIT KAYNAĞINI AÇ →</a>`:'<span class="smallnote">SENTETİK DEMO VERİSİ</span>'}</div>`).join(''):'<div class="empty">Kanıt bulunamadı.</div>';
  const relevant=trend.filter(x=>x.mentions>0);const display=relevant.length?trend.slice(Math.max(0,trend.findIndex(x=>x.mentions>0)-1)).slice(-12):trend.slice(-12);
  const max=Math.max(1,...display.map(x=>x.per_1000_docs));
  $('trend-chart').innerHTML=display.length?display.map(x=>`<div class="trend-col" title="${esc(x.week)}: ${Number(x.per_1000_docs).toFixed(1)} /1000"><div class="trend-bar" style="--bar-height:${Math.max(2,Math.round(x.per_1000_docs/max*110))}px"></div><div class="trend-lbl">${esc(x.week.slice(5))}</div></div>`).join(''):'<div class="smallnote">Henüz zaman serisi yok.</div>';
 }catch(e){$('evidence-list').textContent=e.message;}
}
async function refresh(){
 if(ui.refreshing)return;ui.refreshing=true;
 try{
  const state=await request('/api/state');const prior=ui.state;
  ui.state=state;
  $('stat-docs').textContent=state.documents;$('stat-cands').textContent=(state.candidates||[]).length;
  if(!prior||JSON.stringify(prior.settings)!==JSON.stringify(state.settings)||prior.has_secret!==state.has_secret)loadSettings();
  if(ui.view==='discover')renderCandidates();
  if(state.running){status('İŞLEM SÜRÜYOR');$('side-light').classList.add('wait');notice('İşlem devam ediyor: '+state.job,'');}
  else{
   $('side-light').classList.remove('wait');status(state.job_state==='error'?'HATA / AYARLARI KONTROL ET':'SİSTEM HAZIR');
   if(state.job_state==='error')notice(state.last_error,'bad');
   else if(state.last_result)notice(state.last_result,'ok');
  }
  document.querySelectorAll('button[data-job]').forEach(b=>b.disabled=state.running);
 }catch(e){status('SUNUCU HATASI');notice(e.message,'bad')}finally{ui.refreshing=false}
}
async function runJob(action){
 try{await post('/api/job',{action});notice('İşlem başlatıldı: '+action);await refresh();}catch(e){alert(e.message);}
}
async function saveSettings(){
 const data={client_id:$('client-id').value.trim(),client_secret:$('client-secret').value,clear_secret:$('clear-secret').checked,user_agent:$('user-agent').value.trim(),subreddits:$('subreddits').value.trim(),posts_per_sub:Number($('posts-per-sub').value),comments_per_post:Number($('comments-per-post').value),retention_days:Number($('retention-days').value)};
 try{await post('/api/settings',data);$('client-secret').value='';await refresh();alert('Ayarlar güvenli şekilde kaydedildi.');}catch(e){alert('Ayarlar kaydedilemedi: '+e.message);}
}
async function saveReview(){if(!ui.selected)return;
 try{await post('/api/review',{term:ui.selected.term,kind:ui.selected.kind,status:$('review-status').value,note:$('review-note').value});await refresh();ui.selected=ui.state.candidates.find(x=>x.term===ui.selected.term&&x.kind===ui.selected.kind);alert('Değerlendirme kaydedildi.');}catch(e){alert(e.message)}
}
function showInfo(open){$('info-dialog').hidden=!open;if(open)$('close-info').focus();}
function init(){
 $('date-now').textContent=new Date().toLocaleDateString('tr-TR',{day:'2-digit',month:'long',year:'numeric'}).toUpperCase();
 document.querySelectorAll('[data-view]').forEach(b=>b.addEventListener('click',()=>switchView(b.dataset.view)));
 document.querySelectorAll('[data-job]').forEach(b=>b.addEventListener('click',()=>runJob(b.dataset.job)));
 for(const f of ['search','kind-filter','status-filter'])$(f).addEventListener(f==='search'?'input':'change',renderCandidates);
 $('save-settings').addEventListener('click',saveSettings);
 $('toggle-secret').addEventListener('click',()=>{$('client-secret').type=$('client-secret').type==='password'?'text':'password';});
 $('save-review').addEventListener('click',saveReview);
 $('open-info').addEventListener('click',()=>showInfo(true));$('close-info').addEventListener('click',()=>showInfo(false));$('got-info').addEventListener('click',()=>showInfo(false));
 $('info-dialog').addEventListener('click',e=>{if(e.target===$('info-dialog'))showInfo(false)});
 window.addEventListener('keydown',e=>{if(e.key==='Escape')showInfo(false)});
 $('import-btn').addEventListener('click',()=>$('import-file').click());
 $('import-file').addEventListener('change',async e=>{
   const file=e.target.files?.[0];if(!file)return;
   if(file.size>850000){alert('JSONL dosyası en fazla 850 KB olmalıdır.');e.target.value='';return;}
   try{const result=await post('/api/import',{lines:await file.text()});alert(result.imported+' belge alındı, '+result.candidates+' aday hesaplandı.');await refresh();}
   catch(error){alert('İçe aktarma hatası: '+error.message)}finally{e.target.value='';}
 });
 $('exit-app').addEventListener('click',async()=>{
   if(!confirm('Slang Discovery Engine kapatılsın mı?'))return;
   try{await post('/api/shutdown',{});document.body.innerHTML='<main style="font:16px Tahoma;padding:60px;text-align:center">Slang Discovery Engine kapatıldı. Bu sekmeyi kapatabilirsiniz.</main>';}
   catch(e){alert('Kapatma sırasında hata: '+e.message)}
 });
 $('export-btn').addEventListener('click',()=>{window.location.href='/api/export.csv';});
 refresh();ui.poll=setInterval(refresh,2500);
}
init();
