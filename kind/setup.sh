#!/bin/bash
set -e

echo "Creating kind cluster..."
kind create cluster --config kind/kind-config.yaml

echo "Loading mock backend image..."
docker build -t mock-backend:local ./mock-backend
kind load docker-image mock-backend:local --name kthena-bench

echo "Creating namespace..."
kubectl create namespace kthena-system

echo "Deploying mock backend..."
kubectl apply -f kind/mock-backend.yaml

echo "Waiting for pods..."
kubectl wait --for=condition=ready pod -l app=mock-backend \
  -n kthena-system --timeout=60s

echo "Done. Mock backend running at:"
kubectl get svc -n kthena-system
