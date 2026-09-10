import { chromium } from 'playwright';
import { spawn } from 'node:child_process';
import { mkdir, rm, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const frontendDir = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const outputDir = path.resolve(frontendDir, '../docs/screenshots');
const baseURL = 'http://127.0.0.1:4173';

const stories = {
  en: {
    title: 'The Cartographer of Rain',
    core: 'A literary mystery about memory, maps, and a city that quietly rewrites itself.',
    direction: 'Reveal the map’s origin while forcing Mara to decide which memories deserve to remain.',
    synopsis: 'Mara inherits a map shop whose charts change whenever the rain begins.',
    chapters: [
      ['The Map That Remembered', 'Mara discovers that the city map redraws itself after midnight.', 'accepted'],
      ['A Door Below the River', 'Following the new street, Mara finds a sealed archive beneath the old quay.', 'accepted'],
      ['The Cartographer’s Debt', 'Elias admits why every altered map erases one of his memories.', 'review'],
      ['Ink Without a Shadow', 'The pair trace the counterfeit ink to the royal observatory.', 'pending'],
      ['The City Turns North', 'Mara must choose which version of the city will survive dawn.', 'pending'],
    ],
    summary: 'Mara confronts Elias in the archive and learns what the living map costs.',
    blocks: [
      'Rain ticked against the archive windows.',
      'Mara spread the living map across the oak table. A pale street was appearing where the river should have been.',
      '“You knew it would take something from you,” she said.',
      'Elias watched his own handwriting fade from the margin. “I knew the price. I did not know what it would choose.”',
    ],
    characters: ['Mara Vale', 'Elias Rook'],
    foreshadow: 'The missing north gate',
    fact: 'The living map changes only during rain.',
    chat: 'I can help review this chapter, trace a continuity detail, or plan what comes next.',
  },
  zh: {
    title: '雨之绘图师',
    core: '一部关于记忆、地图，以及一座悄然改写自身的城市的文学幻想悬疑小说。',
    direction: '揭开活地图的起源，并迫使玛拉决定哪些记忆值得留下。',
    synopsis: '玛拉继承了一间地图铺，每逢下雨，店里的地图就会自行改变。',
    chapters: [
      ['会记忆的地图', '玛拉发现城市地图会在午夜之后重新绘制自己。', 'accepted'],
      ['河流之下的门', '沿着新出现的街道，玛拉在旧码头下找到一座封闭档案馆。', 'accepted'],
      ['绘图师的债', '伊莱亚斯坦白：每次修改地图，都会抹去他的一段记忆。', 'review'],
      ['没有影子的墨水', '两人追踪伪造墨水的来源，线索指向皇家天文台。', 'pending'],
      ['城市转向北方', '黎明之前，玛拉必须决定让哪一个版本的城市存续。', 'pending'],
    ],
    summary: '玛拉在档案馆质问伊莱亚斯，并得知活地图索取的真正代价。',
    blocks: [
      '雨点轻敲着档案馆的窗。',
      '玛拉把活地图铺在橡木桌上。河流原本所在的位置，一条苍白的街道正缓缓浮现。',
      '“你早就知道它会从你身上拿走什么。”她说。',
      '伊莱亚斯看着自己写在页边的字迹逐渐消失。“我知道要价，却不知道它会选中哪一段记忆。”',
    ],
    characters: ['玛拉·维尔', '伊莱亚斯·鲁克'],
    foreshadow: '消失的北门',
    fact: '活地图只在下雨时发生变化。',
    chat: '我可以帮你检查本章、追踪前后文细节，或规划接下来的情节。',
  },
};

function makeData(lang) {
  const story = stories[lang];
  const chapters = story.chapters.map(([title, outline, status], i) => ({
    num: i + 1, title, outline, status,
    summary: i === 2 ? story.summary : '',
    word_count: status === 'pending' ? 0 : 1840 + i * 93,
    content_rev: status === 'pending' ? '' : `rev-${i + 1}`,
    characters: story.characters.map((name, j) => ({ name, first_appearance: i === 0 && j === 1 })),
  }));
  const progress = {
    phase: 'writing', title: story.title, core_prompt: story.core,
    current_chapter_index: 2, book_status: 'active', long_term_direction: story.direction,
    chapters,
    outline_batches: [{ id: 1, start_ch: 1, end_ch: 5, synopsis: story.synopsis, revision: 1 }],
    foreshadows: [{ id: 1, name: story.foreshadow, description: story.fact, plant_chapter: 1, target_chapter: 5, status: 'progressing', events: [] }],
  };
  const chapter = {
    ...chapters[2], content_rev: 'rev-3', content: story.blocks.join('\n\n'),
    blocks: story.blocks.map((text, i) => ({ id: i + 1, type: 'paragraph', text })),
  };
  return { story, chapters, progress, chapter };
}

const json = (route, value) => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(value) });

async function waitForServer() {
  for (let i = 0; i < 50; i++) {
    try { if ((await fetch(baseURL)).ok) return; } catch {}
    await new Promise(resolve => setTimeout(resolve, 100));
  }
  throw new Error('Vite preview did not start');
}

async function saveWebP(page, file) {
  const png = await page.screenshot({ type: 'png', animations: 'disabled' });
  const dataURL = await page.evaluate(async base64 => {
    const image = new Image();
    image.src = `data:image/png;base64,${base64}`;
    await image.decode();
    const canvas = Object.assign(document.createElement('canvas'), { width: image.width, height: image.height });
    canvas.getContext('2d').drawImage(image, 0, 0);
    return canvas.toDataURL('image/webp', 0.86);
  }, png.toString('base64'));
  await writeFile(file, Buffer.from(dataURL.split(',')[1], 'base64'));
}

const server = spawn(process.execPath, ['./node_modules/vite/bin/vite.js', 'preview', '--host', '127.0.0.1', '--port', '4173'], { cwd: frontendDir, stdio: 'inherit' });
let browser;
try {
  await waitForServer();
  browser = await chromium.launch({ channel: 'chrome', headless: true });
  for (const lang of Object.keys(stories)) {
    const { story, chapters, progress, chapter } = makeData(lang);
    const localeDir = path.join(outputDir, lang);
    await mkdir(localeDir, { recursive: true });
    const page = await browser.newPage({ viewport: { width: 1600, height: 1000 }, deviceScaleFactor: 1 });
    await page.addInitScript(locale => localStorage.setItem('showmethestory.uiLocale', locale), lang);
    await page.route('**/api/events', route => route.abort());
    await page.route('**/api/**', route => {
      const pathname = new URL(route.request().url()).pathname;
      if (pathname === '/api/projects/current') return json(route, { name: 'rain-cartographer', language: lang });
      if (pathname === '/api/config') return json(route, { story: { title: story.title, genre: lang === 'en' ? 'Literary fantasy' : '文学幻想', style: lang === 'en' ? 'Atmospheric and precise' : '克制、富有氛围', pov: lang === 'en' ? 'Third person limited' : '第三人称限知' } });
      if (pathname === '/api/progress') return json(route, progress);
      if (pathname === '/api/settings') return json(route, { characters: [], worldview: [] });
      if (pathname === '/api/chat/sessions') return json(route, { sessions: [{ id: 'demo', title: lang === 'en' ? 'Story notes' : '故事笔记', updated_at: '2026-01-01T12:00:00Z' }] });
      if (pathname === '/api/chat/sessions/demo') return json(route, { id: 'demo', messages: [{ role: 'assistant', content: story.chat, timestamp: '2026-01-01T12:00:00Z' }] });
      if (pathname.startsWith('/api/chapters/')) return json(route, chapter);
      if (pathname === '/api/knowledge') return json(route, { facts: [{ id: 7, content: story.fact, references: [{ chapter: 3, block_id: 2, quote: chapter.blocks[1].text }] }] });
      if (pathname === '/api/autoconfirm') return json(route, { enabled: false });
      if (pathname === '/api/import/status') return json(route, { active: false });
      if (pathname === '/api/status') return json(route, { is_task_running: false });
      if (pathname === '/api/version') return json(route, { version: 'v4' });
      if (pathname === '/api/skills') return json(route, []);
      return json(route, {});
    });
    for (const pageName of ['writing', 'outline']) {
      await page.goto(`${baseURL}/#${pageName}`, { waitUntil: 'networkidle' });
      await page.getByText(story.title, { exact: true }).first().waitFor();
      if (pageName === 'writing') {
        await page.getByText(chapters[1].title, { exact: true }).locator('..').click();
        await page.getByText(chapters[2].title, { exact: true }).first().locator('..').click();
        await page.locator('#story-block-2').click();
      } else {
        await page.locator('[data-outline-chapter="5"]').waitFor();
      }
      await page.evaluate(() => document.fonts.ready);
      const appTitle = lang === 'en' ? 'AI Novel Generator' : 'AI 小说写手';
      const textRect = await page.getByText(appTitle, { exact: true }).evaluate(el => el.getBoundingClientRect().toJSON());
      if (textRect.width < 30 || textRect.height < 10) throw new Error(`Screenshot font rendering failed for ${lang}: ${JSON.stringify(textRect)}`);
      await page.evaluate(() => document.activeElement?.blur());
      await page.mouse.move(0, 0);
      await saveWebP(page, path.join(localeDir, `${pageName}.webp`));
    }
    await page.close();
  }
  await Promise.all(['writing.png', 'outline.png'].map(name => rm(path.join(outputDir, name), { force: true })));
} finally {
  await browser?.close();
  server.kill('SIGTERM');
}
