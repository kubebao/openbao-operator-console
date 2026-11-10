# Evaluate on a local cluster

This guide runs the console against a real OpenBao Operator on your workstation. One command creates a disposable
[kind](https://kind.sigs.k8s.io/) cluster with:

- the [OpenBao Operator](https://github.com/kubebao/openbao-operator) 0.6.0 in single-tenant mode for the namespace
  `openbao-demo`;
- one Development `OpenBaoCluster` with one replica;
- persistent SQLite state on a PVC in the disposable cluster;
- the console from your checkout, with HTTPS and the cluster already connected.

No identity provider is needed: you create a local administrator during setup. Do not keep data in this environment.
For a deployment you keep, follow the [installation guide](installation.md).

## Requirements

Docker, kind, kubectl, Helm 3.x or 4, and OpenSSL. About 4 GB of free memory for Docker. Optional:
[mkcert](https://github.com/FiloSottile/mkcert) for a browser-trusted certificate.

The script touches only its own kind cluster, `console-evaluation`. It writes its kubeconfig,
encryption key, setup token, and certificate to `.local/evaluation` in your checkout, with private file permissions.

## Start

```sh
make evaluation
```

The first run builds the console image and pulls the operator and OpenBao images, which takes several minutes. Run it
again to rebuild the console from your checkout; the database and configuration stay.

Then forward the console to your workstation:

```sh
make evaluation-open
```

Open <https://localhost:8443>. Without mkcert, the certificate is self-signed and the browser asks you to accept it.

## Set up the console

1. Enter the setup token from `.local/evaluation/setup-token`.
2. Choose **Local administrator**. Enter a username and a password of at least 15 characters, add an authenticator
   app, and save the recovery codes.
3. The console opens with the evaluation cluster already connected. The fleet shows the OpenBao deployment
   `evaluation` in `openbao-demo` after the first collection, within 30 seconds.

To try the guided onboarding, add another namespace or cluster in **Settings → Clusters → Add cluster**. To try an
identity provider, see [Keycloak](keycloak.md) and **Settings → Identity provider**.

## Explore

Some ideas that show how the console interprets operator evidence:

- Scale the OpenBao cluster and watch the replica findings and the history:

  ```sh
  export KUBECONFIG=$PWD/.local/evaluation/kubeconfig
  kubectl --namespace openbao-demo patch openbaocluster evaluation --type=merge --patch '{"spec":{"replicas":3}}'
  ```

- Stop the operator controller and watch its health and the collection evidence change:

  ```sh
  kubectl --namespace openbao-operator-system scale deployment/openbao-operator-controller --replicas=0
  ```

- Add a second local account in **Settings → Local accounts** and open its enrollment link in a private window.

## Remove

```sh
make evaluation-down
```

This deletes the kind cluster and `.local/evaluation`.

## Troubleshooting

| Symptom | Check |
| --- | --- |
| `kind create cluster` fails with `could not find a log line that matches "Reached target .*Multi-User System.*"` | The node did not boot within kind's time limit, usually because Docker is short on CPU or memory. Stop other kind clusters, or give Docker more resources, and run `make evaluation` again. |
| The OpenBao cluster stays unavailable and its logs show `dial tcp 10.96.0.1:443: i/o timeout` | The operator's NetworkPolicy does not allow the API server endpoint. The script sets `spec.network.apiServerEndpointIPs` to the kind API server address; check that the field is present with `kubectl --namespace openbao-demo get openbaocluster evaluation -o yaml`. |
| The OpenBao cluster stays unavailable | `kubectl --namespace openbao-demo describe openbaocluster evaluation` and the controller logs in `openbao-operator-system`. Image pulls can take several minutes. |
| `make evaluation-open` reports the port in use | Set `CONSOLE_EVALUATION_PORT`, for example `CONSOLE_EVALUATION_PORT=9443`, for both `make evaluation` and `make evaluation-open`. The public address must match the port. |
| The setup window expired | Setup closes one hour after the console starts. Restart it with `kubectl --namespace console-system rollout restart deployment/openbao-operator-console`, with `KUBECONFIG` set as above. |
