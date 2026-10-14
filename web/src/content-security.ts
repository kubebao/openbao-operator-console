import { setNonce } from 'get-nonce'
import { z } from 'zod'

// The console runs under a content security policy without 'unsafe-eval' or inline styles.
// main.tsx imports this module before any other application module.

// Zod otherwise probes for eval support when it builds an object schema, and the browser
// reports the blocked probe as a policy violation.
z.config({ jitless: true })

// Style elements need the nonce of this response. Dialogs inject their scroll lock styles
// with it.
const nonce = document.querySelector('meta[name="csp-nonce"]')
if (nonce instanceof HTMLMetaElement && nonce.content) setNonce(nonce.content)
