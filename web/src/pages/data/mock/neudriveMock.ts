import type { FileNode } from '../../../api'

// Helper to make ISO timestamps days ago
function daysAgo(n: number) {
  const d = new Date()
  d.setDate(d.getDate() - n)
  return d.toISOString()
}

export function buildNeuDriveMock(): FileNode[] {
  const rows: FileNode[] = []

  // Ensure top-level namespaces are visible
  const tops = ['projects', 'skills', 'memory', 'roles', 'inbox', 'notes']
  for (let i = 0; i < tops.length; i++) {
    const seg = tops[i]
    rows.push({
      path: `/${seg}`,
      name: seg,
      is_dir: true,
      updated_at: daysAgo(i + 1),
    })
  }

  // Projects alpha/bravo/charlie
  ;(['alpha', 'bravo', 'charlie'] as const).forEach((p, idx) => {
    rows.push({ path: `/projects/${p}`, name: p, is_dir: true, updated_at: daysAgo(idx) })
    rows.push({
      path: `/projects/${p}/context.md`,
      name: 'context.md',
      is_dir: false,
      kind: 'project_context',
      mime_type: 'text/markdown',
      size: 2200 + idx * 30,
      updated_at: daysAgo(idx),
      content: `# ${p.toUpperCase()} 项目\n- 目标：对齐 OneDrive 范式\n- 范围：文件浏览/搜索/预览`,
    })
    rows.push({
      path: `/projects/${p}/log.jsonl`,
      name: 'log.jsonl',
      is_dir: false,
      kind: 'project_log',
      mime_type: 'application/json',
      size: 5200 + idx * 100,
      updated_at: daysAgo(idx + 1),
      content: `{"ts":"${daysAgo(idx)}","msg":"init"}\n`,
    })
    rows.push({
      path: `/projects/${p}/notes.md`,
      name: 'notes.md',
      is_dir: false,
      mime_type: 'text/markdown',
      size: 900 + idx * 50,
      updated_at: daysAgo(idx + 2),
      content: `- [ ] ${p} TODO A\n- [ ] ${p} TODO B`,
    })
    rows.push({ path: `/projects/${p}/assets`, name: 'assets', is_dir: true, updated_at: daysAgo(idx + 2) })
    rows.push({
      path: `/projects/${p}/assets/spec.txt`,
      name: 'spec.txt',
      is_dir: false,
      mime_type: 'text/plain',
      size: 600,
      updated_at: daysAgo(idx + 3),
      content: `Spec of ${p}`,
    })
  })

  // Skills bundles
  ;([
    { bundle: 'qa-bot', name: 'QA Bot', ro: false },
    { bundle: 'translator', name: 'Translator', ro: true },
    { bundle: 'planner', name: 'Task Planner', ro: false },
  ]).forEach((s, idx) => {
    const base = `/skills/${s.bundle}`
    rows.push({ path: base, name: s.bundle, is_dir: true, updated_at: daysAgo(1 + idx) })
    rows.push({
      path: `${base}/SKILL.md`,
      name: 'SKILL.md',
      is_dir: false,
      kind: 'skill',
      mime_type: 'text/markdown',
      size: 3200 + idx * 120,
      updated_at: daysAgo(1 + idx),
      content: `---\nname: ${s.name}\n---\n# ${s.name}\n说明与使用…`,
      metadata: s.ro ? { read_only: true } : undefined,
    })
    rows.push({
      path: `${base}/assets`,
      name: 'assets',
      is_dir: true,
      updated_at: daysAgo(2 + idx),
    })
    rows.push({
      path: `${base}/assets/prompt.md`,
      name: 'prompt.md',
      is_dir: false,
      mime_type: 'text/markdown',
      size: 1200 + idx * 40,
      updated_at: daysAgo(2 + idx),
      content: 'You are a helpful assistant…',
      metadata: s.ro ? { read_only: true } : undefined,
    })
  })

  // Memory
  rows.push({ path: '/memory/profile', name: 'profile', is_dir: true, updated_at: daysAgo(2) })
  rows.push({
    path: '/memory/profile/display_name.md',
    name: 'display_name.md',
    is_dir: false,
    kind: 'memory_profile',
    mime_type: 'text/markdown',
    size: 40,
    updated_at: daysAgo(2),
    content: 'neuDrive User',
  })
  rows.push({ path: '/memory/scratch', name: 'scratch', is_dir: true, updated_at: daysAgo(3) })
  for (let i = 1; i <= 8; i++) {
    rows.push({
      path: `/memory/scratch/note-${i}.md`,
      name: `note-${i}.md`,
      is_dir: false,
      kind: 'memory_scratch',
      mime_type: 'text/markdown',
      size: 150 + i * 5,
      updated_at: daysAgo(3 + Math.floor(i / 3)),
      content: `快速笔记 ${i}\n- 事项 A\n- 事项 B`,
    })
  }

  // Notes & root
  if (!rows.find(n => n.path === '/notes')) {
    rows.push({ path: '/notes', name: 'notes', is_dir: true, updated_at: daysAgo(0) })
  }
  rows.push({
    path: '/notes/todo-1.md',
    name: 'todo-1.md',
    is_dir: false,
    mime_type: 'text/markdown',
    size: 280,
    updated_at: daysAgo(0),
    content: '- [ ] 任务 1',
  })
  ;(['readme.md', 'changelog.md', 'roadmap.md'] as const).forEach((f, idx) => {
    rows.push({
      path: `/${f}`,
      name: f,
      is_dir: false,
      mime_type: 'text/markdown',
      size: 800 + idx * 200,
      updated_at: daysAgo(7 + idx),
      content: `# ${f}`,
    })
  })

  return rows
}

export function listChildren(all: FileNode[], dir: string): FileNode[] {
  const base = dir === '/' ? '/' : (dir.endsWith('/') ? dir.slice(0, -1) : dir)
  const out = all.filter((n) => {
    const parent = n.path.slice(0, n.path.lastIndexOf('/')) || '/'
    return parent === base
  })
  return out.sort((a, b) => (a.is_dir === b.is_dir ? a.name.localeCompare(b.name) : a.is_dir ? -1 : 1))
}
