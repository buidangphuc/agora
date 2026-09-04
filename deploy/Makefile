# Local GitOps deploy plane (EKS/ECR/ALB/ArgoCD stand-ins). See README.md.
.PHONY: cluster-up cluster-down images argocd-ui gitops-push smoke status

cluster-up:            ## kind + registry + ingress + argocd + gitea + app-of-apps
	bash kind/bootstrap.sh
	bash gitea/bootstrap-gitea.sh

images:                ## retag local service images -> registry (ECR) and push
	bash images/push-images.sh

gitops-push:           ## commit + push platform-gitops to Gitea (ArgoCD will sync)
	cd ../platform-gitops && git add -A && \
	  git -c user.email=ci@local -c user.name=ci commit -q -m "gitops: manual push" || true; \
	  git push -q origin main

argocd-ui:             ## port-forward ArgoCD UI to https://localhost:8083 (admin / printed pw)
	@kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' | base64 -d; echo
	kubectl -n argocd port-forward svc/argocd-server 8083:443

status:                ## ArgoCD apps + marketplace pods
	kubectl -n argocd get applications -o wide
	kubectl -n marketplace get pods 2>/dev/null || true

smoke:                 ## run platform-e2e smoke against the cluster ingress
	cd ../platform-e2e && . .venv/bin/activate && \
	  BASE_URL=http://marketplace.localtest.me GATEWAY_URL=http://marketplace.localtest.me/api \
	  HEADLESS=true python -m pytest -m smoke -q

cluster-down:          ## delete cluster + registry
	kind delete cluster --name marketplace || true
	docker rm -f kind-registry || true
