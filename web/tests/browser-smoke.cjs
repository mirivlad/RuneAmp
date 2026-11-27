// Run with PLAYWRIGHT_MODULE pointing to a local Playwright installation.
const {chromium}=require(process.env.PLAYWRIGHT_MODULE||'playwright');
const assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const http=require('node:http');
const root=path.resolve(__dirname,'../dist');
const count=100000,tracks=Array.from({length:count},(_,i)=>({id:i.toString(16).padStart(32,'0'),source:'music',name:'song'+i+'.pt3',path:'ZX/deep/song'+i+'.pt3',mode:'ay',format:'pt3',size:300}));
let lists=[{name:'Test',ids:[]}];
const json=(res,x)=>{res.setHeader('content-type','application/json');res.end(JSON.stringify(x))};
const server=http.createServer((req,res)=>{
 const u=new URL(req.url,'http://localhost');
 if(u.pathname==='/api/library')return json(res,tracks);
 if(u.pathname==='/api/me')return json(res,{role:'admin',name:'test'});
 if(u.pathname==='/api/admin/folder')return json(res,{writable:true,folders:1,tracks:0,entries:[{name:'ZX',path:'ZX',directory:true,music:false,indexed:false}]});
 if(u.pathname==='/api/playlists')return json(res,lists);
 if(u.pathname==='/api/playlists/append'){let data='';req.on('data',x=>data+=x);req.on('end',()=>{const p=JSON.parse(data),list=lists[0],before=list.ids.length;list.ids=[...new Set([...list.ids,...p.ids])];json(res,{added:list.ids.length-before,skipped:p.ids.length-list.ids.length+before,unavailable:0})});return}
 if(u.pathname.startsWith('/api/'))return json(res,[]);
 let file=path.join(root,u.pathname==='/'||u.pathname==='/admin'||u.pathname==='/playlists'?'index.html':u.pathname);
 if(!fs.existsSync(file)){res.statusCode=404;return res.end()}
 res.setHeader('content-type',({'.js':'text/javascript','.css':'text/css','.wasm':'application/wasm','.html':'text/html'})[path.extname(file)]||'application/octet-stream');fs.createReadStream(file).pipe(res);
});
(async()=>{await new Promise(r=>server.listen(0,'127.0.0.1',r));let browser;
 try{
 browser=await chromium.launch({executablePath:process.env.CHROME_BIN||'/usr/bin/google-chrome',headless:true,args:['--no-sandbox']});
 const page=await browser.newPage({viewport:{width:1440,height:900}}),errors=[];
 page.on('pageerror',e=>{errors.push(String(e));console.error(e)});
 const url='http://127.0.0.1:'+server.address().port;
 await page.goto(url+'/admin');await page.locator('#folderContents input[type=checkbox]').first().waitFor();
 await page.waitForFunction(()=>document.querySelector('#libcount').textContent.includes('100000'));
 await page.locator('#selectEntries').check();await page.waitForFunction(()=>document.querySelector('#selectedEntries').textContent.includes('100000'));
 await page.locator('#queueEntries').click();await page.waitForFunction(()=>document.querySelector('#queuecount').textContent.includes('100000'));
 await page.locator('#sendEntries').click();await page.locator('#appendTarget').selectOption('Test');await page.locator('#appendCommit').click();await page.locator('#appendDialog').waitFor({state:'hidden',timeout:60000});assert.equal(lists[0].ids.length,count);
 await page.locator('#home').click();await page.locator('#openPlayer').click();
 assert((await page.locator('#queue .track').count())<100);await page.locator('#queue').evaluate(e=>e.scrollTop=e.scrollHeight);
 await page.waitForFunction(()=>[...document.querySelectorAll('#queue .track')].some(e=>e.textContent.includes('song99999')));
 await page.reload();await page.waitForFunction(()=>document.querySelector('#queuecount').textContent.includes('100000'),{},{timeout:30000});
 assert.deepEqual(errors,[]);console.log('PASS browser: recursive selection, append 100000, virtual scroll, reload');
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
