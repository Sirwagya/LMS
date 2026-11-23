import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync,writeFileSync,rmSync } from 'node:fs';
import { join } from 'node:path';
import { tmpdir } from 'node:os';
import { createJudge } from './coding/judge.mjs';
const judge=createJudge();
const evaluate=(language,source,cases=[{input:'2 3\n',expected:'5',hidden:false}])=>judge.evaluate({language,source,testCases:cases});

test('Python: accepted, wrong answer, custom stdin, runtime errors and hidden-output privacy',async()=>{
 assert.equal((await evaluate('python','a,b=map(int,input().split());print(a+b)')).verdict,'Accepted');
 assert.equal((await evaluate('python','print(0)')).verdict,'Wrong Answer');
 assert.equal((await evaluate('python','print(input()[::-1])',[{input:'abc',expected:null}])).results[0].stdout.trim(),'cba');
 assert.equal((await evaluate('python','print(1/0)')).verdict,'Runtime Error');
 const hidden=await evaluate('python','print(input())',[{input:'secret test data',expected:'different',hidden:true}]);
 assert.equal(hidden.verdict,'Wrong Answer');assert.equal('stdout' in hidden.results[0],false);assert.equal('input' in hidden.results[0],false);assert.equal('expected' in hidden.results[0],false);
});
test('Java: compile, execute, and report compilation errors',async()=>{
 const source='import java.util.*; public class Main {public static void main(String[] args){Scanner s=new Scanner(System.in);System.out.println(s.nextInt()+s.nextInt());}}';
 assert.equal((await evaluate('java',source)).verdict,'Accepted');
 const error=await evaluate('java','public class Main { broken syntax }');assert.equal(error.verdict,'Compilation Error');assert.ok(error.compileError.length>0);
});
test('runner enforces time/output limits and cleans up terminated executions',async()=>{
 assert.equal((await evaluate('python','while True: pass')).verdict,'Time Limit Exceeded');
 assert.equal((await evaluate('python','print("a"*100000)')).verdict,'Output Limit Exceeded');
 assert.equal((await evaluate('python','print(5)')).verdict,'Accepted');
});
test('OS sandbox blocks private file reads, outbound networking and child processes',async()=>{
 const dir=mkdtempSync(join(tmpdir(),'vedam-private-')),secret=join(dir,'private.txt');writeFileSync(secret,'must stay private');
 try{
 const source=`import socket,os\ntry:\n    open(${JSON.stringify(secret)}).read()\n    print("file-open")\nexcept PermissionError:\n    print("file-blocked")\ntry:\n    socket.socket().connect(("127.0.0.1",9))\n    print("network-open")\nexcept PermissionError:\n    print("network-blocked")\ntry:\n    os.fork()\n    print("fork-open")\nexcept PermissionError:\n    print("fork-blocked")`;
 const r=await evaluate('python',source,[{input:'',expected:'file-blocked\nnetwork-blocked\nfork-blocked'}]);assert.equal(r.verdict,'Accepted',JSON.stringify(r));
 }finally{rmSync(dir,{recursive:true,force:true});}
});
