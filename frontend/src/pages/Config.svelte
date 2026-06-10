<script>
  import { onMount } from 'svelte';
  import { api } from '../lib/api.js';
  import { apiConfig, config, progress, settings, editingCharID, editingWvID, wvFilter, addToast, showConfirm, taskRunning } from '../lib/stores.js';

  export let sendToChat = async () => {};

  let showCharForm = false;
  let showWvForm = false;
  let charCollapse = false;
  let wvCollapse = false;

  let charName = '', charAge = '', charAppearance = '', charPersonality = '', charBackground = '', charMotivation = '', charAbilities = '', charNotes = '';
  let wvName = '', wvCategory = 'other', wvDescription = '', wvTags = '';

  $: cfgBase = $apiConfig?.base_url || '';
  $: cfgModel = $apiConfig?.model || '';
  $: cfgKey = $apiConfig?.api_key || '';
  $: cfgTimeout = $apiConfig?.http_timeout_seconds || 300;

  let localApiCfg = { base_url: '', model: '', api_key: '', http_timeout_seconds: 300 };
  let localStoryCfg = { type: '', title: '', chapter_count: 30, target_words_per_chapter: 2500, writing_style: '', story_synopsis: '' };

  let apiCfgSnapshot = '';
  let storyCfgSnapshot = '';

  $: if ($apiConfig) {
    const snap = JSON.stringify($apiConfig);
    if (snap !== apiCfgSnapshot) {
      localApiCfg = { ...$apiConfig };
      apiCfgSnapshot = snap;
    }
  }
  $: if ($config?.story) {
    const snap = JSON.stringify($config.story);
    if (snap !== storyCfgSnapshot) {
      localStoryCfg = { ...$config.story };
      storyCfgSnapshot = snap;
    }
  }

  $: hasAccepted = $progress?.chapters?.some(c => c.status === 'accepted') || false;

  $: chars = ($settings?.characters || []);
  $: allWvs = ($settings?.worldview || []);
  $: filteredWvs = $wvFilter === 'all' ? allWvs : allWvs.filter(w => w.category === $wvFilter);

  const catLabels = { geography: '地理', faction: '势力', rule: '规则', history: '历史', other: '其他' };
  const wvTabs = [
    ['all', '全部'],
    ['geography', '地理'],
    ['faction', '势力'],
    ['rule', '规则'],
    ['history', '历史'],
    ['other', '其他']
  ];

  onMount(async () => {
    try { apiConfig.set(await api('GET', '/api/config/api')); } catch (e) {}
    try { config.set(await api('GET', '/api/config')); } catch (e) {}
    try { settings.set(await api('GET', '/api/settings')); } catch (e) {}
  });

  async function saveAPIConfig() {
    try {
      await api('PUT', '/api/config/api', localApiCfg);
      apiConfig.set({ ...localApiCfg });
      addToast('API 配置已保存', 'success');
    } catch (e) { addToast(e.message, 'error'); }
  }

  // 直接保存故事配置（不经过 AI），存在已确认章节且关键设定有变化时提示协调
  async function saveStoryConfig() {
    const prev = $config?.story || {};
    const story = {
      ...localStoryCfg,
      chapter_count: Number(localStoryCfg.chapter_count) || 30,
      target_words_per_chapter: Number(localStoryCfg.target_words_per_chapter) || 2500,
    };
    const settingsChanged =
      story.type !== prev.type ||
      story.writing_style !== prev.writing_style ||
      story.story_synopsis !== prev.story_synopsis;

    try {
      const saved = await api('PUT', '/api/config', { ...($config || {}), story });
      config.set(saved);
      addToast('故事配置已保存', 'success');

      if (hasAccepted && settingsChanged) {
        showConfirm('检测到关键设定有变化，且已有已确认章节。是否让 AI 协调新设定与已有内容的一致性？（推荐）', async () => {
          try {
            await api('POST', '/api/settings/reconcile', saved.story);
            addToast('设定协调任务已启动', 'info');
          } catch (e) { addToast(e.message, 'error'); }
        });
      }
    } catch (e) { addToast(e.message, 'error'); }
  }

  function openCharForm(char) {
    showCharForm = true;
    if (char) {
      $editingCharID = char.id;
      charName = char.name || '';
      charAge = char.age || '';
      charAppearance = char.appearance || '';
      charPersonality = char.personality || '';
      charBackground = char.background || '';
      charMotivation = char.motivation || '';
      charAbilities = char.abilities || '';
      charNotes = char.notes || '';
    } else {
      $editingCharID = null;
      charName = charAge = charAppearance = charPersonality = charBackground = charMotivation = charAbilities = charNotes = '';
    }
  }

  function closeCharForm() {
    showCharForm = false;
    $editingCharID = null;
  }

  async function saveCharacter() {
    if (!charName.trim()) { addToast('角色名不能为空', 'error'); return; }
    const data = { name: charName.trim(), age: charAge, appearance: charAppearance, personality: charPersonality, background: charBackground, motivation: charMotivation, abilities: charAbilities, notes: charNotes };
    try {
      if ($editingCharID) {
        await api('PUT', '/api/characters/' + $editingCharID, data);
      } else {
        await api('POST', '/api/characters', data);
      }
      addToast('角色已保存', 'success');
      closeCharForm();
      settings.set(await api('GET', '/api/settings'));
    } catch (e) { addToast(e.message, 'error'); }
  }

  async function deleteCharacter(id) {
    showConfirm('确认删除此角色？', async () => {
      try {
        await api('DELETE', '/api/characters/' + id);
        addToast('角色已删除', 'success');
        settings.set(await api('GET', '/api/settings'));
      } catch (e) { addToast(e.message, 'error'); }
    });
  }

  async function submitCharacters() {
    if (chars.length === 0) { addToast('暂无角色可提交', 'error'); return; }
    const lines = chars.map(c => `- ${c.name}${c.age ? '，' + c.age : ''}${c.personality ? '，' + c.personality : ''}`);
    await sendToChat(`请查看以下角色设定（共 ${chars.length} 个）：\n${lines.join('\n')}\n请使用 read_characters 工具获取详细信息。`);
    addToast('角色设定已提交到 AI', 'success');
  }

  function openWvForm(item) {
    showWvForm = true;
    if (item) {
      $editingWvID = item.id;
      wvName = item.name || '';
      wvCategory = item.category || 'other';
      wvDescription = item.description || '';
      wvTags = item.tags || '';
    } else {
      $editingWvID = null;
      wvName = ''; wvCategory = 'other'; wvDescription = ''; wvTags = '';
    }
  }

  function closeWvForm() {
    showWvForm = false;
    $editingWvID = null;
  }

  async function saveWorldview() {
    if (!wvName.trim() || !wvDescription.trim()) { addToast('名称和描述不能为空', 'error'); return; }
    const data = { name: wvName.trim(), category: wvCategory, description: wvDescription.trim(), tags: wvTags };
    try {
      if ($editingWvID) {
        await api('PUT', '/api/worldview/' + $editingWvID, data);
      } else {
        await api('POST', '/api/worldview', data);
      }
      addToast('世界观条目已保存', 'success');
      closeWvForm();
      settings.set(await api('GET', '/api/settings'));
    } catch (e) { addToast(e.message, 'error'); }
  }

  async function deleteWorldview(id) {
    showConfirm('确认删除此世界观条目？', async () => {
      try {
        await api('DELETE', '/api/worldview/' + id);
        addToast('世界观条目已删除', 'success');
        settings.set(await api('GET', '/api/settings'));
      } catch (e) { addToast(e.message, 'error'); }
    });
  }

  async function submitWorldview() {
    if (allWvs.length === 0) { addToast('暂无世界观条目可提交', 'error'); return; }
    const lines = allWvs.map(w => `- [${catLabels[w.category] || w.category}] ${w.name}: ${w.description.slice(0, 50)}`);
    await sendToChat(`请查看以下世界观设定（共 ${allWvs.length} 条）：\n${lines.join('\n')}\n请使用 read_worldview 工具获取详细信息。`);
    addToast('世界观设定已提交到 AI', 'success');
  }
</script>

<div class="space-y-3">
  <!-- API + Story Config: side by side -->
  <div class="grid grid-cols-2 gap-3">
    <div class="card bg-base-200 shadow-sm">
      <div class="card-body p-4 gap-2">
        <h3 class="card-title text-base">API 配置</h3>
        <div class="grid grid-cols-2 gap-x-3 gap-y-1.5">
          <div class="col-span-2">
            <label class="text-xs text-base-content/50 mb-0.5 block">API Base URL</label>
            <input type="text" class="input input-sm w-full" bind:value={localApiCfg.base_url} placeholder="https://api.example.com/v1/" />
          </div>
          <div>
            <label class="text-xs text-base-content/50 mb-0.5 block">Model</label>
            <input type="text" class="input input-sm w-full" bind:value={localApiCfg.model} placeholder="gpt-4" />
          </div>
          <div>
            <label class="text-xs text-base-content/50 mb-0.5 block">HTTP 超时（秒）</label>
            <input type="number" class="input input-sm w-full" bind:value={localApiCfg.http_timeout_seconds} />
          </div>
          <div class="col-span-2">
            <label class="text-xs text-base-content/50 mb-0.5 block">API Key</label>
            <input type="password" class="input input-sm w-full" bind:value={localApiCfg.api_key} placeholder="sk-..." />
          </div>
        </div>
        <div class="flex justify-end">
          <button class="btn btn-primary btn-xs" on:click={saveAPIConfig} disabled={$taskRunning}>保存</button>
        </div>
      </div>
    </div>

    <div class="card bg-base-200 shadow-sm">
      <div class="card-body p-4 gap-2">
        <h3 class="card-title text-base">故事配置</h3>
        {#if hasAccepted}
          <div class="alert alert-warning text-xs py-1.5 px-3">
            <span>已有已确认章节，修改关键设定后建议执行设定协调。</span>
          </div>
        {/if}
        <div class="grid grid-cols-2 gap-x-3 gap-y-1.5">
          <div>
            <label class="text-xs text-base-content/50 mb-0.5 block">故事类型</label>
            <input type="text" class="input input-sm w-full" bind:value={localStoryCfg.type} placeholder="奇幻/都市/科幻..." />
          </div>
          <div>
            <label class="text-xs text-base-content/50 mb-0.5 block">小说标题（留空由 AI 生成）</label>
            <input type="text" class="input input-sm w-full" bind:value={localStoryCfg.title} placeholder="留空则 AI 自动生成" />
          </div>
          <div>
            <label class="text-xs text-base-content/50 mb-0.5 block">章节数量</label>
            <input type="number" class="input input-sm w-full" bind:value={localStoryCfg.chapter_count} />
          </div>
          <div>
            <label class="text-xs text-base-content/50 mb-0.5 block">每章目标字数</label>
            <input type="number" class="input input-sm w-full" bind:value={localStoryCfg.target_words_per_chapter} />
          </div>
        </div>
        <div class="flex justify-end">
          <button class="btn btn-primary btn-xs" on:click={saveStoryConfig} disabled={$taskRunning}>保存</button>
        </div>
      </div>
    </div>
  </div>

  <!-- Writing Style -->
  <div class="card bg-base-200 shadow-sm">
    <div class="card-body p-4 gap-2">
      <h3 class="card-title text-base">写作风格</h3>
      <textarea class="textarea w-full h-40 text-base" bind:value={localStoryCfg.writing_style} placeholder="描述你期望的写作风格..."></textarea>
      <div class="flex justify-end">
        <button class="btn btn-primary btn-xs" on:click={saveStoryConfig} disabled={$taskRunning}>保存</button>
      </div>
    </div>
  </div>

  <!-- Story Synopsis -->
  <div class="card bg-base-200 shadow-sm">
    <div class="card-body p-4 gap-2">
      <h3 class="card-title text-base">故事梗概</h3>
      <textarea class="textarea w-full h-40 text-base" bind:value={localStoryCfg.story_synopsis} placeholder="可包含：故事主线走向、核心冲突、关键转折点..."></textarea>
      <div class="flex justify-end">
        <button class="btn btn-primary btn-xs" on:click={saveStoryConfig} disabled={$taskRunning}>保存</button>
      </div>
    </div>
  </div>

  <!-- Characters -->
  <div class="card bg-base-200 shadow-sm">
    <div class="card-body p-4 gap-2">
      <!-- svelte-ignore a11y-click-events-have-key-events -->
      <!-- svelte-ignore a11y-no-static-element-interactions -->
      <div class="flex justify-between items-center cursor-pointer select-none" on:click={() => charCollapse = !charCollapse}>
        <h3 class="card-title text-base">角色管理 <span class="text-xs font-normal text-base-content/40">({chars.length})</span></h3>
        <svg class="w-4 h-4 text-base-content/40 transition-transform" class:rotate-180={charCollapse} viewBox="0 0 20 20" fill="currentColor"><path fill-rule="evenodd" d="M5.23 7.21a.75.75 0 011.06.02L10 11.168l3.71-3.938a.75.75 0 111.08 1.04l-4.25 4.5a.75.75 0 01-1.08 0l-4.25-4.5a.75.75 0 01.02-1.06z" clip-rule="evenodd"/></svg>
      </div>
      {#if !charCollapse}
        <div class="grid grid-cols-[repeat(auto-fill,minmax(220px,1fr))] gap-2">
          {#if chars.length === 0}
            <p class="text-xs text-base-content/40 col-span-full py-2">暂无角色，点击下方按钮创建。</p>
          {:else}
            {#each chars as c}
              <div class="flex items-start gap-2.5 bg-base-300 rounded-lg p-2.5 group">
                <div class="w-8 h-8 rounded-full bg-primary/20 text-primary flex items-center justify-center text-xs font-bold shrink-0">{c.name[0]}</div>
                <div class="flex-1 min-w-0">
                  <div class="text-sm font-medium truncate">{c.name}</div>
                  <div class="text-xs text-base-content/40 line-clamp-1">{c.personality || c.background || c.age || ''}</div>
                </div>
                <div class="flex gap-0.5 opacity-0 group-hover:opacity-100 transition-opacity shrink-0">
                  <button class="btn btn-ghost btn-xs px-1" on:click={() => openCharForm(c)} disabled={$taskRunning}>编辑</button>
                  <button class="btn btn-ghost btn-xs px-1 text-error" on:click={() => deleteCharacter(c.id)} disabled={$taskRunning}>删除</button>
                </div>
              </div>
            {/each}
          {/if}
        </div>

        {#if showCharForm}
          <div class="bg-base-300 rounded-lg p-3 space-y-2 mt-1">
            <div class="grid grid-cols-2 gap-x-3 gap-y-1.5">
              <div>
                <label class="text-xs text-base-content/50 mb-0.5 block">名称</label>
                <input type="text" class="input input-sm w-full" bind:value={charName} />
              </div>
              <div>
                <label class="text-xs text-base-content/50 mb-0.5 block">年龄</label>
                <input type="text" class="input input-sm w-full" bind:value={charAge} />
              </div>
            </div>
            <div class="grid grid-cols-2 gap-x-3 gap-y-1.5">
              <div>
                <label class="text-xs text-base-content/50 mb-0.5 block">外貌</label>
                <textarea class="textarea textarea-sm w-full h-14 text-sm" bind:value={charAppearance}></textarea>
              </div>
              <div>
                <label class="text-xs text-base-content/50 mb-0.5 block">性格</label>
                <textarea class="textarea textarea-sm w-full h-14 text-sm" bind:value={charPersonality}></textarea>
              </div>
              <div>
                <label class="text-xs text-base-content/50 mb-0.5 block">背景</label>
                <textarea class="textarea textarea-sm w-full h-14 text-sm" bind:value={charBackground}></textarea>
              </div>
              <div>
                <label class="text-xs text-base-content/50 mb-0.5 block">动机</label>
                <textarea class="textarea textarea-sm w-full h-14 text-sm" bind:value={charMotivation}></textarea>
              </div>
              <div>
                <label class="text-xs text-base-content/50 mb-0.5 block">能力</label>
                <textarea class="textarea textarea-sm w-full h-14 text-sm" bind:value={charAbilities}></textarea>
              </div>
              <div>
                <label class="text-xs text-base-content/50 mb-0.5 block">备注</label>
                <textarea class="textarea textarea-sm w-full h-14 text-sm" bind:value={charNotes}></textarea>
              </div>
            </div>
            <div class="flex gap-1.5">
              <button class="btn btn-success btn-xs" on:click={saveCharacter} disabled={$taskRunning}>保存角色</button>
              <button class="btn btn-ghost btn-xs" on:click={closeCharForm}>取消</button>
            </div>
          </div>
        {/if}

        <div class="flex gap-1.5">
          <button class="btn btn-primary btn-xs" on:click={() => openCharForm(null)} disabled={$taskRunning}>新建角色</button>
          {#if chars.length > 0}
            <button class="btn btn-accent btn-xs" on:click={submitCharacters} disabled={$taskRunning}>提交角色设定给 AI</button>
          {/if}
        </div>
      {/if}
    </div>
  </div>

  <!-- Worldview -->
  <div class="card bg-base-200 shadow-sm">
    <div class="card-body p-4 gap-2">
      <!-- svelte-ignore a11y-click-events-have-key-events -->
      <!-- svelte-ignore a11y-no-static-element-interactions -->
      <div class="flex justify-between items-center cursor-pointer select-none" on:click={() => wvCollapse = !wvCollapse}>
        <h3 class="card-title text-base">世界观管理 <span class="text-xs font-normal text-base-content/40">({filteredWvs.length})</span></h3>
        <svg class="w-4 h-4 text-base-content/40 transition-transform" class:rotate-180={wvCollapse} viewBox="0 0 20 20" fill="currentColor"><path fill-rule="evenodd" d="M5.23 7.21a.75.75 0 011.06.02L10 11.168l3.71-3.938a.75.75 0 111.08 1.04l-4.25 4.5a.75.75 0 01-1.08 0l-4.25-4.5a.75.75 0 01.02-1.06z" clip-rule="evenodd"/></svg>
      </div>
      {#if !wvCollapse}
        <div class="tabs tabs-box tabs-xs bg-base-300 w-fit">
          {#each wvTabs as [cat, label]}
            <button class="tab tab-xs {$wvFilter === cat ? 'tab-active' : ''}" on:click={() => wvFilter.set(cat)}>
              {label}
            </button>
          {/each}
        </div>

        <div class="grid grid-cols-[repeat(auto-fill,minmax(220px,1fr))] gap-2">
          {#if filteredWvs.length === 0}
            <p class="text-xs text-base-content/40 col-span-full py-2">暂无世界观条目。</p>
          {:else}
            {#each filteredWvs as w}
              <div class="flex items-start gap-2.5 bg-base-300 rounded-lg p-2.5 group">
                <div class="w-8 h-8 rounded-lg bg-accent/20 text-accent flex items-center justify-center text-xs font-bold shrink-0">{w.name[0]}</div>
                <div class="flex-1 min-w-0">
                  <div class="text-sm font-medium truncate">{w.name} <span class="text-xs font-normal text-base-content/30">[{catLabels[w.category] || w.category}]</span></div>
                  <div class="text-xs text-base-content/40 line-clamp-1">{w.description}</div>
                </div>
                <div class="flex gap-0.5 opacity-0 group-hover:opacity-100 transition-opacity shrink-0">
                  <button class="btn btn-ghost btn-xs px-1" on:click={() => openWvForm(w)} disabled={$taskRunning}>编辑</button>
                  <button class="btn btn-ghost btn-xs px-1 text-error" on:click={() => deleteWorldview(w.id)} disabled={$taskRunning}>删除</button>
                </div>
              </div>
            {/each}
          {/if}
        </div>

        {#if showWvForm}
          <div class="bg-base-300 rounded-lg p-3 space-y-2 mt-1">
            <div class="grid grid-cols-2 gap-x-3 gap-y-1.5">
              <div>
                <label class="text-xs text-base-content/50 mb-0.5 block">名称</label>
                <input type="text" class="input input-sm w-full" bind:value={wvName} />
              </div>
              <div>
                <label class="text-xs text-base-content/50 mb-0.5 block">分类</label>
                <select class="select select-sm w-full" bind:value={wvCategory}>
                  <option value="geography">地理</option>
                  <option value="faction">势力</option>
                  <option value="rule">规则</option>
                  <option value="history">历史</option>
                  <option value="other">其他</option>
                </select>
              </div>
            </div>
            <div>
              <label class="text-xs text-base-content/50 mb-0.5 block">描述</label>
              <textarea class="textarea textarea-sm w-full h-16 text-sm" bind:value={wvDescription}></textarea>
            </div>
            <div>
              <label class="text-xs text-base-content/50 mb-0.5 block">标签</label>
              <input type="text" class="input input-sm w-full" bind:value={wvTags} placeholder="逗号分隔" />
            </div>
            <div class="flex gap-1.5">
              <button class="btn btn-success btn-xs" on:click={saveWorldview} disabled={$taskRunning}>保存</button>
              <button class="btn btn-ghost btn-xs" on:click={closeWvForm}>取消</button>
            </div>
          </div>
        {/if}

        <div class="flex gap-1.5">
          <button class="btn btn-primary btn-xs" on:click={() => openWvForm(null)} disabled={$taskRunning}>新建世界观条目</button>
          {#if allWvs.length > 0}
            <button class="btn btn-accent btn-xs" on:click={submitWorldview} disabled={$taskRunning}>提交世界观设定给 AI</button>
          {/if}
        </div>
      {/if}
    </div>
  </div>
</div>
