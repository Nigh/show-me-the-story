<script>
  import { onMount } from 'svelte';
  import { api, apiFetch } from '../lib/api.js';
  import { skills, addToast, taskRunning, confirmModal } from '../lib/stores.js';
  import { t } from '../lib/i18n/index.js';
  let mode='paste', markdown='', selectedFile=null, folderFiles=[], overwrite=false, installing=false, detail=null, wasRunning=false, staticCheck=null;
  async function load(){try{skills.set(await api('GET','/api/skill-library'));}catch(e){addToast(e.message,'error')}}
  onMount(load); $: if(wasRunning&&!$taskRunning)load(); $: wasRunning=$taskRunning;
  async function toggleSkill(id,enabled){try{await api('PUT','/api/skills/'+encodeURIComponent(id)+'/toggle',{enabled});addToast(enabled?$t('skills.toast.enabled'):$t('skills.toast.disabled'),'success');await load()}catch(e){addToast(e.message,'error');await load()}}
  async function upload(body,validateOnly=false){const url='/api/skill-library/install'+(validateOnly?'?validate_only=true':'');return (await apiFetch(url,{method:'POST',body})).json()}
  async function install(){installing=true;staticCheck=null;try{if(mode==='paste'){if(!markdown.trim())throw new Error($t('skills.install.needContent'));staticCheck=await api('POST','/api/skill-library/install?validate_only=true',{markdown,overwrite});await api('POST','/api/skill-library/install',{markdown,overwrite})}else{const fd=new FormData();fd.append('overwrite',String(overwrite));if(mode==='zip'){if(!selectedFile)throw new Error($t('skills.install.needFile'));fd.append('zip',selectedFile)}else if(mode==='markdown'){if(!selectedFile)throw new Error($t('skills.install.needFile'));fd.append('files',selectedFile);fd.append('paths',JSON.stringify([selectedFile.name]))}else{if(!folderFiles.length)throw new Error($t('skills.install.needFolder'));fd.append('paths',JSON.stringify(folderFiles.map(f=>f.webkitRelativePath||f.name)));folderFiles.forEach(f=>fd.append('files',f))}staticCheck=await upload(fd,true);await upload(fd)}addToast($t('skills.install.done'),'success');markdown='';selectedFile=null;folderFiles=[];overwrite=false;await load()}catch(e){staticCheck={valid:false,error:e.message};addToast(e.message,'error')}finally{installing=false}}
  async function validateSkill(id){try{await api('POST',`/api/skill-library/${encodeURIComponent(id)}/validate`);addToast($t('skills.validate.started'),'info');await load()}catch(e){addToast(e.message,'error')}}
  async function optimizeSkill(id){try{await api('POST',`/api/skill-library/${encodeURIComponent(id)}/optimize`);addToast($t('skills.optimize.started'),'info')}catch(e){addToast(e.message,'error')}}
  async function showDetail(id){try{detail=await api('GET',`/api/skill-library/${encodeURIComponent(id)}`)}catch(e){addToast(e.message,'error')}}
  function removeSkill(id){confirmModal.set({message:$t('skills.delete.confirm'),onConfirm:async()=>{try{await api('DELETE',`/api/skill-library/${encodeURIComponent(id)}`);addToast($t('skills.delete.done'),'success');detail=null;await load()}catch(e){addToast(e.message,'error')}}})}
  const blocked=s=>['failed','needs_optimization','validating'].includes(s);
  function statusIcon(s){return s==='passed'?'✓':s==='failed'?'✕':s==='needs_optimization'?'⚠':s==='validating'?'…':'!'}
  function statusClass(s){return s==='passed'?'badge-success':s==='failed'?'badge-error':s==='needs_optimization'?'badge-warning':s==='validating'?'badge-info':'badge-ghost'}
</script>

<div class="space-y-4">
 <div class="card bg-base-200"><div class="card-body p-4">
  <h2 class="card-title text-base">{$t('skills.install.title')}</h2><div class="tabs tabs-box w-fit">{#each ['paste','markdown','zip','folder'] as item}<button class:tab-active={mode===item} class="tab" on:click={()=>mode=item}>{$t(`skills.install.${item}`)}</button>{/each}</div>
  {#if mode==='paste'}<textarea class="textarea textarea-bordered w-full h-40 font-mono text-xs" bind:value={markdown} disabled={$taskRunning} placeholder="---&#10;id: my-skill&#10;name: My Skill&#10;category: writing&#10;applies_to: [chapter.generate]&#10;---"></textarea>
  {:else if mode==='folder'}<input class="file-input file-input-bordered w-full" type="file" webkitdirectory multiple disabled={$taskRunning} on:change={e=>folderFiles=Array.from(e.currentTarget.files)}/>{#if folderFiles.length}<p class="text-xs opacity-60">{folderFiles.length} {$t('skills.install.files')}</p>{/if}
  {:else}<input class="file-input file-input-bordered w-full" type="file" accept={mode==='zip'?'.zip':'.md,text/markdown'} disabled={$taskRunning} on:change={e=>selectedFile=e.currentTarget.files[0]}/>{/if}
  <label class="label cursor-pointer justify-start gap-3"><input type="checkbox" class="checkbox checkbox-sm" bind:checked={overwrite} disabled={$taskRunning}/><span>{$t('skills.install.overwrite')}</span></label>
  <button class="btn btn-primary btn-sm w-fit" disabled={$taskRunning||installing} on:click={install}>{installing?$t('skills.install.installing'):$t('skills.install.button')}</button>{#if staticCheck}<div class="alert py-2 {staticCheck.valid?'alert-success':'alert-error'}"><span>{staticCheck.valid?$t('skills.install.valid'):`${$t('skills.install.invalid')}: ${staticCheck.error}`}</span></div>{/if}<p class="text-xs opacity-60">{$t('skills.install.security')}</p>
 </div></div>
 <div class="card bg-base-200"><div class="card-body p-4"><h2 class="card-title text-base">{$t('skills.title')}</h2><p class="text-sm opacity-60 mb-3">{$t('skills.intro')}</p><div class="overflow-x-auto"><table class="table table-sm">
  <thead><tr><th>{$t('skills.col.name')}</th><th>{$t('skills.col.scope')}</th><th>{$t('skills.col.validation')}</th><th>{$t('skills.col.source')}</th><th>{$t('skills.col.actions')}</th><th>{$t('skills.col.enabled')}</th></tr></thead><tbody>
  {#if $skills.length===0}<tr><td colspan="6" class="text-center opacity-50 py-8">{$t('skills.empty')}</td></tr>{/if}
  {#each $skills as sv}<tr>
   <td><button class="link link-hover font-medium text-left" on:click={()=>showDetail(sv.skill.id)}>{sv.skill.name}</button><div class="text-xs opacity-50 max-w-xs">{sv.skill.description}</div></td>
   <td><div class="flex flex-wrap gap-1">{#each sv.skill.applies_to||[] as scope}<span class="badge badge-outline badge-xs">{scope}</span>{/each}</div></td>
   <td><span class="badge badge-sm {statusClass(sv.validation_status)}" title={$t(`skills.status.${sv.validation_status}`)}>{statusIcon(sv.validation_status)}</span></td>
   <td>{sv.skill.source==='builtin'?$t('skills.source.builtin'):$t('skills.source.user')}</td>
   <td><div class="flex gap-1">{#if sv.can_validate}<button class="btn btn-outline btn-xs" disabled={$taskRunning} on:click={()=>validateSkill(sv.skill.id)}>{$t('skills.validate.button')}</button>{/if}{#if sv.validation_status==='failed'||sv.validation_status==='needs_optimization'}<button class="btn btn-warning btn-xs" disabled={$taskRunning} on:click={()=>optimizeSkill(sv.skill.id)}>{$t('skills.optimize.button')}</button>{/if}{#if sv.can_delete}<button class="btn btn-error btn-outline btn-xs" disabled={$taskRunning} on:click={()=>removeSkill(sv.skill.id)}>{$t('common.delete')}</button>{/if}</div></td>
   <td><input type="checkbox" class="toggle toggle-primary toggle-sm" checked={sv.enabled} disabled={$taskRunning||blocked(sv.validation_status)} on:change={e=>toggleSkill(sv.skill.id,e.target.checked)}/></td>
  </tr>{/each}</tbody></table></div></div></div>
</div>

{#if detail}<div class="modal modal-open"><div class="modal-box max-w-3xl"><h3 class="font-bold text-lg">{detail.skill.name}</h3><p class="text-sm opacity-70">{detail.skill.description}</p><div class="flex flex-wrap gap-1 my-3">{#each detail.skill.applies_to||[] as scope}<span class="badge badge-outline badge-sm">{scope}</span>{/each}</div>{#if detail.skill.validation}<div class="alert mb-3"><div><strong>{$t(`skills.status.${detail.validation_status}`)}</strong><p>{detail.skill.validation.summary}</p>{#if detail.skill.validation.issues?.length}<ul class="list-disc ml-5 text-sm">{#each detail.skill.validation.issues as issue}<li>{issue}</li>{/each}</ul>{/if}</div></div>{/if}{#if detail.skill.resources?.length}<p class="text-xs mb-2">{$t('skills.resources')}: {detail.skill.resources.join(', ')}</p>{/if}<pre class="bg-base-300 rounded p-3 max-h-96 overflow-auto whitespace-pre-wrap text-xs">{detail.skill.content}</pre><div class="modal-action"><button class="btn" on:click={()=>detail=null}>{$t('common.close')}</button></div></div><button class="modal-backdrop" on:click={()=>detail=null}>close</button></div>{/if}
