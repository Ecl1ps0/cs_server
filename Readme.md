# CS2 Agones Minikube Backend Setup

This project runs a Counter-Strike 2 dedicated server through Agones on Minikube.

The backend exposes an HTTP endpoint:

```http
POST /servers
```

When this endpoint is called, the backend creates a new Agones `GameServer`, starts a CS2 server, waits for it to become ready, and returns the connection address and port.

---

## 1. Start Minikube

Delete the old cluster if needed:

```bash
minikube delete -p agones
```

Start a fresh Minikube profile:

```bash
minikube start -p agones --kubernetes-version=v1.34.6 --cpus=6 --memory=10g --disk-size=120g --ports=7000-7100:7000-7100/udp --ports=7000-7100:7000-7100/tcp
```

The port range `7000-7100` is used by Agones for dynamically allocated game server ports.

---

## 2. Install Agones

Add the Agones Helm repository:

```bash
helm repo add agones https://agones.dev/chart/stable
```

Update Helm repositories:

```bash
helm repo update
```

Install Agones with the same dynamic port range:

```bash
helm install agones agones/agones --namespace agones-system --create-namespace --set gameservers.minPort=7000,gameservers.maxPort=7100
```

Check Agones pods:

```bash
kubectl get pods -n agones-system
```

---

## 3. Create CS2 Steam Token Secret

Create a Kubernetes secret containing your Steam Game Server Login Token:

```bash
kubectl create secret generic cs2-secret --from-literal=SRCDS_TOKEN='YOUR_STEAM_GSLT_TOKEN'
```

The backend-created CS2 `GameServer` reads this secret as the `SRCDS_TOKEN` environment variable.

---

## 4. Build Lifecycle Sidecar

The CS2 server does not call the Agones SDK by itself.

The lifecycle sidecar waits until CS2 opens port `27015`, then calls Agones `Ready()` and keeps sending `Health()`.

Build and load the lifecycle image:

```bash
docker build -t cs2-agones:v1 . 
minikube image load cs2-agones-lifecycle:v1 -p agones
```

---

## 5. Build Backend API

Build and load the backend image:

```bash
cd api 
docker build -t cs2-api:v1 . 
minikube image load cs2-api:v1 -p agones
```

---

## 6. Deploy Backend API

Apply the Kubernetes manifests:

```bash
kubectl apply -f api/cs2-api.yaml
```

Wait until the backend is running:

```bash
kubectl rollout status deployment/cs2-api
```

Check backend pod:

```bash
kubectl get pods -l app=cs2-api
```

Check backend logs:

```bash
kubectl logs -f deployment/cs2-api
```

---

## 7. Port-Forward Backend API

Forward the backend service to localhost:

```bash
kubectl port-forward svc/cs2-api 8080:8080
```

Keep this terminal open.

In another terminal, test the health endpoint:

```bash
curl http://localhost:8080/healthz
```

Expected response:

```text
ok
```

---

## 8. Create a New CS2 Server Through API

Call the backend endpoint:

```bash
curl -X POST http://localhost:8080/servers -H "Content-Type: application/json" -d '{"serverName":"CS2 From Backend API","maxPlayers":"10","rconPassword":"change-this-password"}'
```

The backend creates a new Agones `GameServer`.

First startup can take several minutes because the CS2 dedicated server may need to download or update server files.
