'use strict'
// Runtime dependencies live outside the project and shared skill sources.
const fs = require('node:fs'), path = require('node:path'), os = require('node:os')
const { spawnSync } = require('node:child_process')
const { createRequire } = require('node:module')
function cacheBase() {
  const base = process.platform === 'win32' ? process.env.LOCALAPPDATA || path.join(os.homedir(),'AppData','Local') : process.platform === 'darwin' ? path.join(os.homedir(),'Library','Caches') : process.env.XDG_CACHE_HOME || path.join(os.homedir(),'.cache')
  return path.join(base,'ffc','skill-runtime','v1')
}
function run(exe,args,opts={}) {
  const result = spawnSync(exe,args,{ windowsHide:true,shell:false,encoding:'utf8',timeout:180000,...opts })
  if (result.error || result.status !== 0) throw new Error(result.error?.message || String(result.stderr || result.stdout || `${exe} exited ${result.status}`).slice(-4000))
  return result.stdout?.trim() || ''
}
async function locked(name,task) {
  const root=cacheBase();fs.mkdirSync(root,{recursive:true});const lock=path.join(root,`${name}.lock`),deadline=Date.now()+185000
  for (;;) { try { const fd=fs.openSync(lock,'wx');try{return await task()}finally{fs.closeSync(fd);fs.rmSync(lock,{force:true})} }
    catch(e) { if(e.code!=='EEXIST')throw e;if(Date.now()>deadline)throw new Error(`Another ${name} setup is still running`);if(Date.now()-fs.statSync(lock).mtimeMs>240000){fs.rmSync(lock,{force:true});continue}await new Promise(r=>setTimeout(r,400)) }
  }
}
function npmCLI() {
  const candidates=[process.env.npm_execpath,path.join(path.dirname(process.execPath),'node_modules','npm','bin','npm-cli.js')]
  for(const dir of (process.env.PATH||'').split(path.delimiter))candidates.push(path.join(dir,'node_modules','npm','bin','npm-cli.js'))
  const result=candidates.find(p=>p&&p.endsWith('.js')&&fs.existsSync(p));if(!result)throw new Error('Node.js npm is required to prepare Mermaid');return result
}
function nodeRoot(){return path.join(cacheBase(),'node')}
function nodeReady(){try{return createRequire(path.join(nodeRoot(),'package.json'))('beautiful-mermaid/package.json').version==='1.1.3'}catch{return fs.existsSync(path.join(nodeRoot(),'node_modules','beautiful-mermaid','dist','index.js'))}}
async function ensureNode(){
  if(nodeReady())return nodeRoot()
  return locked('mermaid',async()=>{
    if(!nodeReady()){
      fs.mkdirSync(nodeRoot(),{recursive:true})
      for(const file of ['package.json','package-lock.json'])fs.copyFileSync(path.join(__dirname,file),path.join(nodeRoot(),file))
      run(process.execPath,[npmCLI(),'ci','--ignore-scripts','--no-audit','--no-fund','--prefix',nodeRoot()])
    }
    return nodeRoot()
  })
}
function venvPython(){return process.platform==='win32'?path.join(cacheBase(),'python','Scripts','python.exe'):path.join(cacheBase(),'python','bin','python')}
function findPython(){
  const candidates=[process.env.FFC_PYTHON,venvPython(),path.join(os.homedir(),'.cache','codex-runtimes','codex-primary-runtime','dependencies','python',process.platform==='win32'?'python.exe':'bin/python3'),'python3','python']
  for(const exe of candidates.filter(Boolean)){try{const result=run(exe,['-c','import sys; print(sys.executable)'],{timeout:5000});if(result)return result}catch{}}
  throw new Error('Python 3 is required for PDF and Morph helpers; set FFC_PYTHON to its executable')
}
function pdfReady(exe){try{run(exe,['-c','import pypdf, reportlab, pypdfium2'],{timeout:10000});return true}catch{return false}}
async function ensurePython(pdf=false){
  const exe=findPython();if(!pdf||pdfReady(exe))return exe
  return locked('pdf',async()=>{
    if(!fs.existsSync(venvPython()))run(exe,['-m','venv',path.join(cacheBase(),'python')])
    if(!pdfReady(venvPython()))run(venvPython(),['-m','pip','install','--disable-pip-version-check','--no-input','-r',path.join(__dirname,'requirements.txt')])
    return venvPython()
  })
}
module.exports={cacheBase,ensureNode,ensurePython,run,nodeRoot}
if(require.main===module){
  const only=process.argv[2]||'all'
  Promise.all([only==='node'||only==='all'?ensureNode():null,only==='pdf'||only==='all'?ensurePython(true):null]).then(([node,python])=>console.log(JSON.stringify({node,python}))).catch(e=>{console.error(e.message);process.exitCode=1})
}
