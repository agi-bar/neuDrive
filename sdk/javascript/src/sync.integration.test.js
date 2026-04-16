const test = require('node:test')
const assert = require('node:assert/strict')
const crypto = require('node:crypto')
const fs = require('node:fs')
const os = require('node:os')
const path = require('node:path')
const { execFileSync } = require('node:child_process')
const { unzipSync, strFromU8 } = require('fflate')

const BASE_URL = (process.env.NEUDRIVE_TEST_URL || '').replace(/\/+$/, '')
const DEV_SLUG_CANDIDATES = (process.env.NEUDRIVE_TEST_DEV_SLUGS || 'demo,de,admin')
  .split(',')
  .map((value) => value.trim())
  .filter(Boolean)
const ROOT = path.resolve(__dirname, '../../..')
const FIXTURE_DIR = path.join(ROOT, 'internal', 'services', 'testdata')

function runNeuDrive(args) {
  const configured = process.env.NEUDRIVE_CLI
  if (configured) {
    execFileSync(configured, args, { stdio: 'inherit' })
    return
  }
  if (fs.existsSync('/tmp/neudrive')) {
    execFileSync('/tmp/neudrive', args, { stdio: 'inherit' })
    return
  }
  execFileSync('go', ['run', './cmd/neudrive', ...args], { cwd: ROOT, stdio: 'inherit' })
}

function skipIfNoServer() {
  if (!BASE_URL) {
    return true
  }
  return false
}

function loadSkillFixture() {
  const archive = unzipSync(new Uint8Array(fs.readFileSync(path.join(FIXTURE_DIR, 'ahub-sync.skill'))))
  const files = {}
  for (const [name, data] of Object.entries(archive)) {
    if (!name.startsWith('pkg-skill/') || name.endsWith('/')) continue
    files[name.replace(/^pkg-skill\//, '')] = strFromU8(data)
  }
  return files
}

function loadPlan() {
  return JSON.parse(fs.readFileSync(path.join(FIXTURE_DIR, 'sync-fixture-plan.json'), 'utf8'))
}

function expandedBinary(base, seed, multiplier) {
  const targetSize = base.length + Math.max(multiplier, 1) * (256 << 10)
  const chunks = [base]
  let size = base.length
  let counter = 0
  while (size < targetSize) {
    const chunk = crypto.createHash('sha256').update(`${seed}:${counter}`).digest()
    chunks.push(chunk)
    size += chunk.length
    counter += 1
  }
  return Buffer.concat(chunks).subarray(0, targetSize)
}

function materializeSource(multiplier = 1) {
  const tmpRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'neudrive-js-sync-'))
  const files = loadSkillFixture()
  const plan = loadPlan()
  const binary = fs.readFileSync(path.join(FIXTURE_DIR, 'tiny.png'))

  for (const skillName of plan.skill_names) {
    const skillRoot = path.join(tmpRoot, skillName)
    fs.mkdirSync(skillRoot, { recursive: true })
    for (const [relPath, content] of Object.entries(files)) {
      const target = path.join(skillRoot, relPath)
      fs.mkdirSync(path.dirname(target), { recursive: true })
      fs.writeFileSync(target, content, 'utf8')
    }
    for (const extra of plan.extra_text_files) {
      const target = path.join(skillRoot, extra.path)
      fs.mkdirSync(path.dirname(target), { recursive: true })
      fs.writeFileSync(target, (files[extra.source] + '\n').repeat(extra.repeat * Math.max(multiplier, 1)), 'utf8')
    }
    for (const relPath of plan.binary_assignments[skillName] || []) {
      const target = path.join(skillRoot, relPath)
      fs.mkdirSync(path.dirname(target), { recursive: true })
      fs.writeFileSync(target, expandedBinary(binary, `${skillName}:${relPath}`, multiplier))
    }
  }
  return tmpRoot
}

async function registerUser() {
  const slug = `js-sync-${Date.now()}`
  const email = `${slug}@test.local`
  const password = 'neudrive-sync-1234'
  const registerRes = await fetch(`${BASE_URL}/api/auth/register`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ slug, email, password }),
  })
  let jwtToken
  if (registerRes.status !== 404) {
    assert.equal(registerRes.status, 201)
    const registerBody = await registerRes.json()
    jwtToken = registerBody.access_token
  } else {
    let devTokenBody
    let lastStatus = 0
    for (const candidate of DEV_SLUG_CANDIDATES) {
      const devTokenRes = await fetch(`${BASE_URL}/api/auth/token/dev`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ slug: candidate }),
      })
      lastStatus = devTokenRes.status
      if (devTokenRes.status === 404) continue
      assert.equal(devTokenRes.status, 200)
      devTokenBody = await devTokenRes.json()
      break
    }
    assert.ok(devTokenBody, `expected one of ${DEV_SLUG_CANDIDATES.join(', ')} to exist, last status ${lastStatus}`)
    jwtToken = devTokenBody.token
  }

  const tokenRes = await fetch(`${BASE_URL}/api/tokens`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${jwtToken}`,
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({
      name: 'js-sync-test',
      scopes: ['read:bundle', 'write:bundle'],
      max_trust_level: 3,
      expires_in_days: 1,
    }),
  })
  assert.equal(tokenRes.status, 201)
  const tokenBody = await tokenRes.json()
  return tokenBody.data ? tokenBody.data.token : tokenBody.token
}

function readManifestFromArchive(archivePath) {
  const files = unzipSync(new Uint8Array(fs.readFileSync(archivePath)))
  return JSON.parse(strFromU8(files['manifest.json']))
}

test('NeuDrive JS SDK handles json bundle and archive sessions', { skip: skipIfNoServer() }, async () => {
  const { NeuDrive } = require('../dist/index.js')
  const token = await registerUser()
  const sourceDir = materializeSource(2)
  const bundlePath = path.join(os.tmpdir(), `neudrive-js-${Date.now()}.ndrv`)
  const archivePath = path.join(os.tmpdir(), `neudrive-js-${Date.now()}.ndrvz`)

  runNeuDrive(['sync', 'export', '--source', sourceDir, '-o', bundlePath])
  runNeuDrive(['sync', 'export', '--source', sourceDir, '--format', 'archive', '-o', archivePath])

  const bundle = JSON.parse(fs.readFileSync(bundlePath, 'utf8'))
  const manifest = readManifestFromArchive(archivePath)
  const archiveBytes = new Uint8Array(fs.readFileSync(archivePath))
  const hub = new NeuDrive({ baseURL: BASE_URL, token })

  const authInfo = await hub.getAuthInfo()
  assert.equal(authInfo.api_base, BASE_URL)
  assert.deepEqual(authInfo.scopes, ['read:bundle', 'write:bundle'])

  const preview = await hub.previewBundle(bundle)
  assert.ok(preview.fingerprint)

  const imported = await hub.importBundle(bundle)
  assert.ok(imported.files_written > 0)

  const exported = await hub.exportBundle('json')
  assert.ok(exported.skills)

  const session = await hub.startSyncSession({
    transport_version: 'ahub.sync/v1',
    format: 'archive',
    mode: 'merge',
    manifest,
    archive_size_bytes: archiveBytes.length,
    archive_sha256: manifest.archive_sha256,
  })
  const resumed = await hub.resumeSession(session.session_id, archiveBytes)
  assert.ok(['ready', 'uploading'].includes(resumed.status))
  const committed = await hub.commitSession(session.session_id)
  assert.ok(committed.files_written > 0)

  const jobs = await hub.listSyncJobs()
  assert.ok(jobs.length >= 3)
})
