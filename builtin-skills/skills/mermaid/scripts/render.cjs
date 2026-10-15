'use strict'
const fs=require('node:fs'),path=require('node:path'),{createRequire}=require('node:module'),{pathToFileURL}=require('node:url')
async function main(){
  const args=process.argv.slice(2)
  if(args.includes('--help')||!args.length){console.log('render.cjs FILE.mmd [--output FILE.svg] [--ascii] [--theme NAME] [--stdin] [--force]');return}
  let input,output,theme='github-light',ascii=false,stdin=false,force=false
  for(let i=0;i<args.length;i++){const a=args[i];if(a==='--output')output=args[++i];else if(a==='--theme')theme=args[++i];else if(a==='--ascii')ascii=true;else if(a==='--stdin')stdin=true;else if(a==='--force')force=true;else if(a.startsWith('-'))throw new Error(`Unknown option: ${a}`);else input=a}
  const installed=path.join(__dirname,'..','..','.ffc-runtime','setup.cjs')
  const runtime=require(fs.existsSync(installed)?installed:path.join(__dirname,'..','..','..','runtime','setup.cjs'))
  const root=await runtime.ensureNode(),req=createRequire(path.join(root,'package.json')),engine=await import(pathToFileURL(req.resolve('beautiful-mermaid')).href)
  const source=stdin?await new Promise((resolve,reject)=>{let text='';process.stdin.setEncoding('utf8');process.stdin.on('data',b=>{text+=b;if(text.length>1024*1024){reject(new Error('Mermaid source exceeds 1 MiB'));process.stdin.destroy()}});process.stdin.on('end',()=>resolve(text))}):fs.readFileSync(input,'utf8')
  if(source.length>1024*1024)throw new Error('Mermaid source exceeds 1 MiB')
  const colors=engine.THEMES[theme];if(!colors)throw new Error(`Unknown theme; choose ${Object.keys(engine.THEMES).join(', ')}`)
  const result=ascii?engine.renderMermaidAscii(source):await engine.renderMermaid(source,colors)
  if(output){fs.mkdirSync(path.dirname(path.resolve(output)),{recursive:true});fs.writeFileSync(output,result,{encoding:'utf8',flag:force?'w':'wx'})}else console.log(result)
}
main().catch(e=>{console.error(e.message);process.exitCode=1})
