package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

var gameServerGVR = schema.GroupVersionResource{
	Group:    "agones.dev",
	Version:  "v1",
	Resource: "gameservers",
}

type CreateServerRequest struct {
	ServerName   string `json:"serverName"`
	MaxPlayers   string `json:"maxPlayers"`
	RconPassword string `json:"rconPassword"`
}

type ServerResponse struct {
	Name    string `json:"name"`
	State   string `json:"state"`
	Address string `json:"address,omitempty"`
	Port    int64  `json:"port,omitempty"`
	Connect string `json:"connect,omitempty"`
	Message string `json:"message,omitempty"`
}

func getenv(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func newCS2GameServer(req CreateServerRequest) *unstructured.Unstructured {
	serverName := req.ServerName
	if serverName == "" {
		serverName = "CS2 Agones Server"
	}

	maxPlayers := req.MaxPlayers
	if maxPlayers == "" {
		maxPlayers = "10"
	}

	rconPassword := req.RconPassword
	if rconPassword == "" {
		rconPassword = "change-this-password"
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "agones.dev/v1",
			"kind":       "GameServer",
			"metadata": map[string]interface{}{
				"generateName": "cs2-",
				"labels": map[string]interface{}{
					"game": "cs2",
				},
			},
			"spec": map[string]interface{}{
				"container": "cs2",
				"ports": []interface{}{
					map[string]interface{}{
						"name":          "game",
						"portPolicy":    "Dynamic",
						"containerPort": int64(27015),
						"protocol":      "TCPUDP",
					},
					map[string]interface{}{
						"name":          "sourcetv",
						"portPolicy":    "Dynamic",
						"containerPort": int64(27020),
						"protocol":      "UDP",
					},
				},
				"health": map[string]interface{}{
					"disabled":            false,
					"initialDelaySeconds": int64(30),
					"periodSeconds":       int64(5),
					"failureThreshold":    int64(10),
				},
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"terminationGracePeriodSeconds": int64(5),
						"containers": []interface{}{
							map[string]interface{}{
								"name":            "cs2",
								"image":           "joedwards32/cs2:latest",
								"imagePullPolicy": "IfNotPresent",
								"env": []interface{}{
									map[string]interface{}{
										"name": "SRCDS_TOKEN",
										"valueFrom": map[string]interface{}{
											"secretKeyRef": map[string]interface{}{
												"name": "cs2-secret",
												"key":  "", // put token
											},
										},
									},
									map[string]interface{}{
										"name":  "CS2_SERVERNAME",
										"value": serverName,
									},
									map[string]interface{}{
										"name":  "CS2_PORT",
										"value": "27015",
									},
									map[string]interface{}{
										"name":  "CS2_RCONPW",
										"value": rconPassword,
									},
									map[string]interface{}{
										"name":  "CS2_MAXPLAYERS",
										"value": maxPlayers,
									},
									map[string]interface{}{
										"name":  "CS2_LAN",
										"value": "0",
									},
								},
								"ports": []interface{}{
									map[string]interface{}{
										"name":          "game-tcp",
										"containerPort": int64(27015),
										"protocol":      "TCP",
									},
									map[string]interface{}{
										"name":          "game-udp",
										"containerPort": int64(27015),
										"protocol":      "UDP",
									},
									map[string]interface{}{
										"name":          "sourcetv",
										"containerPort": int64(27020),
										"protocol":      "UDP",
									},
								},
								"resources": map[string]interface{}{
									"requests": map[string]interface{}{
										"cpu":    "2",
										"memory": "2Gi",
									},
									"limits": map[string]interface{}{
										"cpu":    "4",
										"memory": "6Gi",
									},
								},
							},
							map[string]interface{}{
								"name":            "agones-lifecycle",
								"image":           "cs2-agones:v1",
								"imagePullPolicy": "IfNotPresent",
								"env": []interface{}{
									map[string]interface{}{
										"name":  "GAME_PORT",
										"value": "27015",
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func extractGamePort(gs *unstructured.Unstructured) int64 {
	ports, found, _ := unstructured.NestedSlice(gs.Object, "status", "ports")
	if !found {
		return 0
	}

	for _, item := range ports {
		portMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		name, _ := portMap["name"].(string)
		if name != "game" {
			continue
		}

		switch value := portMap["port"].(type) {
		case int64:
			return value
		case int:
			return int64(value)
		case float64:
			return int64(value)
		}
	}

	return 0
}

func waitForGameServerReady(
	ctx context.Context,
	client dynamic.Interface,
	namespace string,
	name string,
	timeout time.Duration,
) (*ServerResponse, error) {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		gs, err := client.
			Resource(gameServerGVR).
			Namespace(namespace).
			Get(ctx, name, metav1.GetOptions{})

		if err != nil {
			return nil, err
		}

		state, _, _ := unstructured.NestedString(gs.Object, "status", "state")
		address, _, _ := unstructured.NestedString(gs.Object, "status", "address")
		port := extractGamePort(gs)

		if state == "Ready" && address != "" && port != 0 {
			return &ServerResponse{
				Name:    name,
				State:   state,
				Address: address,
				Port:    port,
				Connect: fmt.Sprintf("connect %s:%d", address, port),
			}, nil
		}

		time.Sleep(5 * time.Second)
	}

	return nil, fmt.Errorf("timed out waiting for GameServer %s to become Ready", name)
}

func main() {
	namespace := getenv("NAMESPACE", "default")

	config, err := rest.InClusterConfig()
	if err != nil {
		log.Fatalf("failed to create in-cluster config: %v", err)
	}

	client, err := dynamic.NewForConfig(config)
	if err != nil {
		log.Fatalf("failed to create Kubernetes dynamic client: %v", err)
	}

	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	http.HandleFunc("/servers", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req CreateServerRequest
		_ = json.NewDecoder(r.Body).Decode(&req)

		ctx := r.Context()

		created, err := client.
			Resource(gameServerGVR).
			Namespace(namespace).
			Create(ctx, newCS2GameServer(req), metav1.CreateOptions{})

		if err != nil {
			http.Error(w, fmt.Sprintf("failed to create GameServer: %v", err), http.StatusInternalServerError)
			return
		}

		name := created.GetName()

		resp, err := waitForGameServerReady(ctx, client, namespace, name, 15*time.Minute)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(ServerResponse{
				Name:    name,
				State:   "Creating",
				Message: err.Error(),
			})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	log.Println("CS2 API listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
