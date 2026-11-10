import { expect, test } from '@playwright/test'
import { spawn, execFileSync } from 'node:child_process'
import type { ChildProcess } from 'node:child_process'
import { createHmac, randomBytes } from 'node:crypto'
import { once } from 'node:events'
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { resolve, join } from 'node:path'
import { z } from 'zod'

function authenticatorCode(key: string): string {
  const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567'
  const bits = key
    .split('')
    .map((c) => alphabet.indexOf(c).toString(2).padStart(5, '0'))
    .join('')
  const secret = Buffer.from((bits.match(/.{8}/g) ?? []).map((v) => parseInt(v, 2)))
  const counter = Buffer.alloc(8)
  counter.writeBigUInt64BE(BigInt(Math.floor(Date.now() / 30_000)))
  const digest = createHmac('sha1', secret).update(counter).digest()
  const last = digest.at(-1)
  if (last === undefined) throw new Error('Missing HMAC result')
  return String((digest.readUInt32BE(last & 15) & 0x7fffffff) % 1_000_000).padStart(6, '0')
}

async function stop(child: ChildProcess | undefined): Promise<void> {
  if (child?.exitCode !== null || child.signalCode !== null) return
  const exited = once(child, 'exit', { signal: AbortSignal.timeout(10_000) })
  child.kill('SIGTERM')
  await exited
}

test('SQLite retains browser setup and consumed recovery codes through restart and backup restore', async ({
  page,
  context,
  request,
}) => {
  const directory = mkdtempSync(join(tmpdir(), 'console-sqlite-browser-'))
  const binary = resolve('../bin/console')
  const keyFile = join(directory, 'key')
  const tokenFile = join(directory, 'setup-token')
  const certificate = join(directory, 'tls.crt')
  const tlsKey = join(directory, 'tls.key')
  const source = join(directory, 'source.db')
  const backup = join(directory, 'restored.db')
  const setupToken = randomBytes(32).toString('hex')
  const password = 'sqlite durable browser credential 42'
  writeFileSync(keyFile, randomBytes(32).toString('hex'), { mode: 0o600 })
  writeFileSync(tokenFile, setupToken, { mode: 0o600 })
  execFileSync(
    'openssl',
    [
      'req',
      '-x509',
      '-newkey',
      'rsa:2048',
      '-nodes',
      '-days',
      '1',
      '-subj',
      '/CN=127.0.0.1',
      '-addext',
      'subjectAltName=IP:127.0.0.1',
      '-keyout',
      tlsKey,
      '-out',
      certificate,
    ],
    { stdio: 'ignore' },
  )
  const environment = {
    ...process.env,
    CONSOLE_STORAGE: 'sqlite',
    CONSOLE_OIDC_ISSUER: '',
    CONSOLE_OIDC_CLIENT_ID: '',
    CONSOLE_OIDC_CLIENT_SECRET: '',
    CONSOLE_OIDC_CA_FILE: '',
    CONSOLE_BOOTSTRAP_ADMIN_SUBJECTS: '',
    CONSOLE_TRUSTED_PROXIES: '',
    CONSOLE_HISTORY_RETENTION: '168h',
    CONSOLE_DATABASE_URL: '',
    CONSOLE_SQLITE_PATH: source,
    CONSOLE_CONFIG_KEY_FILE: keyFile,
    CONSOLE_SETUP_TOKEN_FILE: tokenFile,
    CONSOLE_OIDC_REDIRECT_URL: 'https://127.0.0.1:5178/auth/callback',
  }
  let child: ChildProcess | undefined
  const start = async (path: string) => {
    child = spawn(
      binary,
      [
        '--mode=live',
        '--listen=127.0.0.1:5178',
        `--tls-cert=${certificate}`,
        `--tls-key=${tlsKey}`,
      ],
      { env: { ...environment, CONSOLE_SQLITE_PATH: path }, stdio: 'ignore' },
    )
    await expect
      .poll(async () => {
        try {
          return (await request.get('/readyz')).status()
        } catch {
          return 0
        }
      })
      .toBe(204)
  }
  try {
    await start(source)
    await page.goto('/setup')
    await page.getByLabel('Setup token', { exact: true }).fill(setupToken)
    await page.getByRole('button', { name: 'Unlock setup', exact: true }).click()
    await page.getByRole('radio', { name: /Local administrator/ }).check()
    await page.getByLabel(/^Username/).fill('admin')
    await page.getByLabel(/^Display name/).fill('SQLite administrator')
    await page.getByLabel(/^Password/).fill(password)
    await page.getByLabel('Confirm password', { exact: true }).fill(password)
    const authenticator = page.waitForResponse('**/api/v1/setup/local/authenticator')
    await page.getByRole('button', { name: 'Add an authenticator app' }).click()
    const { key } = z.object({ key: z.string() }).parse(await (await authenticator).json())
    await page.getByLabel(/^Authenticator code/).fill(authenticatorCode(key))
    await page.getByRole('button', { name: 'Create administrator' }).click()
    await expect(page.getByRole('heading', { name: 'Save your recovery codes' })).toBeVisible()
    const codes = (
      await page.getByRole('region', { name: 'Recovery codes', exact: true }).innerText()
    ).match(/[A-Z2-7]{4}(?:-[A-Z2-7]{4}){3}/g)
    const [first, second] = z.array(z.string()).min(2).parse(codes)
    if (!first || !second) throw new Error('Missing recovery codes')
    await stop(child)
    await start(source)
    await context.clearCookies()
    await page.goto('/fleet')
    await expect(page.getByLabel('Setup token', { exact: true })).toHaveCount(0)
    const form = page.getByRole('form', { name: 'Sign in with a local account' })
    const signIn = async (code: string) => {
      await form.getByLabel(/^Username/).fill('admin')
      await form.getByLabel(/^Password/).fill(password)
      await form.getByLabel(/^Authenticator code/).fill(code)
      await form.getByRole('button', { name: 'Sign in' }).click()
    }
    await signIn(first)
    await page.getByRole('button', { name: 'Continue', exact: true }).click()
    await expect(page.getByRole('navigation', { name: 'Main navigation' })).toBeVisible()
    await stop(child)
    execFileSync(binary, ['--mode=live', '--schema=backup', `--backup=${backup}`], {
      env: environment,
    })
    expect(readFileSync(backup).includes(Buffer.from(password))).toBe(false)
    expect(readFileSync(backup).includes(Buffer.from(key))).toBe(false)
    await start(backup)
    await context.clearCookies()
    await page.goto('/fleet')
    await signIn(first)
    await expect(form.getByRole('alert')).toHaveText(
      'The username, password, or code is incorrect.',
    )
    await signIn(second)
    await page.getByRole('button', { name: 'Continue', exact: true }).click()
    await expect(page.getByRole('navigation', { name: 'Main navigation' })).toBeVisible()
  } finally {
    await stop(child)
    rmSync(directory, { recursive: true, force: true })
  }
})
