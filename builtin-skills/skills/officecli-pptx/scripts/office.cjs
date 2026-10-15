'use strict'
const path=require('node:path'),fs=require('node:fs')
const installed=path.join(__dirname,'..','..','.ffc-runtime','office.cjs')
const runtime=fs.existsSync(installed)?installed:path.join(__dirname,'..','..','..','runtime','office.cjs')
require(runtime).invoke().catch(e=>{console.error(e.message);process.exitCode=1})
