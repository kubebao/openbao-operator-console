import { Download } from 'lucide-react'
import type { ReaderAccessResult } from '../api/schema'
import { Field } from './form'
import { Button } from './ui/button'
import { CodeBlock } from './code-block'
import { downloadText } from '../lib/download'

function shellQuote(value: string): string {
  return `'${value.replaceAll("'", "'\"'\"'")}'`
}

function contextLine(context: string): string {
  return `CONSOLE_CLUSTER_CONTEXT=${shellQuote(context || 'your-context')}`
}

function applyCommands(access: ReaderAccessResult, context: string): string {
  const namespace = shellQuote(access.serviceAccountNamespace)
  return `${contextLine(context)}
kubectl --context="$CONSOLE_CLUSTER_CONTEXT" get namespace ${namespace}
# If this namespace does not exist, create it:
# kubectl --context="$CONSOLE_CLUSTER_CONTEXT" create namespace ${namespace}
kubectl --context="$CONSOLE_CLUSTER_CONTEXT" apply -f console-reader.yaml`
}

function kubeconfigCommands(access: ReaderAccessResult, context: string): string {
  const namespace = shellQuote(access.serviceAccountNamespace)
  const account = shellQuote(access.serviceAccount)
  return `${contextLine(context)}
# Write a new, private kubeconfig with a short-lived reader token.
(
set -eu
umask 077
CONSOLE_READER_TARGET="$PWD/console-reader.kubeconfig"
if [ -e "$CONSOLE_READER_TARGET" ]; then
  printf '%s\\n' 'console-reader.kubeconfig already exists. Move it before generating a new reader.' >&2
  exit 1
fi
CONSOLE_READER_DIR="$(mktemp -d "$PWD/.console-reader.XXXXXX")"
trap 'rm -rf "$CONSOLE_READER_DIR"' EXIT
CONSOLE_READER_FILE="$CONSOLE_READER_DIR/reader.kubeconfig"
CONSOLE_CA_FILE="$CONSOLE_READER_DIR/ca.crt"
CONSOLE_API_SERVER="$(kubectl --context="$CONSOLE_CLUSTER_CONTEXT" config view --minify -o jsonpath='{.clusters[0].cluster.server}')"
kubectl --context="$CONSOLE_CLUSTER_CONTEXT" config view --raw --minify --flatten -o jsonpath='{.clusters[0].cluster.certificate-authority-data}' | base64 --decode > "$CONSOLE_CA_FILE"
CONSOLE_READER_TOKEN="$(kubectl --context="$CONSOLE_CLUSTER_CONTEXT" -n ${namespace} create token ${account} --duration=24h)"
if [ -s "$CONSOLE_CA_FILE" ]; then
  kubectl --kubeconfig="$CONSOLE_READER_FILE" config set-cluster reader --server="$CONSOLE_API_SERVER" --certificate-authority="$CONSOLE_CA_FILE" --embed-certs=true
else
  kubectl --kubeconfig="$CONSOLE_READER_FILE" config set-cluster reader --server="$CONSOLE_API_SERVER"
fi
kubectl --kubeconfig="$CONSOLE_READER_FILE" config set-credentials reader --token="$CONSOLE_READER_TOKEN"
kubectl --kubeconfig="$CONSOLE_READER_FILE" config set-context reader --cluster=reader --user=reader
kubectl --kubeconfig="$CONSOLE_READER_FILE" config use-context reader
unset CONSOLE_READER_TOKEN
# ln refuses to replace an existing file, including one created during this command.
ln "$CONSOLE_READER_FILE" "$CONSOLE_READER_TARGET"
)`
}

export function OnboardingReader({
  access,
  local,
  context,
  onContext,
  onNotice,
  onError,
}: {
  access: ReaderAccessResult | null
  local: boolean
  context: string
  onContext: (context: string) => void
  onNotice: (message: string) => void
  onError: (message: string) => void
}) {
  return (
    <>
      <h2>Prepare the Kubernetes reader</h2>
      <p className="onboarding-help">
        Run these commands in a terminal with administrator access to the selected cluster.
      </p>
      {local && (
        <p className="onboarding-help">
          If the Helm values readerNamespaces and operatorReaders already grant these namespaces and
          the operator Deployment, skip the commands and continue. Verification checks the access.
        </p>
      )}
      {access ? (
        <>
          <Field
            label="kubectl context"
            help="Used in the commands below. Choose the context for this cluster."
          >
            <input
              value={context}
              onChange={(event) => {
                onContext(event.target.value)
              }}
              placeholder="your-context"
            />
          </Field>
          <ol className="reader-steps">
            <li>
              <div className="reader-step-heading">
                <h3>Apply the scoped reader RBAC</h3>
                <Button
                  type="button"
                  size="small"
                  onClick={() => {
                    downloadText(access.rbac, 'console-reader.yaml')
                    onNotice('Saved console-reader.yaml.')
                  }}
                >
                  <Download size={14} />
                  Download console-reader.yaml
                </Button>
              </div>
              <p className="onboarding-help">
                The manifests grant read access to the selected namespaces only.
              </p>
              <details className="manifest-details">
                <summary>Review manifests</summary>
                <CodeBlock code={access.rbac} label="Reader RBAC manifests" onCopyError={onError} />
              </details>
              <CodeBlock
                code={applyCommands(access, context)}
                label="Apply commands"
                onCopyError={onError}
              />
            </li>
            {!local && (
              <li>
                <div className="reader-step-heading">
                  <h3>Create the reader kubeconfig</h3>
                </div>
                <p className="onboarding-help">
                  Writes console-reader.kubeconfig with a short-lived token. The server may shorten
                  the requested 24-hour lifetime. Import this file in the next step.
                </p>
                <CodeBlock
                  code={kubeconfigCommands(access, context)}
                  label="Kubeconfig commands"
                  onCopyError={onError}
                />
              </li>
            )}
          </ol>
        </>
      ) : (
        <p>Return to Scope to generate reader instructions.</p>
      )}
    </>
  )
}
