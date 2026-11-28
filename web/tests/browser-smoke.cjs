// Run with PLAYWRIGHT_MODULE pointing to a local Playwright installation.
const {chromium,firefox}=require(process.env.PLAYWRIGHT_MODULE||'playwright');
const assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const http=require('node:http');
const root=path.resolve(__dirname,'../dist');
const count=100000,tracks=Array.from({length:count},(_,i)=>({id:i.toString(16).padStart(32,'0'),source:'music',name:'song'+i+'.pt3',path:'ZX/deep/song'+i+'.pt3',mode:'ay',format:'pt3',size:300}));
let lists=[{name:'Test',ids:[]}],uploads=[];
const json=(res,x)=>{res.setHeader('content-type','application/json');res.end(JSON.stringify(x))};
const server=http.createServer((req,res)=>{
 const u=new URL(req.url,'http://localhost');
 if(u.pathname==='/api/library'||u.pathname==='/api/rescan')return json(res,tracks);
 if(u.pathname==='/api/upload'||u.pathname==='/api/import-zip'){let chunks=[];req.on('data',x=>chunks.push(x));req.on('end',()=>{uploads.push({path:u.pathname,dir:u.searchParams.get('dir'),source:u.searchParams.get('source'),body:Buffer.concat(chunks).toString()});json(res,u.pathname.endsWith('zip')?{tracks:2,archives:1,duplicates:0,skipped:0}:tracks[0])});return}
 if(u.pathname==='/api/me')return json(res,{role:'admin',name:'test'});
 if(u.pathname==='/api/admin/folder')return json(res,{writable:true,folders:1,tracks:0,entries:[{name:'ZX',path:'ZX',directory:true,music:false,indexed:false}]});
 if(u.pathname==='/api/playlists')return json(res,lists);
 if(u.pathname==='/api/admin/skins')return json(res,[{id:'a',name:'Classic',size:1024,visible:true,default:true},{id:'b',name:'Blue',size:2048,visible:false,default:false}]);
 if(u.pathname==='/api/personal-playlists')return json(res,[{name:'Test',ids:[tracks[1].id]}]);
 if(u.pathname==='/api/playlists/append'){let data='';req.on('data',x=>data+=x);req.on('end',()=>{const p=JSON.parse(data),list=lists[0],before=list.ids.length;list.ids=[...new Set([...list.ids,...p.ids])];json(res,{added:list.ids.length-before,skipped:p.ids.length-list.ids.length+before,unavailable:0})});return}
 if(u.pathname.startsWith('/api/'))return json(res,[]);
 let file=path.join(root,['/','/admin','/playlists','/library','/radio','/favorites','/recent'].includes(u.pathname)?'index.html':u.pathname);
 if(!fs.existsSync(file)){res.statusCode=404;return res.end()}
 res.setHeader('content-type',({'.js':'text/javascript','.css':'text/css','.wasm':'application/wasm','.html':'text/html'})[path.extname(file)]||'application/octet-stream');fs.createReadStream(file).pipe(res);
});
(async()=>{await new Promise(r=>server.listen(0,'127.0.0.1',r));let browser;
 try{
 browser=process.env.BROWSER==='firefox'?await firefox.launch({headless:true,executablePath:process.env.FIREFOX_BIN||undefined}):await chromium.launch({executablePath:process.env.CHROME_BIN||'/usr/bin/google-chrome',headless:true,args:['--no-sandbox']});
 const page=await browser.newPage({viewport:{width:1440,height:900}}),errors=[];
 page.on('pageerror',e=>{errors.push(String(e));console.error(e)});
 const url='http://127.0.0.1:'+server.address().port;
 await page.goto(url+'/admin');await page.locator('#folderContents input[type=checkbox]').first().waitFor();
 await page.waitForFunction(()=>document.querySelector('#libcount').textContent.includes('100000'));
 assert.equal(await page.locator('#uploadZip').count(),0);
 await page.getByRole('button',{name:'Действия: ZX'}).click();await page.locator('#serviceActions').waitFor({state:'visible'});assert(await page.locator('#serviceActions').getByRole('button',{name:'Переименовать'}).isEnabled());await page.keyboard.press('Escape');await page.locator('#serviceActions').waitFor({state:'hidden'});
 await page.locator('#adminLibrary summary').click();await page.locator('#browseRefresh').waitFor({state:'visible'});await page.locator('#browseRefresh').click();await page.locator('#folderContents input[type=checkbox]').first().waitFor();
 await page.setViewportSize({width:390,height:844});assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1));await page.setViewportSize({width:1440,height:900});
 console.log('PASS admin file table, contextual actions and maintenance menu');
 await page.locator('#uploadFiles').setInputFiles([{name:'test.pt3',mimeType:'application/octet-stream',buffer:Buffer.alloc(300)},{name:'archive.ZIP',mimeType:'application/zip',buffer:Buffer.from('PK')}]);
 await page.waitForFunction(()=>document.querySelector('#uploadStatus').textContent.includes('библиотека обновлена'));
 assert.deepEqual(uploads.map(x=>x.path),['/api/upload','/api/import-zip']);assert(uploads.every(x=>x.source==='music'&&x.dir==='.'));assert(uploads[0].body.includes('test.pt3')&&uploads[1].body.includes('archive.ZIP'));
 console.log('PASS mixed upload: ordinary track and ZIP routed to existing endpoints');
 await page.locator('#selectEntries').check();await page.waitForFunction(()=>document.querySelector('#selectedEntries').textContent.includes('100000'));
 await page.locator('#queueEntries').click();await page.waitForFunction(()=>document.querySelector('#queuecount').textContent.includes('100000'));
 await page.locator('#sendEntries').click();await page.locator('#appendTarget').selectOption('Test');await page.locator('#appendCommit').click();await page.locator('#appendDialog').waitFor({state:'hidden',timeout:60000});assert.equal(lists[0].ids.length,count);
 await page.locator('#tabLists').click();await page.locator('#adminLists .catalog-row').first().waitFor();await page.locator('#adminLists').getByRole('button',{name:'Изменить'}).first().click();await page.locator('#editorTracks .catalog-row').first().waitFor();assert((await page.locator('#editorTracks .catalog-row').count())<100);await page.locator('#editorCancel').click();
 await page.locator('#tabSkins').click();await page.locator('#serverSkinGrid .server-skin-card').first().waitFor();assert.equal(await page.locator('.default-skin').count(),1);await page.locator('#adminSkinFilter').selectOption('hidden');await page.waitForFunction(()=>document.querySelectorAll('#serverSkinGrid .server-skin-card').length===1&&document.querySelector('#serverSkinGrid .server-skin-card strong').textContent==='Blue');await page.locator('#serverSkinGrid').getByRole('button',{name:'⋯ Действия'}).click();await page.locator('#serviceActions').getByRole('button',{name:'Показать',exact:true}).waitFor();await page.keyboard.press('Escape');
 console.log('PASS admin editor virtualization, skin filtering and actions');
 await page.locator('#home').click();await page.locator('#homeHero').waitFor({state:'visible'});await page.locator('#libraryPanel').waitFor({state:'hidden'});
 await page.locator('#libraryNav').click();await page.locator('#libraryPanel').waitFor({state:'visible'});await page.locator('#homeHero').waitFor({state:'hidden'});assert(page.url().endsWith('/library'));
 await page.locator('#folderGrid button[title="ZX"]').click();await page.locator('#folderGrid button[title="ZX/deep"]').click();
 await page.waitForFunction(()=>document.querySelector('#folderSummary').textContent.includes('100000'));
 await page.locator('#folderRecursive').uncheck();await page.locator('#folderGrid button[title="ZX"]').click();await page.waitForFunction(()=>document.querySelector('#folderSummary').textContent.startsWith('0 '));
 await page.locator('#folderRecursive').check();await page.locator('#libraryFormat').selectOption('pt3');await page.locator('#search').fill('song99999.pt3');await page.waitForFunction(()=>document.querySelector('#folderSummary').textContent.startsWith('1 '));
 await page.locator('#search').fill('');await page.locator('#librarySort').selectOption('name-desc');await page.waitForFunction(()=>document.querySelector('#library .track').textContent.includes('song99999'));
 assert((await page.locator('#library .track').count())<100);
 await page.locator('#toggleFolders').click();await page.locator('#folderTreeBody').waitFor({state:'hidden'});await page.locator('#toggleFolders').click();
 console.log('PASS library: nested tree, direct/recursive filters, format, search, sort, virtualization and collapse');
 await page.locator('#radioNav').click();await page.locator('#stationSection').waitFor({state:'visible'});await page.locator('#libraryPanel').waitFor({state:'hidden'});
 await page.goBack();await page.locator('#libraryPanel').waitFor({state:'visible'});assert(page.url().endsWith('/library'));
 await page.setViewportSize({width:390,height:844});assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1));await page.setViewportSize({width:1440,height:900});
 console.log('PASS service navigation, history and mobile width');
 await page.locator('#listsNav').click();await page.locator('#listsCatalog .playlist-card').first().waitFor();await page.waitForFunction(()=>document.querySelectorAll('#listsCatalog .playlist-card').length===2);
 assert.equal(await page.locator('#listsCatalog .playlist-cover').count(),2);
 await page.locator('#listsRows').click();assert.equal(await page.locator('#listsCatalog.compact .playlist-card').count(),2);
 await page.locator('#listsCards').click();await page.locator('#listsCatalog .playlist-card').filter({hasText:'Публичный'}).getByRole('button',{name:'Открыть Test'}).click();
 await page.locator('#detailTracks .track').first().waitFor();assert(new URL(page.url()).searchParams.get('scope')==='public');
 await page.locator('#detailSearch').fill('song99999.pt3');await page.waitForFunction(()=>document.querySelector('#detailCount').textContent.startsWith('1 '));assert.equal(await page.locator('#detailTracks .track').count(),1);
 await page.locator('#detailSearch').fill('');assert((await page.locator('#detailTracks .track').count())<100);
 await page.reload();await page.locator('#detailTracks .track').first().waitFor();assert(await page.locator('#listsOverview').isHidden());await page.locator('#listDetails .detail-back').click();
 await page.locator('#listsCatalog .playlist-card[data-scope="personal"]').getByRole('button',{name:'Открыть Test'}).click();await page.waitForFunction(()=>document.querySelector('#detailCount')?.textContent.startsWith('1 '));
 assert(new URL(page.url()).searchParams.get('scope')==='personal');await page.goBack();await page.locator('#listsOverview').waitFor({state:'visible'});
 await page.setViewportSize({width:390,height:844});assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1));await page.setViewportSize({width:1440,height:900});
 console.log('PASS playlists: cards, rows, scoped links, search, reload, history and virtualization');
 await page.locator('#libraryNav').click();await page.locator('#openPlayer').click();
 assert((await page.locator('#queue .track').count())<100);await page.locator('#queue').evaluate(e=>e.scrollTop=e.scrollHeight);
 await page.waitForFunction(()=>[...document.querySelectorAll('#queue .track')].some(e=>e.textContent.includes('song99999')));
 await page.reload();await page.waitForFunction(()=>document.querySelector('#queuecount').textContent.includes('100000'),{},{timeout:30000});
 assert.deepEqual(errors,[]);console.log('PASS browser: recursive selection, append 100000, virtual scroll, reload');
 // Simulate a shuffle-selected track far outside the visible window.
 await page.addInitScript(()=>{if(window.top!==window)return;const s=JSON.parse(localStorage.getItem('runeamp-queue-v1')||'null');if(!s||s.ids.length<54322)return;s.current=s.ids[54321];s.selected=s.ids[0];localStorage.setItem('runeamp-queue-v1',JSON.stringify(s));const p=JSON.parse(localStorage.getItem('runeamp-pref')||'{}');p.shuffle=true;localStorage.setItem('runeamp-pref',JSON.stringify(p))});
 await page.reload();await page.waitForFunction(()=>document.querySelector('#miniNow').textContent==='song54321.pt3');
 const locate=async(selector)=>{
  await page.locator('#queue').evaluate(e=>e.scrollTop=0);
  await page.locator(selector).dblclick();
  await page.waitForFunction(()=>{const q=document.querySelector('#queue'),r=q.querySelector('.track.current.selected');if(!r||r.dataset.index!=='54321')return false;const a=q.getBoundingClientRect(),b=r.getBoundingClientRect();return b.top>=a.top&&b.bottom<=a.bottom});
  assert((await page.locator('#queue .track').count())<100);
 };
 await page.locator('#miniNow').dblclick();
 await page.locator('#playerWindow').waitFor({state:'visible'});
 await locate('#now');
 if(process.env.WSZ_FIXTURE){
  await page.locator('#skinFile').setInputFiles(process.env.WSZ_FIXTURE);
  await page.locator('#wszPanel').waitFor({state:'visible'});
  await page.locator('#wszPlaylist').click();
  await page.locator('#playlistBody').waitFor({state:'hidden'});
  await page.locator('#wszLocateTrack').dblclick();
  await page.locator('#playlistBody').waitFor({state:'visible'});
  await locate('#wszLocateTrack');
  console.log('PASS browser: Winamp title jump opens collapsed queue and selects current track');
 }
 assert.deepEqual(errors,[]);console.log('PASS browser: locate shuffled track 54321 in a 100000-track queue');
 if(process.env.TS_COLLECTION){
  const dir=process.env.TS_COLLECTION,files=fs.readdirSync(dir,{recursive:true,encoding:'utf8'}).filter(n=>/\.pt3$/i.test(n)).map(n=>path.join(dir,n));
  const packed=files.find(f=>fs.readFileSync(f).subarray(-4).toString()==='02TS');
  const native=files.find(f=>{const b=fs.readFileSync(f);return b[98]!==32&&b.subarray(-4).toString()!=='02TS'});
  assert(packed&&native);
  await page.locator('#openPlayer').click();
  for(const f of [packed,native]){
   await page.locator('#localFiles').setInputFiles(f);
   await page.locator('#queue').evaluate(e=>e.scrollTop=e.scrollHeight);
   await page.locator('#queue .track').filter({hasText:path.basename(f)}).dblclick();
   await page.waitForFunction(()=>document.querySelector('#status').textContent.includes('TurboSound'),{},{timeout:15000});
   await page.waitForFunction(()=>document.querySelector('#time').textContent!=='00:00',{},{timeout:15000});
   await page.locator('#stop').click();
   console.log('PASS browser TurboSound',path.basename(f));
  }
  assert.deepEqual(errors,[]);
 }
 }finally{await browser?.close();server.close()}
})().catch(e=>{console.error(e);process.exitCode=1});
