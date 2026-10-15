'use strict'
const fs=require('node:fs'),path=require('node:path'),{spawn}=require('node:child_process')
const installed=path.join(__dirname,'..','..','.ffc-runtime','setup.cjs')
const runtime=require(fs.existsSync(installed)?installed:path.join(__dirname,'..','..','..','runtime','setup.cjs'))
runtime.ensurePython(true).then(python=>{
  const child=spawn(python,[path.join(__dirname,'pdf.py'),...process.argv.slice(2)],{stdio:'inherit',windowsHide:true,shell:false})
  child.on('error',e=>{console.error(e.message);process.exitCode=1});child.on('exit',code=>{process.exitCode=code??1})
}).catch(e=>{console.error(e.message);process.exitCode=1})
