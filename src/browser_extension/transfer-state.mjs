export function transferSummary(jobs, expected=jobs.length) {
 const valid=jobs.filter(Boolean), total=Math.max(expected,valid.length);
 const done=valid.filter(j=>j.status==='done'&&Number(j.file_count)>0);
 const failed=valid.filter(j=>['error','cancelled'].includes(j.status));
 const active=valid.filter(j=>['queued','downloading'].includes(j.status));
 const files=valid.reduce((n,j)=>n+Math.max(0,Number(j.file_count)||0),0);
 let state='waiting';
 if(total>0&&valid.length===total&&done.length===total)state='done';
 else if(active.length)state='active';
 else if(failed.length)state='partial';
 else if(valid.some(j=>j.status==='unavailable'||j.status==='done'))state='unknown';
 return {state,total,done:done.length,failed:failed.length,files,active:active.length};
}
export function candidateJobs(key,outcome,jobs) {
 if(outcome?.job_ids?.length) return outcome.job_ids.map(id=>jobs.find(j=>j.id===id)).filter(Boolean);
 const matching=jobs.filter(j=>j.key===key);
 if(!matching.length)return [];
 const latest=Math.max(...matching.map(j=>Number(j.created)||0));
 return matching.filter(j=>(Number(j.created)||0)===latest);
}
