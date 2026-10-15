'use strict'
const fs=require('node:fs'),path=require('node:path'),os=require('node:os'),crypto=require('node:crypto')
const {spawn}=require('node:child_process')
const {cacheBase,ensurePython}=require('./setup.cjs')
const release=require('./officecli-release.json')
function valid(file,digest){try{return crypto.createHash('sha256').update(fs.readFileSync(file)).digest('hex')===digest}catch{return false}}
async function executable(){
  if(process.env.FFC_OFFICECLI){const file=path.resolve(process.env.FFC_OFFICECLI);if(!fs.statSync(file).isFile())throw new Error('FFC_OFFICECLI is not a file');return file}
  const platform={win32:'win',darwin:'mac',linux:'linux'}[process.platform],arch={x64:'x64',arm64:'arm64'}[process.arch]
  const asset=release.assets[`${platform}-${arch}`];if(!asset)throw new Error('OfficeCLI does not support this platform')
  const root=path.resolve(cacheBase(),'..','..','tools','officecli',release.version),file=path.join(root,process.platform==='win32'?'officecli.exe':'officecli')
  if(valid(file,asset.sha256))return file
  fs.mkdirSync(root,{recursive:true});let last
  for(const base of ['https://d.officecli.ai/releases/download/','https://github.com/iOfficeAI/OfficeCLI/releases/download/']){
    const temp=path.join(root,`.download-${crypto.randomUUID()}`)
    try{
      const response=await fetch(`${base}v${release.version}/${asset.name}`,{signal:AbortSignal.timeout(75000)})
      if(!response.ok)throw new Error(`OfficeCLI download HTTP ${response.status}`)
      const chunks=[];let size=0
      for await(const chunk of response.body){size+=chunk.length;if(size>80*1024*1024)throw new Error('OfficeCLI download too large');chunks.push(chunk)}
      const bytes=Buffer.concat(chunks);if(crypto.createHash('sha256').update(bytes).digest('hex')!==asset.sha256)throw new Error('OfficeCLI SHA256 mismatch')
      fs.writeFileSync(temp,bytes,{mode:0o700,flag:'wx'})
      try{fs.renameSync(temp,file)}catch(e){if(!valid(file,asset.sha256))throw e}
      return file
    }catch(e){last=e}finally{fs.rmSync(temp,{force:true})}
  }
  throw last
}
async function invoke(args=process.argv.slice(2)){
  const exe=await executable()
  if(args[0]==='--native-path'){console.log(exe);return}
  const env={...process.env,PATH:path.dirname(exe)+path.delimiter+(process.env.PATH||''),FFC_OFFICECLI:exe,OFFICECLI_SKIP_UPDATE:'1',OFFICECLI_NO_AUTO_RESIDENT:'1',OFFICECLI_RESIDENT_FLUSH:'each'}
  let program=exe
  if(args[0]==='--python'){program=await ensurePython(false);args=args.slice(1)}
  for(let attempt=0;attempt<4;attempt++){
    const result=await new Promise((resolve,reject)=>{
      let error='';const child=spawn(program,args,{env,stdio:['inherit','inherit','pipe'],windowsHide:true,shell:false})
      child.stderr.on('data',b=>{error=(error+b).slice(-8192)});child.on('error',reject);child.on('exit',(code,signal)=>resolve({code,signal,error}))
    })
    if(result.code===0){if(result.error)process.stderr.write(result.error);return}
    if(attempt<3&&/Error: The process cannot access the file .*being used by another process/i.test(result.error)){await new Promise(r=>setTimeout(r,300*(attempt+1)));continue}
    throw new Error(result.error||`OfficeCLI exited ${result.code??result.signal}`)
  }
}
module.exports={executable,invoke}
if(require.main===module)invoke().catch(e=>{console.error(e.message);process.exitCode=1})
