<script>
  import { onMount } from 'svelte';
  import { api } from '../lib/api.js';
  import { skills, addToast } from '../lib/stores.js';

  onMount(async () => {
    try { skills.set(await api('GET', '/api/skills')); } catch (e) {}
  });

  async function toggleSkill(id, enabled) {
    try {
      await api('PUT', '/api/skills/' + id + '/toggle', { enabled });
      addToast(enabled ? '技能已启用' : '技能已禁用', 'success');
      skills.set(await api('GET', '/api/skills'));
    } catch (e) { addToast(e.message, 'error'); }
  }
</script>

<div class="card bg-base-200 shadow-sm">
  <div class="card-body">
    <h2 class="card-title">技能管理</h2>
    <p class="text-xs text-base-content/60 mb-3">技能是可选的创作辅助工具。启用后，全局助理会参考这些技能，去AI味功能需要启用 polish 类技能。</p>
    <div class="overflow-x-auto">
      <table class="table table-sm">
        <thead>
          <tr>
            <th>名称</th>
            <th>分类</th>
            <th>描述</th>
            <th>来源</th>
            <th>启用</th>
          </tr>
        </thead>
        <tbody>
          {#if $skills.length === 0}
            <tr><td colspan="5" class="text-center text-base-content/50 py-8">暂无可用技能</td></tr>
          {:else}
            {#each $skills as sv}
              <tr>
                <td class="font-medium">{sv.skill.name}</td>
                <td>{sv.skill.category}</td>
                <td class="text-base-content/60 max-w-md truncate">{sv.skill.description}</td>
                <td>{sv.skill.source === 'builtin' ? '内置' : '项目'}</td>
                <td>
                  <input
                    type="checkbox"
                    class="toggle toggle-primary toggle-sm"
                    checked={sv.enabled}
                    on:change={e => toggleSkill(sv.skill.id, e.target.checked)}
                  />
                </td>
              </tr>
            {/each}
          {/if}
        </tbody>
      </table>
    </div>
  </div>
</div>
