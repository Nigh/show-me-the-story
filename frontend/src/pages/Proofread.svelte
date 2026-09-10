<script>
  import { onMount, tick } from 'svelte';
  import { api, apiFetch } from '../lib/api.js';
  import { progress, postprocess, taskRunning, addToast, confirmModal, config } from '../lib/stores.js';
  import { t } from '../lib/i18n/index.js';

  $: pp = $postprocess?.state || $postprocess || {};
  $: complete = $progress?.book_status === 'completed';
  let downloadedBook = false, downloadedOutline = false, backupChecked = false;
  let preferences = '', selectedStatus = 'all', selectedCategory = 'all';
  let chapter = null, highlighted = 0, selectedBlock = 0, editing = 0, editText = '', insertAfter = 0, insertText = '';
  let continuationName = '';

  async function load() {
    try { const value = await api('GET', '/api/proofread'); postprocess.set({ book_complete: complete, state: value }); preferences = value.author_requirements || ''; }
    catch (e) { addToast(e.message, 'error'); }
  }
  onMount(load);

  async function download(url, mark) {
    try {
      const r = await apiFetch(url);
      const blob = await r.blob(), a = document.createElement('a'); a.href = URL.createObjectURL(blob);
      const base = $config?.story?.title || $progress?.title || 'novel';
      a.download = url.includes('outline') ? `${base}-outlines.md` : url.includes('proofread') ? `${base}-proofreading-report.md` : `${base}.txt`;
      a.click(); URL.revokeObjectURL(a.href); mark();
    } catch (e) { addToast(e.message, 'error'); }
  }
  async function acknowledge() { if (!backupChecked || !downloadedBook || !downloadedOutline) return; const value = await api('POST', '/api/proofread/backup'); postprocess.set({book_complete:true,state:value}); }
  async function run(path, body) { try { await api('POST', path, body); addToast($t('proofread.started'), 'info'); } catch(e) { addToast(e.message, 'error'); } }
  function analyze() { confirmModal.set({message:$t('proofread.analyzeConfirm'),onConfirm:()=>run('/api/proofread/analyze')}); }
  function applyAll() { confirmModal.set({message:$t('proofread.applyConfirm'),onConfirm:()=>run('/api/proofread/apply',{preferences})}); }

  $: filtered = (pp.issues || []).filter(i => (selectedStatus === 'all' || i.status === selectedStatus) && (selectedCategory === 'all' || i.category === selectedCategory));
  $: categories = [...new Set((pp.issues || []).map(i => i.category))];
  $: revisedBlocks = (pp.revisions || []).reduce((n, revision) => n + (revision.changes || []).length, 0);
  function selectBlock(id) {
    if (editing || insertAfter) return;
    highlighted = 0;
    selectedBlock = selectedBlock === id ? 0 : id;
  }
  async function jump(a) {
    try { chapter = await api('GET','/api/chapters/'+a.chapter_num); const block=(chapter.blocks||[]).find(b=>b.id===a.block_id); if(!block){addToast($t('proofread.anchorMissing'),'warning');return;} if(a.content_rev&&chapter.content_rev&&a.content_rev!==chapter.content_rev)addToast($t('proofread.reportOutdated'),'warning'); selectedBlock = 0; highlighted = a.block_id; await tick(); document.getElementById('proofread-block-'+a.block_id)?.scrollIntoView({block:'center',behavior:'smooth'}); }
    catch(e) { addToast(e.message,'error'); }
  }
  async function setStatus(issue,status) { try { const value=await api('PUT','/api/proofread/issues/'+encodeURIComponent(issue.id),{status}); postprocess.set({book_complete:true,state:value}); } catch(e){addToast(e.message,'error');} }
  async function saveBlock(id) { try { chapter=await api('PUT',`/api/proofread/chapters/${chapter.num}/blocks/${id}`,{text:editText}); editing=0; await load(); } catch(e){addToast(e.message,'error');} }
  async function deleteBlock(id) { confirmModal.set({message:$t('proofread.deleteConfirm'),onConfirm:async()=>{try{chapter=await api('DELETE',`/api/proofread/chapters/${chapter.num}/blocks/${id}`);await load();}catch(e){addToast(e.message,'error');}}}); }
  async function addBlock() { try { chapter=await api('POST',`/api/proofread/chapters/${chapter.num}/blocks`,{after_id:insertAfter,text:insertText}); insertAfter=0;insertText='';await load(); } catch(e){addToast(e.message,'error');} }
  async function undo(num) { try { const value=await api('POST','/api/proofread/undo/'+num);postprocess.set({book_complete:true,state:value});chapter=await api('GET','/api/chapters/'+num); }catch(e){addToast(e.message,'error');} }
  async function createContinuation() {
    const name=continuationName.trim(); if(!name)return;
    try { await api('POST','/api/projects/continue',{name}); await api('POST','/api/projects/select',{name}); window.location.hash='#outline'; window.location.reload(); }
    catch(e){addToast(e.message,'error');}
  }
</script>

{#if !complete}
  <div class="alert alert-info">{$t('proofread.completeRequired')}</div>
{:else if !pp.backup_acknowledged}
  <div class="card bg-base-200"><div class="card-body max-w-2xl">
    <h2 class="card-title">{$t('proofread.backupTitle')}</h2><p class="text-sm">{$t('proofread.backupHint')}</p>
    <div class="flex gap-2"><button class="btn btn-primary btn-sm" on:click={()=>download('/api/export/txt',()=>downloadedBook=true)}>{$t('proofread.downloadBook')}</button><button class="btn btn-primary btn-sm" on:click={()=>download('/api/export/outline',()=>downloadedOutline=true)}>{$t('proofread.downloadOutline')}</button></div>
    <label class="label justify-start gap-2"><input class="checkbox checkbox-sm" type="checkbox" bind:checked={backupChecked}/><span>{$t('proofread.backupConfirm')}</span></label>
    <button class="btn btn-success btn-sm w-fit" disabled={!backupChecked||!downloadedBook||!downloadedOutline} on:click={acknowledge}>{$t('proofread.enter')}</button>
  </div></div>
{:else}
  <div class="space-y-3">
    <div class="card bg-base-200"><div class="card-body p-4 gap-3">
      <div class="flex gap-2 items-center flex-wrap"><h2 class="card-title flex-1">{$t('proofread.title')}</h2><button class="btn btn-outline btn-sm" on:click={()=>download('/api/export/txt',()=>{})}>{$t('proofread.downloadBook')}</button><button class="btn btn-outline btn-sm" on:click={()=>download('/api/export/outline',()=>{})}>{$t('proofread.downloadOutline')}</button><button class="btn btn-outline btn-sm" on:click={()=>download('/api/proofread/export',()=>{})}>{$t('proofread.downloadReport')}</button></div>
      <p class="text-xs opacity-60">{$t('proofread.boundary')}</p>
      <textarea class="textarea textarea-bordered textarea-sm w-full" bind:value={preferences} placeholder={$t('proofread.preferences')}></textarea>
      <div class="flex gap-2"><button class="btn btn-primary btn-sm" disabled={$taskRunning} on:click={applyAll}>{$t('proofread.apply')}</button><button class="btn btn-secondary btn-sm" disabled={$taskRunning} on:click={analyze}>{$t('proofread.analyze')}</button></div>
      {#if Object.keys(pp.apply_errors||{}).length}<div class="alert alert-warning text-xs">{$t('proofread.someFailed')}</div>{/if}
    </div></div>

    {#if pp.proofread_applied_at}
      <div class="card bg-base-200"><div class="card-body p-4 gap-3">
        <h3 class="font-bold">{$t('proofread.resultTitle')}</h3>
        {#if (pp.revisions || []).length}
          <p class="text-sm">{$t('proofread.resultSummary', {chapters:(pp.revisions || []).length, blocks:revisedBlocks})}</p>
          <div class="space-y-2">
            {#each pp.revisions || [] as revision}
              <details class="border border-base-300 rounded">
                <summary class="cursor-pointer px-3 py-2 text-sm font-medium">{$t('proofread.resultChapter', {chapter:revision.chapter_num, blocks:(revision.changes || []).length})}</summary>
                <div class="border-t border-base-300 p-3 space-y-3">
                  {#each revision.changes || [] as change}
                    <div class="space-y-2">
                      <div class="text-xs opacity-60">{$t('proofread.resultBlock', {block:change.block_id})}</div>
                      <div class="grid gap-2 min-[48rem]:grid-cols-2">
                        <div><div class="text-xs font-medium mb-1">{$t('proofread.before')}</div><p class="text-sm whitespace-pre-wrap rounded bg-error/10 p-2">{change.before}</p></div>
                        <div><div class="text-xs font-medium mb-1">{$t('proofread.after')}</div><p class="text-sm whitespace-pre-wrap rounded bg-success/10 p-2">{change.after}</p></div>
                      </div>
                    </div>
                  {/each}
                </div>
              </details>
            {/each}
          </div>
        {:else}
          <p class="text-sm opacity-60">{$t('proofread.noChanges')}</p>
        {/if}
      </div></div>
    {/if}

    <div class="grid grid-cols-[345px_minmax(0,1fr)] gap-3 min-h-[520px]">
      <div class="card bg-base-200"><div class="card-body p-3 gap-2 overflow-y-auto max-h-[70vh]">
        <div class="flex gap-1"><select class="select select-xs flex-1" bind:value={selectedStatus}><option value="all">{$t('proofread.allStatus')}</option><option value="pending">{$t('proofread.pending')}</option><option value="resolved">{$t('proofread.resolved')}</option><option value="ignored">{$t('proofread.ignored')}</option></select><select class="select select-xs flex-1" bind:value={selectedCategory}><option value="all">{$t('proofread.allCategory')}</option>{#each categories as c}<option value={c}>{$t('proofread.category.' + c)}</option>{/each}</select></div>
        {#each filtered as issue}
          <div class="border border-base-300 rounded p-2 space-y-1"><div class="font-medium text-sm">{issue.title}</div><div class="text-xs opacity-70">{issue.detail}</div><div class="text-xs">{$t('proofread.suggestion')}：{issue.suggestion}</div>
            <div class="flex flex-wrap gap-1">{#each issue.anchors as a}<button class="btn btn-outline btn-xs" on:click={()=>jump(a)}>{$t('proofread.anchor',{chapter:a.chapter_num,block:a.block_id})}</button>{/each}</div>
            <div class="tabs tabs-box tabs-xs w-fit"><button class="tab" class:tab-active={issue.status==='resolved'} on:click={()=>setStatus(issue,'resolved')}>{$t('proofread.resolved')}</button><button class="tab" class:tab-active={issue.status==='ignored'} on:click={()=>setStatus(issue,'ignored')}>{$t('proofread.ignored')}</button><button class="tab" class:tab-active={issue.status==='pending'} on:click={()=>setStatus(issue,'pending')}>{$t('proofread.pending')}</button></div>
          </div>
        {:else}<p class="text-sm opacity-50 text-center py-6">{$t('proofread.noIssues')}</p>{/each}
      </div></div>
      <div class="card bg-base-200"><div class="card-body p-4 overflow-y-auto max-h-[70vh]">
        {#if chapter}<div class="flex items-center"><h3 class="font-bold flex-1">{$t('proofread.chapter',{n:chapter.num,title:chapter.title})}</h3>{#if (pp.revisions||[]).some(r=>r.chapter_num===chapter.num)}<button class="btn btn-warning btn-xs" on:click={()=>undo(chapter.num)}>{$t('proofread.undo')}</button>{/if}</div>
          <div class="space-y-3 mt-3">{#each chapter.blocks||[] as b (b.id)}<div id={'proofread-block-'+b.id} class="relative rounded px-2 py-1 -mx-2 cursor-pointer transition-colors hover:bg-base-100/40 {highlighted===b.id ? 'bg-info/20' : selectedBlock===b.id ? 'bg-primary/10' : ''}" role="button" tabindex="0" aria-pressed={selectedBlock===b.id} on:click={() => selectBlock(b.id)} on:keydown={(e) => { if (e.currentTarget === e.target && (e.key === 'Enter' || e.key === ' ')) { e.preventDefault(); selectBlock(b.id); } }}>
            {#if editing===b.id}<textarea class="textarea w-full" rows="5" bind:value={editText}></textarea><div class="flex gap-1 justify-end"><button class="btn btn-xs" on:click={()=>editing=0}>{$t('common.cancel')}</button><button class="btn btn-primary btn-xs" on:click={()=>saveBlock(b.id)}>{$t('common.save')}</button></div>
            {:else}<p class="whitespace-pre-wrap leading-relaxed">{b.text}</p>{#if selectedBlock===b.id}<div class="absolute right-1 top-1 flex gap-1 bg-base-200 border border-base-content/20 rounded px-1 py-0.5"><button class="btn btn-outline btn-xs" on:click|stopPropagation={()=>{editing=b.id;editText=b.text}}>{$t('common.edit')}</button><button class="btn btn-outline btn-xs" on:click|stopPropagation={()=>{insertAfter=b.id;insertText=''}}>{$t('proofread.insert')}</button><button class="btn btn-error btn-outline btn-xs" on:click|stopPropagation={()=>deleteBlock(b.id)}>{$t('common.delete')}</button></div>{/if}{/if}
            {#if insertAfter===b.id}<div class="mt-2"><textarea class="textarea w-full" bind:value={insertText}></textarea><button class="btn btn-primary btn-xs" disabled={!insertText.trim()} on:click={addBlock}>{$t('proofread.add')}</button></div>{/if}
          </div>{/each}</div>
        {:else}<p class="opacity-50 text-center py-12">{$t('proofread.pickIssue')}</p>{/if}
      </div></div>
    </div>
    <div class="card bg-base-200"><div class="card-body p-4"><h3 class="font-bold">{$t('proofread.continueTitle')}</h3><p class="text-xs opacity-60">{$t('proofread.continueHint')}</p><div class="join"><input class="input input-sm input-bordered join-item" bind:value={continuationName} placeholder={$t('proofread.continueName')}/><button class="btn btn-primary btn-sm join-item" disabled={!continuationName.trim()||$taskRunning} on:click={createContinuation}>{$t('proofread.createContinuation')}</button></div></div></div>
  </div>
{/if}
