package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	aiv1alpha1 "github.com/volcano-sh/kthena/pkg/apis/networking/v1alpha1"
	"github.com/volcano-sh/kthena/pkg/kthena-router/datastore"
	"github.com/volcano-sh/kthena/pkg/kthena-router/router"
)

func main() {
	log.Println("Starting Mock Kthena Router (No K8s)")

	store := datastore.New()

	backend1IP := os.Getenv("BACKEND_1_IP") // e.g. "backend-1"
	backend2IP := os.Getenv("BACKEND_2_IP") // e.g. "backend-2"
	
	if backend1IP == "" { backend1IP = "127.0.0.1" }
	if backend2IP == "" { backend2IP = "127.0.0.1" }

	modelName := "mock-model"
	ms := &aiv1alpha1.ModelServer{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "mock-model-server"},
		Spec: aiv1alpha1.ModelServerSpec{
			Model: &modelName,
			WorkloadPort: aiv1alpha1.WorkloadPort{Port: 8080},
		},
	}
	
	mr := &aiv1alpha1.ModelRoute{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "mock-model-route"},
		Spec: aiv1alpha1.ModelRouteSpec{
			ModelName: "mock-model",
			Rules: []*aiv1alpha1.Rule{
				{
					TargetModels: []*aiv1alpha1.TargetModel{
						{
							ModelServerName: "mock-model-server",
						},
					},
				},
			},
		},
	}

	pod1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "pod-1"},
		Status: corev1.PodStatus{PodIP: backend1IP},
	}
	pod2 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "pod-2"},
		Status: corev1.PodStatus{PodIP: backend2IP},
	}

	err := store.AddOrUpdateModelServer(ms, nil)
	if err != nil { log.Fatal(err) }
	
	err = store.AddOrUpdateModelRoute(mr)
	if err != nil { log.Fatal(err) }
	
	err = store.AddOrUpdatePod(pod1, []*aiv1alpha1.ModelServer{ms})
	if err != nil { log.Fatal(err) }

	err = store.AddOrUpdatePod(pod2, []*aiv1alpha1.ModelServer{ms})
	if err != nil { log.Fatal(err) }

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	store.Run(ctx)
	
	r := router.NewRouter(store, "router-config.yaml")
	
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(gin.Recovery())
	
	engine.GET("/healthz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"message": "ok"}) })
	engine.GET("/readyz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"message": "router is ready"}) })
	engine.GET("/metrics", gin.WrapH(promhttp.Handler()))
	
	v1Group := engine.Group("/v1")
	v1Group.Use(r.AccessLog())
	v1Group.Use(r.Auth())
	v1Group.Any("/*path", r.HandlerFunc())

	log.Println("Mock Kthena Router listening on :8080")
	if err := http.ListenAndServe(":8080", engine); err != nil {
		log.Fatal(err)
	}
}
